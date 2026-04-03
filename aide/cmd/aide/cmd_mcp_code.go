package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	SymbolName string `json:"symbol" jsonschema:"Name of the symbol to find references for (e.g., 'getUserById')"`
	Kind       string `json:"kind,omitempty" jsonschema:"Filter by reference kind: call, type_ref"`
	FilePath   string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum results (default 50)"`
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
- Impact analysis before refactoring

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
		limit = DefaultCodeRefsLimit
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

// ============================================================================
// Code outline helpers
// ============================================================================

// bodyRange represents a collapsible body region in a file.
type bodyRange struct {
	startLine int // 1-indexed, first line of body (e.g., the { line)
	endLine   int // 1-indexed, last line of body
	symbol    *code.Symbol
}

// commentPattern matches common single-line comment patterns.
var commentPattern = regexp.MustCompile(`^\s*(//|#|/\*|\*|\*/|--)`)

// buildOutline creates a collapsed view of a file using symbol body ranges.
// Lines inside function/method bodies are replaced with a single "{ ... }" marker.
// If stripComments is true, standalone comment lines outside bodies are removed.
func buildOutline(content []byte, symbols []*code.Symbol, stripComments bool) string {
	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	totalLines := len(lines)

	if totalLines == 0 {
		return "(empty file)"
	}

	// Build body ranges from symbols, sorted by start line.
	// Only include symbols that actually have body ranges.
	var ranges []bodyRange
	for _, sym := range symbols {
		if sym.BodyStartLine > 0 && sym.BodyEndLine > 0 && sym.BodyEndLine > sym.BodyStartLine {
			ranges = append(ranges, bodyRange{
				startLine: sym.BodyStartLine,
				endLine:   sym.BodyEndLine,
				symbol:    sym,
			})
		}
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].startLine < ranges[j].startLine
	})

	// Merge overlapping/nested ranges: for nested symbols (e.g., methods inside classes),
	// we want to collapse the inner bodies but keep the class structure visible.
	// Strategy: only collapse leaf-level bodies (functions/methods), not container bodies (classes).
	// For classes, we keep the body open so inner methods are visible.
	leafRanges := filterLeafBodies(ranges, symbols)

	// Build a set of lines that are "inside a collapsed body"
	// Map: lineNumber -> bodyRange (for the collapse marker)
	collapseStart := make(map[int]*bodyRange) // first line of body -> range
	collapsedLines := make(map[int]bool)      // lines to skip entirely

	for i := range leafRanges {
		r := &leafRanges[i]
		// The body start line itself gets the collapse marker
		collapseStart[r.startLine] = r
		// All lines after the start until end are collapsed (skipped)
		for line := r.startLine + 1; line <= r.endLine; line++ {
			collapsedLines[line] = true
		}
	}

	// Build the outline
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Outline: %d symbols, %d lines total\n\n", len(symbols), totalLines)

	for lineNum := 1; lineNum <= totalLines; lineNum++ {
		lineIdx := lineNum - 1
		line := lines[lineIdx]

		// If this line is inside a collapsed body (not the start), skip it
		if collapsedLines[lineNum] {
			continue
		}

		// If this line starts a collapsed body, emit the collapse marker
		if r, ok := collapseStart[lineNum]; ok {
			// Emit the signature line (which should be the line(s) before the body)
			// and then the collapse marker
			indent := extractIndent(line)
			fmt.Fprintf(&sb, "%s%s{ ... }  // lines %d-%d\n", lineNumPrefix(lineNum), indent, r.startLine, r.endLine)
			continue
		}

		// Strip comment-only lines if requested
		if stripComments && isCommentLine(line) {
			continue
		}

		// Strip blank lines between collapsed sections to keep output tight
		if strings.TrimSpace(line) == "" {
			// Keep blank lines that separate logical sections, skip consecutive ones
			continue
		}

		fmt.Fprintf(&sb, "%s%s\n", lineNumPrefix(lineNum), line)
	}

	return sb.String()
}

// filterLeafBodies returns only body ranges for leaf symbols (functions, methods)
// and not for container symbols (classes, interfaces) that contain other symbols.
func filterLeafBodies(ranges []bodyRange, _ []*code.Symbol) []bodyRange {
	var result []bodyRange
	for _, r := range ranges {
		kind := r.symbol.Kind
		// Classes and interfaces are containers — don't collapse their bodies
		// so inner methods remain visible
		if kind == code.KindClass || kind == code.KindInterface {
			continue
		}
		result = append(result, r)
	}
	return result
}

// isCommentLine checks if a line is a standalone comment (not code with trailing comment).
func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	return commentPattern.MatchString(trimmed)
}

// extractIndent returns the leading whitespace of a line.
func extractIndent(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return line
}

// lineNumPrefix formats a line number for the outline output.
func lineNumPrefix(lineNum int) string {
	return fmt.Sprintf("%-4d: ", lineNum)
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
