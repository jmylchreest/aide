// Package clone provides code clone detection using Rabin-Karp rolling hash.
//
// The algorithm works as follows:
// 1. Tokenize source files into normalized token sequences using tree-sitter.
// 2. Compute Rabin-Karp rolling hashes over sliding windows of tokens.
// 3. Build an index mapping hash → locations.
// 4. Detect clone pairs where multiple locations share the same hash.
// 5. Report findings with file, line, and similarity details.
package clone

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Token represents a single normalized token from source code.
type Token struct {
	Kind string // Normalized token type (e.g., "id", "lit", "op", "kw")
	Line int    // Source line (1-indexed)
}

// TokenSequence holds the token list for a file.
type TokenSequence struct {
	FilePath string
	Tokens   []Token
}

// identifierTypes are tree-sitter node types that represent identifiers.
// These get normalized to "id" to detect structural clones.
var identifierTypes = map[string]bool{
	"identifier":                            true,
	"type_identifier":                       true,
	"field_identifier":                      true,
	"package_identifier":                    true,
	"property_identifier":                   true,
	"shorthand_property_identifier":         true,
	"shorthand_property_identifier_pattern": true,
	// Additional cross-language identifier node types
	"variable_name":      true, // PHP
	"name":               true, // PHP, Ruby
	"constant":           true, // Ruby, Elixir
	"atom":               true, // Elixir
	"symbol":             true, // Ruby
	"simple_identifier":  true, // Kotlin, Swift
	"word":               true, // Bash
	"variable_reference": true, // Bash (e.g., $VAR)
}

// literalTypes are tree-sitter node types for literal values.
// These get normalized to "lit" to detect structural clones.
var literalTypes = map[string]bool{
	"interpreted_string_literal": true,
	"raw_string_literal":         true,
	"string":                     true,
	"template_string":            true,
	"string_literal":             true,
	"number":                     true,
	"integer":                    true,
	"float":                      true,
	"int_literal":                true,
	"float_literal":              true,
	"true":                       true,
	"false":                      true,
	"nil":                        true,
	"null":                       true,
	"none":                       true,
	"None":                       true,
	"undefined":                  true,
	// Additional cross-language literal node types
	"integer_literal":           true, // Kotlin, C, C++
	"real_literal":              true, // Kotlin, C#
	"long_literal":              true, // Kotlin
	"character_literal":         true, // Java, Kotlin, C
	"hex_literal":               true, // Various
	"octal_literal":             true, // Various
	"binary_literal":            true, // Various
	"boolean_literal":           true, // Kotlin, Scala
	"string_content":            true, // Various
	"heredoc_content":           true, // Ruby, Bash, PHP
	"regex":                     true, // Ruby, JS
	"regex_literal":             true, // Various
	"encapsed_string":           true, // PHP
	"nowdoc_string":             true, // PHP
	"line_string_literal":       true, // Kotlin
	"multi_line_string_literal": true, // Kotlin, Swift
	"True":                      true, // Python
	"False":                     true, // Python
}

// keywordTypes are significant structural keywords.
var keywordTypes = map[string]bool{
	"if":       true,
	"else":     true,
	"for":      true,
	"while":    true,
	"switch":   true,
	"case":     true,
	"return":   true,
	"break":    true,
	"continue": true,
	"func":     true,
	"function": true,
	"def":      true,
	"class":    true,
	"struct":   true,
	"import":   true,
	"try":      true,
	"catch":    true,
	"finally":  true,
	"throw":    true,
	"async":    true,
	"await":    true,
	// Additional cross-language structural keywords
	"do":        true,
	"match":     true, // Rust, Scala
	"when":      true, // Kotlin
	"guard":     true, // Swift
	"yield":     true, // Python, JS
	"lambda":    true, // Python
	"fn":        true, // Rust, Elixir
	"let":       true, // JS, Rust, Swift
	"var":       true, // JS, Go, Swift, Kotlin
	"val":       true, // Kotlin, Scala
	"const":     true, // JS, Go
	"enum":      true, // Various
	"interface": true, // Various
	"trait":     true, // Rust, Scala, PHP
	"impl":      true, // Rust
	"module":    true, // Ruby, Elixir
	"require":   true, // Ruby, Lua
	"include":   true, // Ruby, C/C++
	"rescue":    true, // Ruby
	"ensure":    true, // Ruby
	"unless":    true, // Ruby
	"until":     true, // Ruby
	"elsif":     true, // Ruby
	"elif":      true, // Python
	"except":    true, // Python
	"raise":     true, // Python, Ruby
	"with":      true, // Python
	"select":    true, // Go
	"defer":     true, // Go
	"go":        true, // Go
}

