package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// Code tool input types
// ============================================================================

type CodeSearchInput struct {
	Query    string `json:"query" jsonschema:"Search query for symbol names or signatures. Supports Bleve query syntax."`
	Kind     string `json:"kind,omitempty" jsonschema:"Filter by symbol kind: function, method, class, interface, type"`
	Language string `json:"lang,omitempty" jsonschema:"Filter by language: typescript, javascript, go, python"`
	FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 20)"`
}

type CodeSymbolsInput struct {
	FilePath string `json:"file" jsonschema:"Path to the file to get symbols from"`
}

type CodeStatsInput struct{}

type CodeReferencesInput struct {
	SymbolName string `json:"symbol" jsonschema:"Name of the symbol to find references for (e.g., 'getUserById')"`
	Kind       string `json:"kind,omitempty" jsonschema:"Filter by reference kind: call, type_ref"`
	FilePath   string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum results (default 50)"`
}

// ============================================================================
// Code tool registration and handlers
// ============================================================================

func (s *MCPServer) registerCodeTools() {
	mcpLog.Printf("code tools: registered (store may initialize lazily)")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_search",
		Description: `Search indexed code symbols (functions, methods, classes, interfaces, types).

**What gets indexed?**
Symbols are extracted from source files using tree-sitter parsing:
- Functions and methods with their signatures
- Classes and interfaces
- Type definitions
- Includes doc comments when present

**Search capabilities:**
- Full-text search on symbol names and signatures
- Filter by kind (function, method, class, interface, type)
- Filter by language (typescript, javascript, go, python)
- Filter by file path pattern

**Note:** Run 'aide code index' to index your codebase first.`,
	}, s.handleCodeSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_symbols",
		Description: `List all symbols in a specific file.

Returns all indexed symbols (functions, methods, classes, etc.) from the given file.
If the file isn't indexed yet, it will be parsed on-demand.`,
	}, s.handleCodeSymbols)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_stats",
		Description: `Get code index statistics.

Returns the number of indexed files, symbols, and references.
Use this to check if the codebase has been indexed.`,
	}, s.handleCodeStats)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_references",
		Description: `Find all references (call sites) for a symbol.

**What are references?**
References are places where a symbol is used:
- Function/method calls
- Type references
- Constructor invocations

**Use cases:**
- Find all callers of a function
- Understand how a type is used
- Impact analysis before refactoring

**Note:** Run 'aide code index' to index your codebase first.`,
	}, s.handleCodeReferences)
}

func (s *MCPServer) handleCodeSearch(_ context.Context, _ *mcp.CallToolRequest, input CodeSearchInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_search query=%q kind=%s lang=%s", input.Query, input.Kind, input.Language)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	// Auto-wrap simple queries with wildcards for substring matching
	query := input.Query
	if query != "" && !containsBleveSyntax(query) {
		query = "*" + query + "*"
		mcpLog.Printf("  auto-wildcarded query: %q", query)
	}

	opts := code.SearchOptions{
		Kind:     input.Kind,
		Language: input.Language,
		FilePath: input.FilePath,
		Limit:    limit,
	}

	results, err := codeStore.SearchSymbols(query, opts)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d symbols", len(results))
	return textResult(formatCodeSearchResults(results)), nil, nil
}

func (s *MCPServer) handleCodeSymbols(_ context.Context, _ *mcp.CallToolRequest, input CodeSymbolsInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_symbols file=%s", input.FilePath)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	symbols, err := codeStore.GetFileSymbols(input.FilePath)
	if err != nil {
		// If file not in index, try to parse it directly
		parser := code.NewParser()
		symbols, err = parser.ParseFile(input.FilePath)
		if err != nil {
			mcpLog.Printf("  error: %v", err)
			return errorResult(fmt.Sprintf("failed to get symbols: %v", err)), nil, nil
		}
	}

	mcpLog.Printf("  found: %d symbols", len(symbols))
	return textResult(formatCodeSymbols(input.FilePath, symbols)), nil, nil
}

