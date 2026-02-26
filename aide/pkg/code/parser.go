// Package code provides code indexing and symbol extraction.
package code

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/oklog/ulid/v2"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Parser extracts symbols from source code using tree-sitter queries.
// Languages are loaded via the grammar.Loader (built-in or dynamic).
// Queries are compiled and cached lazily per language.
type Parser struct {
	mu         sync.Mutex
	loader     grammar.Loader
	languages  map[string]*tree_sitter.Language // Loaded grammars (cache)
	queries    map[string]*tree_sitter.Query    // Compiled tag queries (cache)
	refQueries map[string]*tree_sitter.Query    // Compiled reference queries (cache)
}

// NewParser creates a new code parser backed by the given grammar loader.
// Grammars and queries are loaded lazily on first use.
func NewParser(loader grammar.Loader) *Parser {
	return &Parser{
		loader:     loader,
		languages:  make(map[string]*tree_sitter.Language),
		queries:    make(map[string]*tree_sitter.Query),
		refQueries: make(map[string]*tree_sitter.Query),
	}
}

// getLanguage returns the tree-sitter language for a given language name,
// loading and caching it on first access.
func (p *Parser) getLanguage(lang string) *tree_sitter.Language {
	p.mu.Lock()
	defer p.mu.Unlock()

	if l, ok := p.languages[lang]; ok {
		return l
	}

	l, err := p.loader.Load(context.Background(), lang)
	if err != nil {
		return nil
	}

	p.languages[lang] = l
	return l
}

// getTagQuery returns the compiled tag query for a language,
// compiling and caching it on first access. Also loads the grammar if needed.
func (p *Parser) getTagQuery(lang string) *tree_sitter.Query {
	p.mu.Lock()
	defer p.mu.Unlock()

	if q, ok := p.queries[lang]; ok {
		return q
	}

	pattern, ok := TagQueries[lang]
	if !ok {
		return nil
	}

	// Ensure language grammar is loaded
	sitterLang, ok := p.languages[lang]
	if !ok {
		l, err := p.loader.Load(context.Background(), lang)
		if err != nil {
			return nil
		}
		sitterLang = l
		p.languages[lang] = sitterLang
	}

	q, qErr := tree_sitter.NewQuery(sitterLang, pattern)
	if qErr != nil {
		return nil
	}

	p.queries[lang] = q
	return q
}

// getRefQuery returns the compiled reference query for a language,
// compiling and caching it on first access. Also loads the grammar if needed.
func (p *Parser) getRefQuery(lang string) *tree_sitter.Query {
	p.mu.Lock()
	defer p.mu.Unlock()

	if q, ok := p.refQueries[lang]; ok {
		return q
	}

	pattern, ok := RefQueries[lang]
	if !ok {
		return nil
	}

	// Ensure language grammar is loaded
	sitterLang, ok := p.languages[lang]
	if !ok {
		l, err := p.loader.Load(context.Background(), lang)
		if err != nil {
			return nil
		}
		sitterLang = l
		p.languages[lang] = sitterLang
	}

	q, qErr := tree_sitter.NewQuery(sitterLang, pattern)
	if qErr != nil {
		return nil
	}

	p.refQueries[lang] = q
	return q
}

// DetectLanguage determines the language for a file using multiple heuristics:
// 1. File extension (fastest, covers ~95% of cases)
// 2. Known filenames (Makefile, Jenkinsfile, etc.)
// 3. Shebang line (for extensionless scripts, requires content)
func DetectLanguage(filePath string, content []byte) string {
	// 1. Try file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := LangExtensions[ext]; ok {
		return lang
	}

	// 2. Try known filenames
	base := filepath.Base(filePath)
	if lang, ok := LangFilenames[base]; ok {
		return lang
	}

	// 3. Try shebang (if content provided)
	if len(content) > 0 {
		return detectShebang(content)
	}

	return ""
}

