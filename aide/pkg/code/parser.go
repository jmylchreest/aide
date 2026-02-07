// Package code provides code indexing and symbol extraction.
package code

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/elm"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/groovy"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/ocaml"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

// Parser extracts symbols from source code using tree-sitter queries.
type Parser struct {
	languages  map[string]*sitter.Language
	queries    map[string]*sitter.Query // Compiled tag queries per language
	refQueries map[string]*sitter.Query // Compiled reference queries per language
}

// NewParser creates a new code parser with supported languages.
func NewParser() *Parser {
	langs := map[string]*sitter.Language{
		// Primary languages
		LangTypeScript: typescript.GetLanguage(),
		LangJavaScript: javascript.GetLanguage(),
		LangGo:         golang.GetLanguage(),
		LangPython:     python.GetLanguage(),
		// Systems languages
		LangRust:   rust.GetLanguage(),
		LangC:      c.GetLanguage(),
		LangCPP:    cpp.GetLanguage(),
		LangCSharp: csharp.GetLanguage(),
		// JVM languages
		LangJava:   java.GetLanguage(),
		LangKotlin: kotlin.GetLanguage(),
		LangScala:  scala.GetLanguage(),
		LangGroovy: groovy.GetLanguage(),
		// Scripting languages
		LangRuby:   ruby.GetLanguage(),
		LangPHP:    php.GetLanguage(),
		LangLua:    lua.GetLanguage(),
		LangElixir: elixir.GetLanguage(),
		LangBash:   bash.GetLanguage(),
		// Apple/Mobile
		LangSwift: swift.GetLanguage(),
		// Functional
		LangOCaml: ocaml.GetLanguage(),
		LangElm:   elm.GetLanguage(),
		// Data/Config
		LangSQL:      sql.GetLanguage(),
		LangYAML:     yaml.GetLanguage(),
		LangTOML:     toml.GetLanguage(),
		LangHCL:      hcl.GetLanguage(),
		LangProtobuf: protobuf.GetLanguage(),
		// Web
		LangHTML: html.GetLanguage(),
		LangCSS:  css.GetLanguage(),
	}

	// Compile queries for each language
	queries := make(map[string]*sitter.Query)
	for lang, sitterLang := range langs {
		if pattern, ok := TagQueries[lang]; ok {
			q, err := sitter.NewQuery([]byte(pattern), sitterLang)
			if err == nil {
				queries[lang] = q
			}
		}
	}

	// Compile reference queries for each language
	refQueries := make(map[string]*sitter.Query)
	for lang, sitterLang := range langs {
		if pattern, ok := RefQueries[lang]; ok {
			q, err := sitter.NewQuery([]byte(pattern), sitterLang)
			if err == nil {
				refQueries[lang] = q
			}
		}
	}

	return &Parser{
		languages:  langs,
		queries:    queries,
		refQueries: refQueries,
	}
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
	// Detect language from extension
	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := LangExtensions[ext]
	if !ok {
		return nil, nil // Unsupported language, skip silently
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return p.ParseContent(content, lang, filePath)
}

// ParseContent parses source code and extracts symbols.
func (p *Parser) ParseContent(content []byte, lang, filePath string) ([]*Symbol, error) {
	language, ok := p.languages[lang]
	if !ok {
		return nil, nil // Unsupported language
	}

	// Create parser
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(language)

	// Parse content
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil || tree == nil {
		return nil, nil
	}
	defer tree.Close()

	// Try query-based extraction first (preferred)
	if query, ok := p.queries[lang]; ok {
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
func (p *Parser) extractWithQuery(query *sitter.Query, root *sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol
	seen := make(map[string]bool) // Dedupe by position

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(query, root)

	// Build capture name index
	nameIndex := -1
	defIndexes := make(map[uint32]string) // capture index -> kind (function, class, etc.)

	for i := uint32(0); i < query.CaptureCount(); i++ {
		captureName := query.CaptureNameForId(i)
		if captureName == "name" {
			nameIndex = int(i)
		} else if strings.HasPrefix(captureName, "definition.") {
			kind := strings.TrimPrefix(captureName, "definition.")
			defIndexes[i] = kind
		}
	}

	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		var name string
		var defNode *sitter.Node
		var kind string

		for _, capture := range match.Captures {
			if int(capture.Index) == nameIndex {
				name = capture.Node.Content(content)
			}
			if k, ok := defIndexes[capture.Index]; ok {
				defNode = capture.Node
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
			ID:        ulid.Make().String(),
			Name:      name,
			Kind:      symbolKind,
			Signature: p.extractSignature(defNode, content),
			FilePath:  filePath,
			StartLine: int(defNode.StartPoint().Row) + 1,
			EndLine:   int(defNode.EndPoint().Row) + 1,
			Language:  lang,
			CreatedAt: time.Now(),
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
func (p *Parser) extractTypeScript(root *sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol

	// Walk the tree looking for relevant nodes
	p.walkNode(root, func(node *sitter.Node) bool {
		nodeType := node.Type()
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
func (p *Parser) extractGo(root *sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	p.walkNode(root, func(node *sitter.Node) bool {
		nodeType := node.Type()
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
func (p *Parser) extractPython(root *sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	p.walkNode(root, func(node *sitter.Node) bool {
		nodeType := node.Type()
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
func (p *Parser) walkNode(node *sitter.Node, fn func(*sitter.Node) bool) {
	if node == nil {
		return
	}

	if !fn(node) {
		return
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		p.walkNode(child, fn)
	}
}

// Helper functions for TypeScript/JavaScript extraction

func (p *Parser) extractTSFunction(node *sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindFunction,
		Signature:  p.extractSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractTSMethod(node *sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindMethod,
		Signature:  p.extractSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractTSClass(node *sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Extract class signature (class name extends X implements Y)
	sig := p.extractClassSignature(node, content)

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindClass,
		Signature:  sig,
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractTSInterface(node *sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindInterface,
		Signature:  p.extractInterfaceSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractTSTypeAlias(node *sitter.Node, content []byte, filePath, lang string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindType,
		Signature:  p.extractTypeSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   lang,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractTSVariableDeclaration(node *sitter.Node, content []byte, filePath, lang string) []*Symbol {
	var symbols []*Symbol

	// Look for arrow functions or function expressions assigned to variables
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode != nil && valueNode != nil {
				valType := valueNode.Type()
				if valType == "arrow_function" || valType == "function_expression" {
					symbols = append(symbols, &Symbol{
						ID:         ulid.Make().String(),
						Name:       p.nodeText(nameNode, content),
						Kind:       KindFunction,
						Signature:  p.extractArrowSignature(child, content),
						DocComment: p.extractPrecedingComment(node, content),
						FilePath:   filePath,
						StartLine:  int(node.StartPoint().Row) + 1,
						EndLine:    int(node.EndPoint().Row) + 1,
						Language:   lang,
						CreatedAt:  time.Now(),
					})
				}
			}
		}
	}

	return symbols
}

// Helper functions for Go extraction

func (p *Parser) extractGoFunction(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindFunction,
		Signature:  p.extractGoFuncSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   LangGo,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractGoMethod(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindMethod,
		Signature:  p.extractGoFuncSignature(node, content),
		DocComment: p.extractPrecedingComment(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   LangGo,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractGoTypeDecl(node *sitter.Node, content []byte, filePath string) []*Symbol {
	var symbols []*Symbol

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")
			if nameNode != nil {
				kind := KindType
				if typeNode != nil {
					switch typeNode.Type() {
					case "struct_type":
						kind = KindClass // Use class for structs
					case "interface_type":
						kind = KindInterface
					}
				}

				symbols = append(symbols, &Symbol{
					ID:         ulid.Make().String(),
					Name:       p.nodeText(nameNode, content),
					Kind:       kind,
					Signature:  p.extractGoTypeSignature(child, content),
					DocComment: p.extractPrecedingComment(node, content),
					FilePath:   filePath,
					StartLine:  int(child.StartPoint().Row) + 1,
					EndLine:    int(child.EndPoint().Row) + 1,
					Language:   LangGo,
					CreatedAt:  time.Now(),
				})
			}
		}
	}

	return symbols
}

// Helper functions for Python extraction

func (p *Parser) extractPythonFunction(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Check if this is a method (inside a class)
	kind := KindFunction
	if p.isInsideClass(node) {
		kind = KindMethod
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       kind,
		Signature:  p.extractPythonFuncSignature(node, content),
		DocComment: p.extractPythonDocstring(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   LangPython,
		CreatedAt:  time.Now(),
	}
}

func (p *Parser) extractPythonClass(node *sitter.Node, content []byte, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &Symbol{
		ID:         ulid.Make().String(),
		Name:       p.nodeText(nameNode, content),
		Kind:       KindClass,
		Signature:  p.extractPythonClassSignature(node, content),
		DocComment: p.extractPythonDocstring(node, content),
		FilePath:   filePath,
		StartLine:  int(node.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Language:   LangPython,
		CreatedAt:  time.Now(),
	}
}

// Utility functions

func (p *Parser) nodeText(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if int(end) > len(content) {
		end = uint32(len(content))
	}
	return string(content[start:end])
}

func (p *Parser) extractSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractClassSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractInterfaceSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractTypeSignature(node *sitter.Node, content []byte) string {
	// For type aliases, extract the full declaration
	text := p.nodeText(node, content)
	// Truncate if too long
	if len(text) > 200 {
		text = text[:200] + "..."
	}
	return text
}

func (p *Parser) extractArrowSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractGoFuncSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractGoTypeSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractPythonFuncSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractPythonClassSignature(node *sitter.Node, content []byte) string {
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

func (p *Parser) extractPrecedingComment(node *sitter.Node, content []byte) string {
	// Look for comment nodes before this node
	prev := node.PrevSibling()
	if prev == nil {
		return ""
	}

	nodeType := prev.Type()
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

func (p *Parser) extractPythonDocstring(node *sitter.Node, content []byte) string {
	// Python docstrings are the first expression in the body
	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return ""
	}

	// Check the first statement for a docstring
	if bodyNode.ChildCount() > 0 {
		child := bodyNode.Child(0)
		if child.Type() == "expression_statement" {
			for j := 0; j < int(child.ChildCount()); j++ {
				expr := child.Child(j)
				if expr.Type() == "string" {
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

func (p *Parser) isInsideClass(node *sitter.Node) bool {
	parent := node.Parent()
	for parent != nil {
		if parent.Type() == "class_definition" || parent.Type() == "class_declaration" {
			return true
		}
		parent = parent.Parent()
	}
	return false
}

// SupportedLanguage returns true if the language is supported.
func (p *Parser) SupportedLanguage(lang string) bool {
	_, ok := p.languages[lang]
	return ok
}

// SupportedExtension returns true if the file extension is supported.
func SupportedExtension(ext string) bool {
	_, ok := LangExtensions[strings.ToLower(ext)]
	return ok
}

// GetLanguageForFile returns the language for a file path, or empty string if unsupported.
func GetLanguageForFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	return LangExtensions[ext]
}

// ParseFileReferences parses a file and extracts references (call sites).
func (p *Parser) ParseFileReferences(filePath string) ([]*Reference, error) {
	// Detect language from extension
	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := LangExtensions[ext]
	if !ok {
		return nil, nil // Unsupported language, skip silently
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return p.ParseContentReferences(content, lang, filePath)
}

// ParseContentReferences parses source code and extracts references.
func (p *Parser) ParseContentReferences(content []byte, lang, filePath string) ([]*Reference, error) {
	language, ok := p.languages[lang]
	if !ok {
		return nil, nil // Unsupported language
	}

	query, ok := p.refQueries[lang]
	if !ok {
		return nil, nil // No reference query for this language
	}

	// Create parser
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(language)

	// Parse content
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil || tree == nil {
		return nil, nil
	}
	defer tree.Close()

	return p.extractReferences(query, tree.RootNode(), content, filePath, lang), nil
}

// extractReferences extracts references using a tree-sitter query.
func (p *Parser) extractReferences(query *sitter.Query, root *sitter.Node, content []byte, filePath, lang string) []*Reference {
	var refs []*Reference
	seen := make(map[string]bool) // Dedupe by position

	// Pre-split content into lines for efficient per-reference context extraction
	contentLines := strings.Split(string(content), "\n")

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(query, root)

	// Build capture name index
	nameIndex := -1
	refIndexes := make(map[uint32]string) // capture index -> kind (call, type)

	for i := uint32(0); i < query.CaptureCount(); i++ {
		captureName := query.CaptureNameForId(i)
		if captureName == "name" {
			nameIndex = int(i)
		} else if strings.HasPrefix(captureName, "reference.") {
			kind := strings.TrimPrefix(captureName, "reference.")
			refIndexes[i] = kind
		}
	}

	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		var name string
		var refNode *sitter.Node
		var kind string

		for _, capture := range match.Captures {
			if int(capture.Index) == nameIndex {
				name = capture.Node.Content(content)
				if refNode == nil {
					refNode = capture.Node
				}
			}
			if k, ok := refIndexes[capture.Index]; ok {
				refNode = capture.Node
				kind = k
			}
		}

		if name == "" || refNode == nil {
			continue
		}

		// Get position
		startPoint := refNode.StartPoint()
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
		context := p.extractLineContext(contentLines, int(startPoint.Row))

		ref := &Reference{
			ID:         ulid.Make().String(),
			SymbolName: name,
			Kind:       refKind,
			FilePath:   filePath,
			Line:       line,
			Column:     col,
			Context:    context,
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