func (s *MCPServer) handleCodeStats(_ context.Context, _ *mcp.CallToolRequest, _ CodeStatsInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_stats")

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	stats, err := codeStore.Stats()
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("failed to get stats: %v", err)), nil, nil
	}

	mcpLog.Printf("  files=%d symbols=%d references=%d", stats.Files, stats.Symbols, stats.References)
	return textResult(fmt.Sprintf("Code Index Statistics:\n- Files indexed: %d\n- Symbols indexed: %d\n- References indexed: %d", stats.Files, stats.Symbols, stats.References)), nil, nil
}

func (s *MCPServer) handleCodeReferences(_ context.Context, _ *mcp.CallToolRequest, input CodeReferencesInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_references symbol=%q kind=%s file=%s", input.SymbolName, input.Kind, input.FilePath)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	if input.SymbolName == "" {
		return errorResult("symbol name is required"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	opts := code.ReferenceSearchOptions{
		SymbolName: input.SymbolName,
		Kind:       input.Kind,
		FilePath:   input.FilePath,
		Limit:      limit,
	}

	refs, err := codeStore.SearchReferences(opts)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d references", len(refs))
	return textResult(formatCodeReferences(input.SymbolName, refs)), nil, nil
}

// ============================================================================
// Code formatting helpers
// ============================================================================

func formatCodeSearchResults(results []*store.CodeSearchResult) string {
	if len(results) == 0 {
		return "No matching symbols found.\n\nTip: Run `aide code index` to index your codebase."
	}

	var sb strings.Builder
	sb.WriteString("# Code Search Results\n\n")

	for _, r := range results {
		sym := r.Symbol
		fmt.Fprintf(&sb, "## `%s` [%s]\n", sym.Name, sym.Kind)
		fmt.Fprintf(&sb, "**File:** `%s:%d`\n", sym.FilePath, sym.StartLine)
		fmt.Fprintf(&sb, "**Signature:** `%s`\n", sym.Signature)
		if sym.DocComment != "" {
			fmt.Fprintf(&sb, "**Doc:** %s\n", sym.DocComment)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// containsBleveSyntax checks if a query contains Bleve query syntax characters.
func containsBleveSyntax(query string) bool {
	special := []string{"*", "?", "+", "-", ":", "^", "~", "(", ")", "[", "]", "{", "}", "\"", "&&", "||", "!"}
	for _, s := range special {
		if strings.Contains(query, s) {
			return true
		}
	}
	return false
}

func formatCodeSymbols(filePath string, symbols []*code.Symbol) string {
	if len(symbols) == 0 {
		return fmt.Sprintf("No symbols found in `%s`", filePath)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Symbols in `%s`\n\n", filePath)
	fmt.Fprintf(&sb, "_Total: %d symbols_\n\n", len(symbols))

	grouped := make(map[string][]*code.Symbol)
	for _, sym := range symbols {
		grouped[sym.Kind] = append(grouped[sym.Kind], sym)
	}

	kindOrder := []string{"interface", "class", "type", "function", "method"}
	for _, kind := range kindOrder {
		syms := grouped[kind]
		if len(syms) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "## %ss\n\n", titleCase(kind))
		for _, sym := range syms {
			fmt.Fprintf(&sb, "- **%s** (line %d): `%s`\n", sym.Name, sym.StartLine, sym.Signature)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatCodeReferences(symbolName string, refs []*code.Reference) string {
	if len(refs) == 0 {
		return fmt.Sprintf("No references found for `%s`.\n\nTip: Run `aide code index` to index your codebase.", symbolName)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# References to `%s`\n\n", symbolName)
	fmt.Fprintf(&sb, "_Found %d references_\n\n", len(refs))

	grouped := make(map[string][]*code.Reference)
	for _, ref := range refs {
		grouped[ref.FilePath] = append(grouped[ref.FilePath], ref)
	}

	for filePath, fileRefs := range grouped {
		fmt.Fprintf(&sb, "## `%s`\n\n", filePath)
		for _, ref := range fileRefs {
			kindTag := ""
			switch ref.Kind {
			case code.RefKindCall:
				kindTag = "[call]"
			case code.RefKindTypeRef:
				kindTag = "[type]"
			}
			fmt.Fprintf(&sb, "- **Line %d** %s: `%s`\n", ref.Line, kindTag, ref.Context)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