// detectShebang parses the first line of content for a shebang interpreter.
func detectShebang(content []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	if !scanner.Scan() {
		return ""
	}
	line := scanner.Text()

	if !strings.HasPrefix(line, "#!") {
		return ""
	}

	// Parse "#!/usr/bin/env python3" or "#!/usr/bin/python3"
	shebang := strings.TrimPrefix(line, "#!")
	shebang = strings.TrimSpace(shebang)

	// Split on whitespace to get the command
	parts := strings.Fields(shebang)
	if len(parts) == 0 {
		return ""
	}

	// If using /usr/bin/env, the interpreter is the next argument
	interpreter := filepath.Base(parts[0])
	if interpreter == "env" && len(parts) > 1 {
		interpreter = filepath.Base(parts[1])
	}

	// Strip version suffixes (python3.11 -> python3 -> python)
	if lang, ok := ShebangLangs[interpreter]; ok {
		return lang
	}
	// Try stripping trailing digits (python3 -> python)
	stripped := strings.TrimRight(interpreter, "0123456789.")
	if lang, ok := ShebangLangs[stripped]; ok {
		return lang
	}

	return ""
}

// TagQueries contains tree-sitter query patterns for extracting symbols.
// Based on standard tags.scm patterns from tree-sitter grammar repositories.
var TagQueries = map[string]string{
	LangGo: `
		(function_declaration name: (identifier) @name) @definition.function
		(method_declaration name: (field_identifier) @name) @definition.method
		(type_declaration (type_spec name: (type_identifier) @name type: (struct_type))) @definition.class
		(type_declaration (type_spec name: (type_identifier) @name type: (interface_type))) @definition.interface
		(type_declaration (type_spec name: (type_identifier) @name)) @definition.type
	`,
	LangPython: `
		(function_definition name: (identifier) @name) @definition.function
		(class_definition name: (identifier) @name) @definition.class
	`,
	LangTypeScript: `
		(function_declaration name: (identifier) @name) @definition.function
		(method_definition name: (property_identifier) @name) @definition.method
		(class_declaration name: (type_identifier) @name) @definition.class
		(interface_declaration name: (type_identifier) @name) @definition.interface
		(type_alias_declaration name: (type_identifier) @name) @definition.type
		(enum_declaration name: (identifier) @name) @definition.class
	`,
	LangJavaScript: `
		(function_declaration name: (identifier) @name) @definition.function
		(method_definition name: (property_identifier) @name) @definition.method
		(class_declaration name: (identifier) @name) @definition.class
	`,
	LangRust: `
		(function_item name: (identifier) @name) @definition.function
		(impl_item type: (type_identifier) @name) @definition.class
		(struct_item name: (type_identifier) @name) @definition.class
		(enum_item name: (type_identifier) @name) @definition.class
		(trait_item name: (type_identifier) @name) @definition.interface
		(type_item name: (type_identifier) @name) @definition.type
		(mod_item name: (identifier) @name) @definition.module
	`,
	LangJava: `
		(method_declaration name: (identifier) @name) @definition.method
		(constructor_declaration name: (identifier) @name) @definition.method
		(class_declaration name: (identifier) @name) @definition.class
		(interface_declaration name: (identifier) @name) @definition.interface
		(enum_declaration name: (identifier) @name) @definition.class
	`,
	LangC: `
		(function_definition declarator: (function_declarator declarator: (identifier) @name)) @definition.function
		(struct_specifier name: (type_identifier) @name) @definition.class
		(enum_specifier name: (type_identifier) @name) @definition.class
		(type_definition declarator: (type_identifier) @name) @definition.type
	`,
	LangCPP: `
		(function_definition declarator: (function_declarator declarator: (identifier) @name)) @definition.function
		(function_definition declarator: (function_declarator declarator: (qualified_identifier name: (identifier) @name))) @definition.method
		(class_specifier name: (type_identifier) @name) @definition.class
		(struct_specifier name: (type_identifier) @name) @definition.class
		(enum_specifier name: (type_identifier) @name) @definition.class
	`,
	LangCSharp: `
		(method_declaration name: (identifier) @name) @definition.method
		(constructor_declaration name: (identifier) @name) @definition.method
		(class_declaration name: (identifier) @name) @definition.class
		(interface_declaration name: (identifier) @name) @definition.interface
		(struct_declaration name: (identifier) @name) @definition.class
		(enum_declaration name: (identifier) @name) @definition.class
	`,
	LangRuby: `
		(method name: (identifier) @name) @definition.method
		(singleton_method name: (identifier) @name) @definition.method
		(class name: (constant) @name) @definition.class
		(module name: (constant) @name) @definition.module
	`,
	LangPHP: `
		(function_definition name: (name) @name) @definition.function
		(method_declaration name: (name) @name) @definition.method
		(class_declaration name: (name) @name) @definition.class
		(interface_declaration name: (name) @name) @definition.interface
		(trait_declaration name: (name) @name) @definition.interface
	`,
	LangSwift: `
		(function_declaration name: (simple_identifier) @name) @definition.function
		(class_declaration name: (type_identifier) @name) @definition.class
		(struct_declaration name: (type_identifier) @name) @definition.class
		(protocol_declaration name: (type_identifier) @name) @definition.interface
		(enum_declaration name: (type_identifier) @name) @definition.class
	`,
	LangKotlin: `
		(function_declaration (simple_identifier) @name) @definition.function
		(class_declaration (type_identifier) @name) @definition.class
		(object_declaration (type_identifier) @name) @definition.class
	`,
	LangScala: `
		(function_definition name: (identifier) @name) @definition.function
		(class_definition name: (identifier) @name) @definition.class
		(object_definition name: (identifier) @name) @definition.class
		(trait_definition name: (identifier) @name) @definition.interface
	`,
	LangElixir: `
		(call target: (identifier) @_def arguments: (arguments (identifier) @name)) @definition.function
		(call target: (identifier) @_defmodule arguments: (arguments (alias) @name)) @definition.module
	`,
	LangLua: `
		(function_declaration name: (identifier) @name) @definition.function
		(function_declaration name: (dot_index_expression field: (identifier) @name)) @definition.method
		(assignment_statement (variable_list name: (identifier) @name) (expression_list value: (function_definition))) @definition.function
	`,
	LangSQL: `
		(create_function_statement name: (identifier) @name) @definition.function
		(create_table_statement name: (identifier) @name) @definition.class
	`,
	LangProtobuf: `
		(message name: (message_name) @name) @definition.class
		(enum name: (enum_name) @name) @definition.class
		(service name: (service_name) @name) @definition.interface
		(rpc name: (rpc_name) @name) @definition.method
	`,
	LangHCL: `
		(block (identifier) @type (string_lit) @name) @definition.type
	`,
	LangBash: `
		(function_definition name: (word) @name) @definition.function
	`,
}

