package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/memory"
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
	SymbolName  string   `json:"symbol" jsonschema:"Name of the symbol to find references for (e.g., 'getUserById'). Required if symbols is empty."`
	SymbolNames []string `json:"symbols,omitempty" jsonschema:"Batch mode: list of symbol names to find references for (max 10). If set, symbol is ignored."`
	Kind        string   `json:"kind,omitempty" jsonschema:"Filter by reference kind: call, type_ref"`
	FilePath    string   `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Maximum results per symbol (default 50)"`
}

type CodeOutlineInput struct {
	File         string `json:"file" jsonschema:"Path to the file to outline. Required."`
	KeepComments bool   `json:"keep_comments,omitempty" jsonschema:"Keep comments in output. By default comments are stripped to minimize tokens."`
}

type CodeTopReferencesInput struct {
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results (default 25)"`
	Kind  string `json:"kind,omitempty" jsonschema:"Filter by symbol kind: function, method, class, interface, type"`
}

type CodeReadCheckInput struct {
	File string `json:"file" jsonschema:"Path to the file to check (relative or absolute). Required."`
}

type CodeReadSymbolInput struct {
	Symbol  string   `json:"symbol" jsonschema:"Name of the symbol to read (e.g., 'getUserById', 'AuthConfig'). Required if symbols is empty."`
	Symbols []string `json:"symbols,omitempty" jsonschema:"Batch mode: list of symbol names to read (max 10). If set, symbol is ignored."`
	Kind    string   `json:"kind,omitempty" jsonschema:"Filter by symbol kind: function, method, class, interface, type"`
}

// ============================================================================
// Code tool registration and handlers
// ============================================================================

func (s *MCPServer) registerCodeTools() {
	mcpLog.Printf("code tools: registered (store may initialize lazily)")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_search",
		Description: `Search indexed code symbol DEFINITIONS (functions, methods, classes, interfaces, types).

**What gets indexed?**
Symbols are extracted from source files using tree-sitter parsing:
- Functions and methods with their signatures
- Classes and interfaces
- Type definitions
- Doc comments (searchable)

**Search capabilities:**
- Full-text search on symbol names, signatures, and doc comments
- Filter by kind (function, method, class, interface, type)
- Filter by language (typescript, javascript, go, python)
- Filter by file path pattern

**What is NOT indexed** (use Grep for these):
- Code inside function bodies (loops, conditionals, error handling)
- Method call chains (.map, .forEach, .filter)
- String literals, SQL queries, error messages
- Import/require statements
- Variable declarations

**Note:** Run 'aide code index' to index your codebase first.`,
	}, s.handleCodeSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_symbols",
		Description: `List all symbols defined in a specific file.

Returns all indexed symbols (functions, methods, classes, interfaces, types)
with their signatures, line numbers, and doc comments.

Use this to understand a file's API surface without reading the entire file.
If the file isn't indexed yet, it will be parsed on-demand.`,
	}, s.handleCodeSymbols)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_stats",
		Description: `Get code index statistics.

Returns the number of indexed files, symbols, and references.
Use this to check if the codebase has been indexed.

If counts are zero, the codebase needs indexing: run 'aide code index'.`,
	}, s.handleCodeStats)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_references",
		Description: `Find all references (call sites) for a symbol.

**What are references?**
References are places where a symbol is used, indexed by tree-sitter:
- Function/method calls (kind: call)
- Type references (kind: type_ref)

**Use cases:**
- Find all callers of a function
- Understand how a type is used
- Impact analysis before refactoring — "what breaks if I change this?"

**Batch mode:** Pass multiple names in the "symbols" array (max 10) to find
references for several symbols in a single call.

**Note:** Run 'aide code index' to index your codebase first.`,
	}, s.handleCodeReferences)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_outline",
		Description: `Get a collapsed outline of a file with bodies replaced by { ... }.

Returns the file structure with signatures preserved and function/method/class bodies
collapsed, showing ~5-15% of the tokens of the full file. Line numbers are preserved
so you can later use Read with offset/limit for specific sections.

**Use this BEFORE reading a file** to understand its structure, then read only the
sections you need. This dramatically reduces context window usage.

By default, comments are stripped. Set keep_comments=true to preserve them.

**Example output:**
` + "```" + `
1:  package auth
3:  type UserRole string
5:  type AuthConfig struct { ... }                           // 5-12
14: func Authenticate(token string) (*User, error) { ... }   // 14-45
47: func validateToken(token string) bool { ... }             // 47-62
` + "```" + ``,
	}, s.handleCodeOutline)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_top_references",
		Description: `Rank symbols by how many times they are referenced across the codebase.

Returns symbols sorted by reference count (descending). Each result includes
the symbol name, reference count, and definition location when available.

**Use cases:**
- Find the most-used functions, types, or methods in the codebase
- Identify core APIs and shared utilities
- Understand codebase coupling — heavily-referenced symbols are high-impact change targets

**Note:** Run 'aide code index' to index your codebase first.`,
	}, s.handleCodeTopReferences)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_read_symbol",
		Description: `Read the full source code of a symbol by name — without reading the entire file.

Returns the complete source (signature + body) for a function, method, class, or type,
extracted from the indexed file using the symbol's known line range. This is dramatically
cheaper than reading the whole file when you only need one symbol.

**Batch mode:** Pass multiple names in the "symbols" array (max 10) to read several
symbols in a single call, eliminating round-trip overhead.

**What you get:**
- The symbol's source code with line numbers preserved
- File path and line range for navigation
- Doc comment if present
- Estimated token savings vs reading the full file

**Use this when:**
- You know the symbol name (from code_search, code_outline, or code_references)
- You need to read the implementation of a specific function
- You want to review a class or type definition
- You need several symbol bodies at once (use batch mode)

**Example:**
- Single: {"symbol": "getUserById"}
- Batch: {"symbols": ["getUserById", "createUser", "deleteUser"]}
- Filtered: {"symbol": "handle", "kind": "method"}

**Note:** Requires the code index (run 'aide code index'). If the symbol isn't found
in the index, check the name with code_search first.`,
	}, s.handleCodeReadSymbol)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_read_check",
		Description: `Check if a file is indexed and whether its content has changed since indexing.

Returns freshness status so you can decide whether to re-read a file or use
code_outline/code_symbols/code_references instead.

**Response fields:**
- indexed: whether the file exists in the code index
- fresh: whether the file hasn't changed since last indexing (mtime match)
- symbols: number of symbols indexed for this file
- outline_available: whether code_outline would return useful data
- estimated_tokens: estimated token count for the full file (calibrated per-language)

**Use this before re-reading a file** to check if the version you already read
is still current. If fresh=true and outline_available=true, prefer code_outline
or code_symbols over a full Read to save context window tokens.`,
	}, s.handleCodeReadCheck)
}