// Tokenize parses source content using tree-sitter and produces a normalized
// token sequence. Identifiers are normalized to "id", literals to "lit",
// keywords and operators are preserved for structural matching.
//
// Pack-specific tokenisation types (from grammar packs) are merged with
// the generic base set — extending, not replacing.
func Tokenize(loader grammar.Loader, filePath string, content []byte, lang string) (*TokenSequence, error) {
	sitterLang, err := loader.Load(context.Background(), lang)
	if err != nil {
		return nil, nil // grammar not available — skip
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitterLang); err != nil {
		return nil, err
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse %s", filePath)
	}
	defer tree.Close()

	// Build merged token type lookups (pack-specific + generic base).
	lookup := buildTokenLookup(lang)

	var tokens []Token
	walkLeaves(tree.RootNode(), content, &tokens, lookup)

	return &TokenSequence{
		FilePath: filePath,
		Tokens:   tokens,
	}, nil
}

// tokenLookup holds the merged identifier/literal/keyword type sets for a
// specific language. Created once per Tokenize call.
type tokenLookup struct {
	identifiers map[string]bool
	literals    map[string]bool
	keywords    map[string]bool
}

// buildTokenLookup creates a merged token lookup for the given language.
// Pack-specific types extend the generic base sets (merge strategy).
func buildTokenLookup(lang string) *tokenLookup {
	// Start with copies of the generic base sets.
	ids := make(map[string]bool, len(identifierTypes)+8)
	for k, v := range identifierTypes {
		ids[k] = v
	}
	lits := make(map[string]bool, len(literalTypes)+8)
	for k, v := range literalTypes {
		lits[k] = v
	}
	kws := make(map[string]bool, len(keywordTypes)+8)
	for k, v := range keywordTypes {
		kws[k] = v
	}

	// Merge pack-specific types if available.
	if pack := grammar.DefaultPackRegistry().Get(lang); pack != nil && pack.Tokenisation != nil {
		for _, t := range pack.Tokenisation.IdentifierTypes {
			ids[t] = true
		}
		for _, t := range pack.Tokenisation.LiteralTypes {
			lits[t] = true
		}
		for _, t := range pack.Tokenisation.KeywordTypes {
			kws[t] = true
		}
	}

	return &tokenLookup{
		identifiers: ids,
		literals:    lits,
		keywords:    kws,
	}
}

// walkLeaves performs a DFS over the tree and collects leaf nodes as tokens.
func walkLeaves(node *tree_sitter.Node, content []byte, tokens *[]Token, lookup *tokenLookup) {
	if node.ChildCount() == 0 {
		// Leaf node — normalize and collect.
		nodeType := node.Kind()
		line := int(node.StartPosition().Row) + 1 // tree-sitter is 0-indexed

		kind := normalizeToken(nodeType, node, content, lookup)
		if kind != "" {
			*tokens = append(*tokens, Token{Kind: kind, Line: line})
		}
		return
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			walkLeaves(child, content, tokens, lookup)
		}
	}
}

// normalizeToken maps a tree-sitter leaf node to a normalized token kind.
func normalizeToken(nodeType string, node *tree_sitter.Node, content []byte, lookup *tokenLookup) string {
	// Skip comments and whitespace.
	if strings.HasSuffix(nodeType, "comment") || nodeType == "comment" {
		return ""
	}

	// Normalize identifiers.
	if lookup.identifiers[nodeType] {
		return "id"
	}

	// Normalize literals.
	if lookup.literals[nodeType] {
		return "lit"
	}

	// Preserve keywords.
	if lookup.keywords[nodeType] {
		return "kw:" + nodeType
	}

	// Operators and punctuation — keep as-is for structural matching.
	text := string(content[node.StartByte():node.EndByte()])
	if len(text) <= 3 {
		return text // operators: +, -, *, /, =, ==, !=, etc.
	}

	// Longer unnamed tokens — normalize.
	return nodeType
}