// RefQueries contains tree-sitter query patterns for extracting references (call sites).
// These capture function/method calls and type references.
var RefQueries = map[string]string{
	LangGo: `
		(call_expression function: (identifier) @name) @reference.call
		(call_expression function: (selector_expression field: (field_identifier) @name)) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangPython: `
		(call function: (identifier) @name) @reference.call
		(call function: (attribute attribute: (identifier) @name)) @reference.call
		(type (identifier) @name) @reference.type
	`,
	LangTypeScript: `
		(call_expression function: (identifier) @name) @reference.call
		(call_expression function: (member_expression property: (property_identifier) @name)) @reference.call
		(new_expression constructor: (identifier) @name) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangJavaScript: `
		(call_expression function: (identifier) @name) @reference.call
		(call_expression function: (member_expression property: (property_identifier) @name)) @reference.call
		(new_expression constructor: (identifier) @name) @reference.call
	`,
	LangRust: `
		(call_expression function: (identifier) @name) @reference.call
		(call_expression function: (field_expression field: (field_identifier) @name)) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangJava: `
		(method_invocation name: (identifier) @name) @reference.call
		(object_creation_expression type: (type_identifier) @name) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangC: `
		(call_expression function: (identifier) @name) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangCPP: `
		(call_expression function: (identifier) @name) @reference.call
		(call_expression function: (field_expression field: (field_identifier) @name)) @reference.call
		(type_identifier) @name @reference.type
	`,
	LangRuby: `
		(call method: (identifier) @name) @reference.call
		(constant) @name @reference.type
	`,
	LangPHP: `
		(function_call_expression function: (name) @name) @reference.call
		(member_call_expression name: (name) @name) @reference.call
		(named_type (name) @name) @reference.type
	`,
}

// ParseFile parses a file and extracts symbols.
func (p *Parser) ParseFile(filePath string) ([]*Symbol, error) {
	// Try extension and filename detection first (no file read needed)
	lang := DetectLanguage(filePath, nil)

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// If no language detected yet, try shebang
	if lang == "" {
		lang = detectShebang(content)
	}

	if lang == "" {
		return nil, nil // Unsupported language, skip silently
	}

	return p.ParseContent(content, lang, filePath)
}

// ParseContent parses source code and extracts symbols.
func (p *Parser) ParseContent(content []byte, lang, filePath string) ([]*Symbol, error) {
	language := p.getLanguage(lang)
	if language == nil {
		return nil, nil // Unsupported language
	}

	// Create parser
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		return nil, nil
	}

	// Parse content
	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, nil
	}
	defer tree.Close()

	// Try query-based extraction first (preferred)
	if query := p.getTagQuery(lang); query != nil {
		return p.extractWithQuery(query, tree.RootNode(), content, filePath, lang), nil
	}

	// Fall back to legacy extractors for languages without queries
	var symbols []*Symbol
	switch lang {
	case LangTypeScript, LangJavaScript:
		symbols = p.extractTypeScript(tree.RootNode(), content, filePath, lang)
	case LangGo:
		symbols = p.extractGo(tree.RootNode(), content, filePath)
	case LangPython:
		symbols = p.extractPython(tree.RootNode(), content, filePath)
	}

	return symbols, nil
}

// extractWithQuery extracts symbols using a tree-sitter query.
// This is the preferred method as it uses standard tags.scm patterns.
func (p *Parser) extractWithQuery(query *tree_sitter.Query, root *tree_sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol
	seen := make(map[string]bool) // Dedupe by position

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	// Build capture name index
	captureNames := query.CaptureNames()
	nameIndex := -1
	defIndexes := make(map[uint32]string) // capture index -> kind (function, class, etc.)

	for i, captureName := range captureNames {
		if captureName == "name" {
			nameIndex = i
		} else if strings.HasPrefix(captureName, "definition.") {
			kind := strings.TrimPrefix(captureName, "definition.")
			defIndexes[uint32(i)] = kind
		}
	}

	matches := cursor.Matches(query, root, content)
	for match := matches.Next(); match != nil; match = matches.Next() {
		var name string
		var defNode *tree_sitter.Node
		var kind string

		for _, capture := range match.Captures {
			if int(capture.Index) == nameIndex {
				name = capture.Node.Utf8Text(content)
			}
			if k, ok := defIndexes[capture.Index]; ok {
				node := capture.Node
				defNode = &node
				kind = k
			}
		}

		if name == "" || defNode == nil {
			continue
		}

		// Dedupe by position
		key := filePath + ":" + name + ":" + kind
		if seen[key] {
			continue
		}
		seen[key] = true

		// Map kind to our constants
		symbolKind := mapQueryKindToSymbolKind(kind)

		sym := &Symbol{
			ID:         ulid.Make().String(),
			Name:       name,
			Kind:       symbolKind,
			Signature:  p.extractSignature(defNode, content),
			DocComment: p.extractPrecedingComment(defNode, content),
			FilePath:   filePath,
			StartLine:  int(defNode.StartPosition().Row) + 1,
			EndLine:    int(defNode.EndPosition().Row) + 1,
			Language:   lang,
			CreatedAt:  time.Now(),
		}

		// Extract body range if present
		bodyNode := defNode.ChildByFieldName("body")
		if bodyNode != nil {
			sym.BodyStartLine = int(bodyNode.StartPosition().Row) + 1
			sym.BodyEndLine = int(bodyNode.EndPosition().Row) + 1
		}

		symbols = append(symbols, sym)
	}

	return symbols
}

// mapQueryKindToSymbolKind maps tree-sitter query kinds to our symbol kinds.
func mapQueryKindToSymbolKind(queryKind string) string {
	switch queryKind {
	case "function":
		return KindFunction
	case "method":
		return KindMethod
	case "class":
		return KindClass
	case "interface":
		return KindInterface
	case "type":
		return KindType
	case "module":
		return KindType // Modules are treated as types
	case "constant":
		return KindConstant
	case "variable":
		return KindVariable
	default:
		return KindFunction
	}
}

// extractTypeScript extracts symbols from TypeScript/JavaScript AST.
func (p *Parser) extractTypeScript(root *tree_sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol

	// Walk the tree looking for relevant nodes
	p.walkNode(root, func(node *tree_sitter.Node) bool {
		nodeType := node.Kind()
		var sym *Symbol

		switch nodeType {
		case "function_declaration":
			sym = p.extractTSFunction(node, content, filePath, lang)
		case "method_definition":
			sym = p.extractTSMethod(node, content, filePath, lang)
		case "class_declaration":
			sym = p.extractTSClass(node, content, filePath, lang)
		case "interface_declaration":
			sym = p.extractTSInterface(node, content, filePath, lang)
		case "type_alias_declaration":
			sym = p.extractTSTypeAlias(node, content, filePath, lang)
		case "lexical_declaration", "variable_declaration":
			// Check for arrow functions assigned to const/let
			syms := p.extractTSVariableDeclaration(node, content, filePath, lang)
			symbols = append(symbols, syms...)
			return true // Continue to children
		}

		if sym != nil {
			symbols = append(symbols, sym)
		}
		return true // Continue walking
	})

	return symbols
}

// extractGo extracts symbols from Go AST.
func (p *Parser) extractGo(root *tree_sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	p.walkNode(root, func(node *tree_sitter.Node) bool {
		nodeType := node.Kind()
		var sym *Symbol

		switch nodeType {
		case "function_declaration":
			sym = p.extractGoFunction(node, content, filePath)
		case "method_declaration":
			sym = p.extractGoMethod(node, content, filePath)
		case "type_declaration":
			syms := p.extractGoTypeDecl(node, content, filePath)
			symbols = append(symbols, syms...)
			return true
		}

		if sym != nil {
			symbols = append(symbols, sym)
		}
		return true
	})

	return symbols
}

// extractPython extracts symbols from Python AST.
func (p *Parser) extractPython(root *tree_sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	p.walkNode(root, func(node *tree_sitter.Node) bool {
		nodeType := node.Kind()
		var sym *Symbol

		switch nodeType {
		case "function_definition":
			sym = p.extractPythonFunction(node, content, filePath)
		case "class_definition":
			sym = p.extractPythonClass(node, content, filePath)
		}

		if sym != nil {
			symbols = append(symbols, sym)
		}
		return true
	})

	return symbols
}

// walkNode walks the AST calling fn for each node.
func (p *Parser) walkNode(node *tree_sitter.Node, fn func(*tree_sitter.Node) bool) {
	if node == nil {
		return
	}

	if !fn(node) {
		return
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			p.walkNode(child, fn)
		}
	}
}

// Helper functions for TypeScript/JavaScript extraction

func (p *Parser) extractTSFunction(node *tree_sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindFunction,
		Signature:  p.extractSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractTSMethod(node *tree_sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindMethod,
		Signature:  p.extractSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractTSClass(node *tree_sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Extract class signature (class name extends X implements Y)
	sig := p.extractClassSignature(node, content)

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindClass,
		Signature:  sig,
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractTSInterface(node *tree_sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindInterface,
		Signature:  p.extractInterfaceSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractTSTypeAlias(node *tree_sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindType,
		Signature:  p.extractTypeSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractTSVariableDeclaration(node *tree_sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol

	// Look for arrow functions or function expressions assigned to variables
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode != nil && valueNode != nil {
				valType := valueNode.Kind()
				if valType == "arrow_function" || valType == "function_expression" {
					sym := &Symbol{
						ID:         ulid.Make().String(),
						Name:       p.nodeText(nameNode, content),
						Kind:       KindFunction,
						Signature:  p.extractArrowSignature(child, content),
						DocComment: p.extractPrecedingComment(node, content),
						FilePath:   filePath,
						StartLine:  int(node.StartPosition().Row) + 1,
						EndLine:    int(node.EndPosition().Row) + 1,
						Language:   lang,
						CreatedAt:  time.Now(),
					}
					setBodyRange(sym, valueNode)
					symbols = append(symbols, sym)
				}
			}
		}
	}

	return symbols
}

// Helper functions for Go extraction

func (p *Parser) extractGoFunction(node *tree_sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindFunction,
		Signature:  p.extractGoFuncSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   LangGo,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractGoMethod(node *tree_sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindMethod,
		Signature:  p.extractGoFuncSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   LangGo,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractGoTypeDecl(node *tree_sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")
			if nameNode != nil {
				kind := KindType
				if typeNode != nil {
					switch typeNode.Kind() {
					case "struct_type":
						kind = KindClass // Use class for structs
					case "interface_type":
						kind = KindInterface
					}
				}

				sym := &Symbol{
					ID:         ulid.Make().String(),
					Name:       p.nodeText(nameNode, content),
					Kind:       kind,
					Signature:  p.extractGoTypeSignature(child, content),
					DocComment: p.extractPrecedingComment(node, content),
					FilePath:   filePath,
					StartLine:  int(child.StartPosition().Row) + 1,
					EndLine:    int(child.EndPosition().Row) + 1,
					Language:   LangGo,
					CreatedAt:  time.Now(),
				}
				setBodyRange(sym, child)
				symbols = append(symbols, sym)
			}
		}
	}

	return symbols
}

// Helper functions for Python extraction

func (p *Parser) extractPythonFunction(node *tree_sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Check if this is a method (inside a class)
	kind := KindFunction
	if p.isInsideClass(node) {
		kind = KindMethod
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       kind,
		Signature:  p.extractPythonFuncSignature(node, content),
		DocComment: p.extractPythonDocstring(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   LangPython,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

func (p *Parser) extractPythonClass(node *tree_sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	sym := &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindClass,
		Signature:  p.extractPythonClassSignature(node, content),
		DocComment: p.extractPythonDocstring(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPosition().Row) + 1,
		EndLine:    int(node.EndPosition().Row) + 1,
		Language:   LangPython,
		CreatedAt:  time.Now(),
	}
	setBodyRange(sym, node)
	return sym
}

// setBodyRange extracts the body node range from a tree-sitter node and sets it on the symbol.
// This is used by legacy extractors. The query-based extractor (extractWithQuery) handles this inline.
func setBodyRange(sym *Symbol, node *tree_sitter.Node) {
	if sym == nil || node == nil {
		return
	}
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		sym.BodyStartLine = int(bodyNode.StartPosition().Row) + 1
		sym.BodyEndLine = int(bodyNode.EndPosition().Row) + 1
		return
	}
	// Go type_spec uses "type" field for the inner type node (struct_type, interface_type)
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		nodeType := typeNode.Kind()
		if nodeType == "struct_type" || nodeType == "interface_type" {
			sym.BodyStartLine = int(typeNode.StartPosition().Row) + 1
			sym.BodyEndLine = int(typeNode.EndPosition().Row) + 1
		}
	}
}

// Utility functions

func (p *Parser) nodeText(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if end > uint(len(content)) {
		end = uint(len(content))
	}
	return string(content[start:end])
}

func (p *Parser) extractSignature(node *tree_sitter.Node, content []byte) string {
	// Extract from start of node to start of body (or end if no body)
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		// Remove trailing { if present
		sig = strings.TrimSuffix(sig, "{")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractClassSignature(node *tree_sitter.Node, content []byte) string {
	// Extract "class Name extends X implements Y"
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		sig = strings.TrimSuffix(sig, "{")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractInterfaceSignature(node *tree_sitter.Node, content []byte) string {
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		sig = strings.TrimSuffix(sig, "{")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractTypeSignature(node *tree_sitter.Node, content []byte) string {
	// For type aliases, extract the full declaration
	text := p.nodeText(node, content)
	// Truncate if too long
	if len(text) > 200 {
		text = text[:200] + "..."
	}
	return text
}

func (p *Parser) extractArrowSignature(node *tree_sitter.Node, content []byte) string {
	// For arrow functions assigned to variables
	text := p.nodeText(node, content)
	// Find the arrow and truncate after parameters
	if idx := strings.Index(text, "=>"); idx > 0 {
		return strings.TrimSpace(text[:idx+2])
	}
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	return text
}

func (p *Parser) extractGoFuncSignature(node *tree_sitter.Node, content []byte) string {
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		sig = strings.TrimSuffix(sig, "{")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractGoTypeSignature(node *tree_sitter.Node, content []byte) string {
	text := p.nodeText(node, content)
	// Find { and truncate
	if idx := strings.Index(text, "{"); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	if len(text) > 200 {
		text = text[:200] + "..."
	}
	return text
}

func (p *Parser) extractPythonFuncSignature(node *tree_sitter.Node, content []byte) string {
	// Extract "def name(params) -> return_type:"
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		sig = strings.TrimSuffix(sig, ":")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractPythonClassSignature(node *tree_sitter.Node, content []byte) string {
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		start := node.StartByte()
		end := bodyNode.StartByte()
		sig := strings.TrimSpace(string(content[start:end]))
		sig = strings.TrimSuffix(sig, ":")
		return strings.TrimSpace(sig)
	}
	return p.nodeText(node, content)
}

func (p *Parser) extractPrecedingComment(node *tree_sitter.Node, content []byte) string {
	// Look for comment nodes before this node
	prev := node.PrevSibling()
	if prev == nil {
		return ""
	}

	nodeType := prev.Kind()
	if nodeType == "comment" || nodeType == "line_comment" || nodeType == "block_comment" {
		text := p.nodeText(prev, content)
		// Clean up comment markers
		text = strings.TrimPrefix(text, "//")
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimPrefix(text, "#")
		return strings.TrimSpace(text)
	}

	return ""
}

func (p *Parser) extractPythonDocstring(node *tree_sitter.Node, content []byte) string {
	// Python docstrings are the first expression in the body
	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return ""
	}

	// Check the first statement for a docstring
	if bodyNode.ChildCount() > 0 {
		child := bodyNode.Child(0)
		if child != nil && child.Kind() == "expression_statement" {
			for j := uint(0); j < child.ChildCount(); j++ {
				expr := child.Child(j)
				if expr != nil && expr.Kind() == "string" {
					text := p.nodeText(expr, content)
					// Clean up docstring markers
					text = strings.Trim(text, `"'`)
					return strings.TrimSpace(text)
				}
			}
		}
	}

	return ""
}