func (s *MCPServer) handleCodeSearch(_ context.Context, _ *mcp.CallToolRequest, input CodeSearchInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_search query=%q kind=%s lang=%s", input.Query, input.Kind, input.Language)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = DefaultCodeSearchLimit
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

	symbols, err := s.getFileSymbolsFresh(input.FilePath)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("failed to get symbols: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d symbols", len(symbols))
	return textResult(formatCodeSymbols(input.FilePath, symbols)), nil, nil
}

// getFileSymbolsFresh returns symbols for a file, checking freshness against disk.
// If the index is stale or missing, it falls back to live tree-sitter parsing.
func (s *MCPServer) getFileSymbolsFresh(filePath string) ([]*code.Symbol, error) {
	root := projectRoot(s.dbPath)
	// Resolve to absolute path for stat, relative for store lookup
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(root, filePath)
	}
	relPath := filePath
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(root, filePath); err == nil {
			relPath = rel
		}
	}

	codeStore := s.getCodeStore()
	if codeStore != nil {
		// Check if the indexed data is fresh
		fileInfo, err := codeStore.GetFileInfo(relPath)
		if err == nil {
			stat, statErr := os.Stat(absPath)
			if statErr == nil && fileInfo.ModTime.Equal(stat.ModTime()) {
				// Index is current — use cached symbols
				symbols, err := codeStore.GetFileSymbols(relPath)
				if err == nil {
					return symbols, nil
				}
			}
		}
	}

	// Index is stale, missing, or unavailable — parse on demand
	mcpLog.Printf("  freshness: parsing %s on demand", relPath)
	parser := code.NewParser(s.grammarLoader)
	return parser.ParseFile(absPath)
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
	// Resolve symbol names: batch mode takes precedence
	names := input.SymbolNames
	if len(names) == 0 && input.SymbolName != "" {
		names = []string{input.SymbolName}
	}

	mcpLog.Printf("tool: code_references symbols=%v kind=%s file=%s", names, input.Kind, input.FilePath)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	if len(names) == 0 {
		return errorResult("symbol name is required (set 'symbol' or 'symbols')"), nil, nil
	}
	if len(names) > 10 {
		return errorResult("batch mode supports at most 10 symbols per call"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = DefaultCodeRefsLimit
	}

	// Single-symbol mode: return as before
	if len(names) == 1 {
		opts := code.ReferenceSearchOptions{
			SymbolName: names[0],
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
		return textResult(formatCodeReferences(names[0], refs)), nil, nil
	}

	// Batch mode: query each symbol and combine results
	var sb strings.Builder
	sb.WriteString("# Batch Reference Results\n\n")
	totalRefs := 0
	for _, name := range names {
		opts := code.ReferenceSearchOptions{
			SymbolName: name,
			Kind:       input.Kind,
			FilePath:   input.FilePath,
			Limit:      limit,
		}
		refs, err := codeStore.SearchReferences(opts)
		if err != nil {
			fmt.Fprintf(&sb, "## `%s` — error: %v\n\n", name, err)
			continue
		}
		totalRefs += len(refs)
		sb.WriteString(formatCodeReferences(name, refs))
	}
	mcpLog.Printf("  batch: %d symbols, %d total references", len(names), totalRefs)
	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleCodeTopReferences(_ context.Context, _ *mcp.CallToolRequest, input CodeTopReferencesInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_top_references limit=%d kind=%s", input.Limit, input.Kind)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 25
	}

	results, err := codeStore.TopReferencedSymbols(limit, input.Kind)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("query failed: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("No referenced symbols found. Run 'aide code index' to index the codebase."), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Top %d most-referenced symbols:\n\n", len(results))
	for i, r := range results {
		loc := ""
		if r.File != "" {
			loc = fmt.Sprintf("  %s", r.File)
		}
		kind := ""
		if r.Kind != "" {
			kind = fmt.Sprintf(" [%s]", r.Kind)
		}
		fmt.Fprintf(&sb, "%3d. %-40s %4d refs%s%s\n", i+1, r.Symbol, r.Count, kind, loc)
	}

	mcpLog.Printf("  returned: %d symbols", len(results))
	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleCodeOutline(_ context.Context, _ *mcp.CallToolRequest, input CodeOutlineInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_outline file=%s keep_comments=%v", input.File, input.KeepComments)

	if input.File == "" {
		return errorResult("file path is required"), nil, nil
	}

	// Get fresh symbols with body ranges
	symbols, err := s.getFileSymbolsFresh(input.File)
	if err != nil {
		mcpLog.Printf("  error getting symbols: %v", err)
		return errorResult(fmt.Sprintf("failed to parse file: %v", err)), nil, nil
	}

	// Read the actual file
	absPath := input.File
	if !filepath.IsAbs(input.File) {
		absPath = filepath.Join(projectRoot(s.dbPath), input.File)
	}

	fileContent, err := os.ReadFile(absPath)
	if err != nil {
		mcpLog.Printf("  error reading file: %v", err)
		return errorResult(fmt.Sprintf("failed to read file: %v", err)), nil, nil
	}

	outline := buildOutline(fileContent, symbols, !input.KeepComments)
	mcpLog.Printf("  outline: %d symbols, %d/%d lines", len(symbols), countLines(outline), countLines(string(fileContent)))

	// Record token event: outline used instead of full file read
	fullTokens := code.EstimateTokensFromSize(input.File, int64(len(fileContent)))
	outlineTokens := code.EstimateTokens(input.File, len(outline))
	saved := fullTokens - outlineTokens
	if saved > 0 && s.store != nil {
		s.store.AddTokenEvent(&memory.TokenEvent{
			EventType:   memory.TokenEventOutlineUsed,
			Tool:        "code_outline",
			FilePath:    input.File,
			Tokens:      outlineTokens,
			TokensSaved: saved,
		})
	}

	return textResult(outline), nil, nil
}

func (s *MCPServer) handleCodeReadCheck(_ context.Context, _ *mcp.CallToolRequest, input CodeReadCheckInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_read_check file=%s", input.File)

	if input.File == "" {
		return errorResult("file path is required"), nil, nil
	}

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return textResult(`{"indexed":false,"fresh":false,"symbols":0,"outline_available":false,"estimated_tokens":0}`), nil, nil
	}

	root := projectRoot(s.dbPath)

	// Resolve to absolute path for os.Stat
	absPath := input.File
	if !filepath.IsAbs(input.File) {
		absPath = filepath.Join(root, input.File)
	}

	// Resolve to relative path for store lookup
	relPath := input.File
	if filepath.IsAbs(input.File) {
		if rel, err := filepath.Rel(root, input.File); err == nil {
			relPath = rel
		}
	}

	fileInfo, err := codeStore.GetFileInfo(relPath)
	if err != nil {
		return textResult(`{"indexed":false,"fresh":false,"symbols":0,"outline_available":false,"estimated_tokens":0}`), nil, nil
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		result := fmt.Sprintf(`{"indexed":true,"fresh":false,"symbols":%d,"outline_available":%t,"estimated_tokens":%d}`,
			len(fileInfo.SymbolIDs), len(fileInfo.SymbolIDs) > 0, fileInfo.Tokens)
		return textResult(result), nil, nil
	}

	fresh := fileInfo.ModTime.Equal(stat.ModTime())
	symbolCount := len(fileInfo.SymbolIDs)
	tokens := fileInfo.Tokens
	if tokens == 0 && stat.Size() > 0 {
		tokens = code.EstimateTokensFromSize(relPath, stat.Size())
	}

	result := fmt.Sprintf(`{"indexed":true,"fresh":%t,"symbols":%d,"outline_available":%t,"estimated_tokens":%d}`,
		fresh, symbolCount, symbolCount > 0, tokens)
	mcpLog.Printf("  result: %s", result)
	return textResult(result), nil, nil
}

func (s *MCPServer) handleCodeReadSymbol(_ context.Context, _ *mcp.CallToolRequest, input CodeReadSymbolInput) (*mcp.CallToolResult, any, error) {
	// Resolve symbol names: batch mode takes precedence
	names := input.Symbols
	if len(names) == 0 && input.Symbol != "" {
		names = []string{input.Symbol}
	}

	mcpLog.Printf("tool: code_read_symbol symbols=%v kind=%s", names, input.Kind)

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	if len(names) == 0 {
		return errorResult("symbol name is required (set 'symbol' or 'symbols')"), nil, nil
	}
	if len(names) > 10 {
		return errorResult("batch mode supports at most 10 symbols per call"), nil, nil
	}

	root := projectRoot(s.dbPath)
	var sb strings.Builder
	if len(names) > 1 {
		sb.WriteString("# Batch Symbol Source\n\n")
	}

	totalTokens := 0
	totalSaved := 0
	found := 0

	for _, name := range names {
		sym, symbolTokens, saved, text := s.readOneSymbol(codeStore, root, name, input.Kind)
		sb.WriteString(text)
		if sym == nil {
			continue
		}
		found++
		totalTokens += symbolTokens
		totalSaved += saved

		if saved > 0 && s.store != nil {
			s.store.AddTokenEvent(&memory.TokenEvent{
				EventType:   memory.TokenEventSymbolRead,
				Tool:        "code_read_symbol",
				FilePath:    sym.FilePath,
				Tokens:      symbolTokens,
				TokensSaved: saved,
			})
		}
	}

	mcpLog.Printf("  returned %d/%d symbols, %d tokens, %d saved", found, len(names), totalTokens, totalSaved)
	return textResult(sb.String()), nil, nil
}

// readOneSymbol looks up a symbol by name in the code index and extracts its source lines.
// Returns the matched symbol (nil if not found), token count, tokens saved, and formatted output.
func (s *MCPServer) readOneSymbol(codeStore store.CodeIndexStore, root, name, kind string) (*code.Symbol, int, int, string) {
	query := name
	if !containsBleveSyntax(query) {
		query = "\"" + query + "\""
	}
	opts := code.SearchOptions{
		Kind:  kind,
		Limit: 20,
	}
	results, err := codeStore.SearchSymbols(query, opts)
	if err != nil {
		return nil, 0, 0, fmt.Sprintf("## `%s` — search error: %v\n\n", name, err)
	}

	var match *code.Symbol
	for _, r := range results {
		if r.Symbol.Name == name {
			match = r.Symbol
			break
		}
	}
	if match == nil {
		return nil, 0, 0, fmt.Sprintf("## `%s` — not found in index\n\n", name)
	}

	absPath := filepath.Join(root, match.FilePath)
	symbolLines, err := readFileLines(absPath, match.StartLine, match.EndLine)
	if err != nil {
		return nil, 0, 0, fmt.Sprintf("## `%s` — file error: %v\n\n", name, err)
	}

	if len(symbolLines) == 0 {
		return nil, 0, 0, fmt.Sprintf("## `%s` — no source lines at %s:%d-%d\n\n", name, match.FilePath, match.StartLine, match.EndLine)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## `%s` [%s]\n", match.Name, match.Kind)
	fmt.Fprintf(&sb, "**File:** `%s:%d-%d`", match.FilePath, match.StartLine, match.EndLine)
	if match.Signature != "" {
		fmt.Fprintf(&sb, " | **Signature:** `%s`", match.Signature)
	}
	sb.WriteString("\n")
	if match.DocComment != "" {
		fmt.Fprintf(&sb, "**Doc:** %s\n", match.DocComment)
	}
	sb.WriteString("\n```\n")
	for i, line := range symbolLines {
		fmt.Fprintf(&sb, "%-4d: %s\n", match.StartLine+i, line)
	}
	sb.WriteString("```\n\n")

	symbolContent := strings.Join(symbolLines, "\n")
	symbolTokens := code.EstimateTokens(match.FilePath, len(symbolContent))
	saved := 0
	if stat, err := os.Stat(absPath); err == nil {
		fileTokens := code.EstimateTokensFromSize(match.FilePath, stat.Size())
		if fileTokens > symbolTokens {
			saved = fileTokens - symbolTokens
		}
	}

	return match, symbolTokens, saved, sb.String()
}

