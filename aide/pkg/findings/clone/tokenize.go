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

	"github.com/jmylchreest/aide/aide/pkg/code"
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

// langProvider returns the tree-sitter language for a given language identifier.
var supportedTokenizeLanguages = map[string]bool{
	code.LangGo:         true,
	code.LangTypeScript: true,
	code.LangJavaScript: true,
	code.LangPython:     true,
	code.LangRust:       true,
	code.LangJava:       true,
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
}

// Tokenize parses source content using tree-sitter and produces a normalized
// token sequence. Identifiers are normalized to "id", literals to "lit",
// keywords and operators are preserved for structural matching.
func Tokenize(loader grammar.Loader, filePath string, content []byte, lang string) (*TokenSequence, error) {
	if !supportedTokenizeLanguages[lang] {
		return nil, nil // unsupported language — skip
	}

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

	var tokens []Token
	walkLeaves(tree.RootNode(), content, &tokens)

	return &TokenSequence{
		FilePath: filePath,
		Tokens:   tokens,
	}, nil
}

// walkLeaves performs a DFS over the tree and collects leaf nodes as tokens.
func walkLeaves(node *tree_sitter.Node, content []byte, tokens *[]Token) {
	if node.ChildCount() == 0 {
		// Leaf node — normalize and collect.
		nodeType := node.Kind()
		line := int(node.StartPosition().Row) + 1 // tree-sitter is 0-indexed

		kind := normalizeToken(nodeType, node, content)
		if kind != "" {
			*tokens = append(*tokens, Token{Kind: kind, Line: line})
		}
		return
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			walkLeaves(child, content, tokens)
		}
	}
}

// normalizeToken maps a tree-sitter leaf node to a normalized token kind.
func normalizeToken(nodeType string, node *tree_sitter.Node, content []byte) string {
	// Skip comments and whitespace.
	if strings.HasSuffix(nodeType, "comment") || nodeType == "comment" {
		return ""
	}

	// Normalize identifiers.
	if identifierTypes[nodeType] {
		return "id"
	}

	// Normalize literals.
	if literalTypes[nodeType] {
		return "lit"
	}

	// Preserve keywords.
	if keywordTypes[nodeType] {
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
