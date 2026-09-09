package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/observe"
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
	Symbol    string   `json:"symbol" jsonschema:"Name of the symbol to read (e.g., 'getUserById', 'AuthConfig'). Required if symbols is empty."`
	Symbols   []string `json:"symbols,omitempty" jsonschema:"Batch mode: list of symbol names to read (max 10). If set, symbol is ignored."`
	Kind      string   `json:"kind,omitempty" jsonschema:"Filter by symbol kind: function, method, class, interface, type"`
	File      string   `json:"file,omitempty" jsonschema:"Exact file path. Reads current source directly, including files not yet indexed. Use to resolve ambiguous names."`
	StartLine int      `json:"start_line,omitempty" jsonschema:"Current definition start line, used with file to resolve duplicate names within a file."`
}

// ============================================================================
// Code tool registration and handlers
// ============================================================================

func (s *MCPServer) registerCodeTools() {
	mcpLog.Printf("code tools: registered (store may initialize lazily)")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_search",
		Description: `Search indexed code symbol DEFINITIONS (functions, methods, classes, interfaces, types).

Use this to locate implementation candidates by name or signature during debugging,
review, or refactoring. For callers/change impact use code_references; after finding
names, batch code_read_symbol with symbols (max 10) to inspect current source.

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

Results are best-effort indexed candidates, not exhaustive or necessarily current.
Verify relevant definitions in current source; empty results do not prove absence.
If indexing appears unavailable or stale, inspect code_stats or run 'aide code index'.`,
	}, s.handleCodeSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_symbols",
		Description: `List parsed symbol definitions in a specific file.

Returns supported symbols (functions, methods, classes, interfaces, types)
with their signatures, line numbers, and doc comments.

Use this for a file's API surface; use code_outline for a collapsed structural view.
Then read selected bodies with code_read_symbol (batch symbols, max 10, with file).
Read the file directly when it is small or most of its contents are needed.
If the index is missing or stale, the file is parsed on demand; coverage depends on grammar support.`,
	}, s.handleCodeSymbols)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_stats",
		Description: `Get code index statistics.

Returns the number of indexed files, symbols, and references.
Use this when troubleshooting missing results or indexing status; it is not a
required preflight before each search or source read.

Zero counts can mean no supported files are indexed. Run 'aide code index' when
indexing is needed; counts alone do not establish freshness or complete coverage.`,
	}, s.handleCodeStats)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_references",
		Description: `Find indexed reference candidates (call sites and type uses) by symbol name.

**What are references?**
References are places where a symbol is used, indexed by tree-sitter:
- Function/method calls (kind: call)
- Type references (kind: type_ref)

**Use cases:**
- Investigate callers of a function
- Understand how a type is used
- Impact analysis before refactoring — "what breaks if I change this?"

**Batch mode:** Pass multiple names in the "symbols" array (max 10) to find
references for several symbols in a single call.

Results are best-effort name matches from the index, not a complete semantic call graph.
Same-name symbols, dynamic calls, unsupported syntax, stale files and result limits
can affect coverage. Verify relevant callers in current source before concluding
change impact; empty results do not prove no callers exist. Use Grep for literals,
imports, or content patterns. If indexing appears unavailable or stale, inspect
code_stats or run 'aide code index'.`,
	}, s.handleCodeReferences)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_outline",
		Description: `Get a collapsed outline of a file with bodies replaced by { ... }.

Returns the file structure with signatures preserved and function/method/class bodies
collapsed. Output size depends on file structure and grammar support. Line numbers are preserved
so you can later use Read with offset/limit for specific sections.

Use this to locate declarations when you need only part of an unfamiliar file.
Once names are known, batch code_read_symbol with symbols (max 10) and file for
current bodies, or use bounded Read for surrounding context.
For a small file or when most of its contents are needed, a direct read can avoid
outline overhead. An outline does not guarantee lower whole-task token usage.

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
		Description: `Rank symbols by indexed reference count.