func (p *Parser) isInsideClass(node *tree_sitter.Node) bool {
	parent := node.Parent()
	for parent != nil {
		if parent.Kind() == "class_definition" || parent.Kind() == "class_declaration" {
			return true
		}
		parent = parent.Parent()
	}
	return false
}

// SupportedLanguage returns true if the language is supported.
func (p *Parser) SupportedLanguage(lang string) bool {
	available := p.loader.Available()
	for _, name := range available {
		if name == lang {
			return true
		}
	}
	return false
}

// SupportedExtension returns true if the file extension is supported.
func SupportedExtension(ext string) bool {
	_, ok := LangExtensions[strings.ToLower(ext)]
	return ok
}

// SupportedFile returns true if the file is supported (by extension or filename).
func SupportedFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	if _, ok := LangExtensions[ext]; ok {
		return true
	}
	base := filepath.Base(filePath)
	_, ok := LangFilenames[base]
	return ok
}

// GetLanguageForFile returns the language for a file path, or empty string if unsupported.
func GetLanguageForFile(filePath string) string {
	return DetectLanguage(filePath, nil)
}

// ParseFileReferences parses a file and extracts references (call sites).
func (p *Parser) ParseFileReferences(filePath string) ([]*Reference, error) {
	// Try extension and filename detection first
	lang := DetectLanguage(filePath, nil)

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// If no language detected yet, try shebang
	if lang == "" {
		lang = detectShebang(content)
	}

	if lang == "" {
		return nil, nil // Unsupported language, skip silently
	}

	return p.ParseContentReferences(content, lang, filePath)
}