Returns symbols sorted by reference count (descending). Each result includes
the symbol name, reference count, and definition location when available.

**Use cases:**
- Find the most-used functions, types, or methods in the codebase
- Identify core APIs and shared utilities
- Understand codebase coupling — heavily-referenced symbols are high-impact change targets

Counts are best-effort indexed name matches, not complete usage or semantic impact.
Verify candidates in current source. If indexing appears unavailable or stale,
inspect code_stats or run 'aide code index'.`,
	}, s.handleCodeTopReferences)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_read_symbol",
		Description: `Read the full source code of a symbol by name — without reading the entire file.

Returns the complete source (signature + body) for a function, method, class, or type,
extracted from current file contents. A focused symbol read can reduce returned text
for large files; headers and additional calls can outweigh that reduction for small files.

**Batch mode:** Pass multiple names in the "symbols" array (max 10) to read several
symbols in a single call, avoiding separate calls per symbol.
When names are unknown, use code_search for definition candidates or code_symbols /
code_outline for a known file. Read directly when the file is small or most of it
is needed; use bounded Read for imports or surrounding context outside the symbol.

**What you get:**
- The symbol's source code with line numbers preserved
- File path and line range for navigation
- Doc comment if present
- Source-version receipt in protocol metadata for conditional text comparisons

**Use this when:**
- You know the symbol name (from code_search, code_symbols, code_outline, or code_references)
- You need to read the implementation of a specific function
- You want to review a class or type definition
- You need several symbol bodies at once (use batch mode)

**Example:**
- Single: {"symbol": "getUserById"}
- Batch: {"symbols": ["getUserById", "createUser", "deleteUser"]}
- Filtered: {"symbol": "handle", "kind": "method"}

**Disambiguation:** Set file to read from an exact path; this also works without an index.
If a file has multiple definitions with the same name, also set start_line to the
current definition line. Ambiguous names return candidates instead of choosing one.
Without file, uses the code index to locate candidate files, then reads current source.`,
	}, s.handleCodeReadSymbol)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "code_read_check",
		Description: `Check if a file is indexed and whether its current mtime matches the index.

Returns freshness status so you can decide whether to re-read a file or use
code_outline/code_symbols/code_references instead.

**Response fields:**
- indexed: whether the file exists in the code index
- fresh: whether a current regular file has the indexed mtime
- symbols: number of symbols indexed for this file
- outline_available: whether code_outline would return useful data
- text_estimate: current regular-file stat bytes, estimated_tokens, and estimator
  (utf8-bytes/3-v1); null when the file is unindexed or its size is unavailable
- estimated_tokens: compatibility integer alias; zero when unknown or outside int32