// ParseContentReferences parses source code and extracts references.
func (p *Parser) ParseContentReferences(content []byte, lang, filePath string) ([]*Reference, error) {
	language := p.getLanguage(lang)
	if language == nil {
		return nil, nil // Unsupported language
	}

	query := p.getRefQuery(lang)
	if query == nil {
		return nil, nil // No reference query for this language
	}

	// Create parser
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		return nil, nil
	}

	// Parse content
	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, nil
	}
	defer tree.Close()

	return p.extractReferences(query, tree.RootNode(), content, filePath, lang), nil
}

// extractReferences extracts references using a tree-sitter query.
func (p *Parser) extractReferences(query *tree_sitter.Query, root *tree_sitter.Node, content []byte, filePath, lang string) []*Reference {
	var refs []*Reference
	seen := make(map[string]bool) // Dedupe by position

	// Pre-split content into lines for efficient per-reference context extraction
	contentLines := strings.Split(string(content), "\n")

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	// Build capture name index
	captureNames := query.CaptureNames()
	nameIndex := -1
	refIndexes := make(map[uint32]string) // capture index -> kind (call, type)

	for i, captureName := range captureNames {
		if captureName == "name" {
			nameIndex = i
		} else if strings.HasPrefix(captureName, "reference.") {
			kind := strings.TrimPrefix(captureName, "reference.")
			refIndexes[uint32(i)] = kind
		}
	}

	matches := cursor.Matches(query, root, content)
	for match := matches.Next(); match != nil; match = matches.Next() {
		var name string
		var refNode *tree_sitter.Node
		var kind string

		for _, capture := range match.Captures {
			if int(capture.Index) == nameIndex {
				name = capture.Node.Utf8Text(content)
				if refNode == nil {
					node := capture.Node
					refNode = &node
				}
			}
			if k, ok := refIndexes[capture.Index]; ok {
				node := capture.Node
				refNode = &node
				kind = k
			}
		}

		if name == "" || refNode == nil {
			continue
		}

		// Get position
		startPoint := refNode.StartPosition()
		line := int(startPoint.Row) + 1
		col := int(startPoint.Column)

		// Dedupe by position
		key := fmt.Sprintf("%s:%d:%d:%s", filePath, line, col, name)
		if seen[key] {
			continue
		}
		seen[key] = true

		// Map kind to our constants
		refKind := mapRefKind(kind)

		// Extract context (the line of code)
		lineContext := p.extractLineContext(contentLines, int(startPoint.Row))

		ref := &Reference{
			ID:         ulid.Make().String(),
			SymbolName: name,
			Kind:       refKind,
			FilePath:   filePath,
			Line:       line,
			Column:     col,
			Context:    lineContext,
			Language:   lang,
			CreatedAt:  time.Now(),
		}
		refs = append(refs, ref)
	}

	return refs
}

// mapRefKind maps tree-sitter reference kinds to our reference kinds.
func mapRefKind(queryKind string) string {
	switch queryKind {
	case "call":
		return RefKindCall
	case "type":
		return RefKindTypeRef
	case "import":
		return RefKindImport
	default:
		return RefKindCall
	}
}

// extractLineContext extracts the line of code at the given row.
func (p *Parser) extractLineContext(lines []string, row int) string {
	if row >= 0 && row < len(lines) {
		line := strings.TrimSpace(lines[row])
		// Truncate if too long
		if len(line) > 120 {
			line = line[:120] + "..."
		}
		return line
	}
	return ""
}