This checks index modification times, not the version previously delivered to
the agent. A matching timestamp does not prove unchanged content or prior coverage.`,
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
	root := store.ProjectRootFromDB(s.dbPath)
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
				if err == nil && completeFileSymbols(fileInfo, symbols) {
					return symbols, nil
				}
			}
		}
	}

	// Index is stale, missing, or unavailable — parse on demand
	mcpLog.Printf("  freshness: parsing %s on demand", relPath)
	parser := code.NewParser(s.grammarLoader)
	defer parser.Close()
	return parser.ParseFile(absPath)
}

// A file record can outlive its symbol records. Never treat a partial index as
// evidence that a file has no more definitions.
func completeFileSymbols(info *code.FileInfo, symbols []*code.Symbol) bool {
	if len(info.SymbolIDs) != len(symbols) {
		return false
	}
	ids := make(map[string]bool, len(symbols))
	for _, sym := range symbols {
		if sym == nil {
			return false
		}
		ids[sym.ID] = true
	}
	for _, id := range info.SymbolIDs {
		if !ids[id] {
			return false
		}
	}
	return true
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

func (s *MCPServer) handleCodeOutline(ctx context.Context, _ *mcp.CallToolRequest, input CodeOutlineInput) (*mcp.CallToolResult, any, error) {
	span := observe.FromContext(ctx).FilePath(input.File)
	mcpLog.Printf("tool: code_outline file=%s keep_comments=%v", input.File, input.KeepComments)

	if input.File == "" {
		return errorResult("file path is required"), nil, nil
	}

	// Parse the same byte snapshot we render. Cached mtimes do not prove that
	// symbol ranges describe these bytes (editors can preserve timestamps).
	snapshot, err := s.readSourceSnapshot(input.File)
	if err != nil {
		mcpLog.Printf("  error getting symbols: %v", err)
		return errorResult(fmt.Sprintf("failed to parse file: %v", err)), nil, nil
	}

	outline := buildOutline(snapshot.content, snapshot.symbols, !input.KeepComments)
	return sourceResult(span, "code_outline", map[string]*sourceSnapshot{snapshot.path: snapshot}, outline), nil, nil
}

func (s *MCPServer) handleCodeReadCheck(_ context.Context, _ *mcp.CallToolRequest, input CodeReadCheckInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: code_read_check file=%s", input.File)

	if input.File == "" {
		return errorResult("file path is required"), nil, nil
	}

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return textResult(`{"indexed":false,"fresh":false,"symbols":0,"outline_available":false,"estimated_tokens":0,"text_estimate":null}`), nil, nil
	}

	root := store.ProjectRootFromDB(s.dbPath)

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
		return textResult(`{"indexed":false,"fresh":false,"symbols":0,"outline_available":false,"estimated_tokens":0,"text_estimate":null}`), nil, nil
	}

	result, err := json.Marshal(code.CheckIndexedFile(absPath, fileInfo))
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	mcpLog.Printf("  result: %s", result)
	return textResult(string(result)), nil, nil
}

func (s *MCPServer) handleCodeReadSymbol(ctx context.Context, _ *mcp.CallToolRequest, input CodeReadSymbolInput) (*mcp.CallToolResult, any, error) {
	span := observe.FromContext(ctx)
	// Resolve symbol names: batch mode takes precedence
	names := input.Symbols
	if len(names) == 0 && input.Symbol != "" {
		names = []string{input.Symbol}
	}

	mcpLog.Printf("tool: code_read_symbol symbols=%v kind=%s", names, input.Kind)

	codeStore := s.getCodeStore()
	if codeStore == nil && input.File == "" {
		return errorResult("code store not available (still initializing or disabled)"), nil, nil
	}

	if len(names) == 0 {
		return errorResult("symbol name is required (set 'symbol' or 'symbols')"), nil, nil
	}
	if len(names) > 10 {
		return errorResult("batch mode supports at most 10 symbols per call"), nil, nil
	}
	if input.StartLine < 0 || (input.StartLine > 0 && input.File == "") {
		return errorResult("start_line must be non-negative and requires file"), nil, nil
	}

	var sb strings.Builder
	if len(names) > 1 {
		sb.WriteString("# Batch Symbol Source\n\n")
	}

	found := 0
	var single *code.Symbol
	// One immutable snapshot per file for the entire batch; each full-file
	// reference appears once even when several definitions are requested.
	snapshots := make(map[string]*sourceSnapshot)
	references := make(map[string]*sourceSnapshot)

	for _, name := range names {
		sym, snapshot, text := s.readOneSymbol(codeStore, name, input, snapshots)
		sb.WriteString(text)
		if sym == nil {
			continue
		}
		found++
		single = sym
		references[snapshot.path] = snapshot
	}

	span.Attr("symbols", fmt.Sprintf("%d/%d", found, len(names)))
	if found == 1 {
		// Single-symbol mode: surface the file path AND symbol body line
		// range on the span so the dashboard's file viewer can scroll
		// straight to the symbol. (Batch mode mixes files; we leave both
		// empty there.)
		span.FilePath(single.FilePath).Attr("start_line", strconv.Itoa(single.StartLine)).Attr("end_line", strconv.Itoa(single.EndLine))
	}

	result := sourceResult(span, "code_read_symbol", references, sb.String())
	result.IsError = found != len(names)
	return result, nil, nil
}
