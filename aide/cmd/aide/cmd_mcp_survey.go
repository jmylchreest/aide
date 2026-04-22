package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Survey MCP Tool Input Types
// =============================================================================

type SurveySearchInput struct {
	Query    string `json:"query" jsonschema:"Search query for survey entry names, titles, and details. Supports Bleve query syntax."`
	Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: topology, entrypoints, churn"`
	Kind     string `json:"kind,omitempty" jsonschema:"Filter by kind: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern"`
	FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 20)"`
}

type SurveyListInput struct {
	Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: topology, entrypoints, churn"`
	Kind     string `json:"kind,omitempty" jsonschema:"Filter by kind: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern"`
	FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

type SurveyStatsInput struct{}

type SurveyRunInput struct {
	Analyzer string `json:"analyzer,omitempty" jsonschema:"Run a specific analyzer: topology, entrypoints, churn. Omit to run all."`
}

type SurveyGraphInput struct {
	Symbol    string `json:"symbol" jsonschema:"Name of the symbol to start traversal from (e.g. 'BuildCallGraph', 'handleSurveyRun')."`
	Direction string `json:"direction,omitempty" jsonschema:"Traversal direction: both (default), callers, callees"`
	MaxDepth  int    `json:"max_depth,omitempty" jsonschema:"Maximum BFS hops from root (default 2)"`
	MaxNodes  int    `json:"max_nodes,omitempty" jsonschema:"Maximum nodes in graph (default 50)"`
}

// =============================================================================
// Survey MCP Tool Registration
// =============================================================================

func (s *MCPServer) registerSurveyTools() {
	mcpLog.Printf("survey tools: registered")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "survey_search",
		Description: `Search codebase survey entries by keyword using full-text search.

Searches entry names, titles, and details — use when you need to find
structural information about a specific module, package, or technology.

**What survey describes:** WHAT the codebase IS — its structure, modules,
entry points, tech stack, and hotspots. NOT code problems (use findings_search
for issues like complexity or security).

**Examples:**
- "auth" → finds modules/entrypoints related to authentication
- "React" → finds tech stack entries for React framework
- "main" → finds main() entry points

Filter by analyzer (topology, entrypoints, churn),
kind (module, entrypoint, dependency, tech_stack, churn, etc.), or file path.

**Tip:** Use survey_list to browse by kind without a search keyword.
Use survey_stats first to see what has been analyzed.`,
	}, s.handleSurveySearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "survey_list",
		Description: `List codebase survey entries with optional filters.

Returns structural information about the codebase filtered by analyzer,
kind, or file path. Does not require a search query.

**What survey describes:** WHAT the codebase IS — its structure, modules,
entry points, tech stack, and hotspots. NOT code problems (use findings_list
for code health issues).

**When to use:**
- "What modules are in this codebase?" → kind=module
- "Where are the entry points?" → kind=entrypoint
- "What technologies does this use?" → kind=tech_stack
- "What files change most?" → kind=churn
- "What's in src/auth/?" → filter by file path

**Kinds:** module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern
**Analyzers:** topology (structure), entrypoints (entry points), churn (git history)`,
	}, s.handleSurveyList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "survey_stats",
		Description: `Get an overview of what has been surveyed in the codebase.

Returns total entry count with breakdowns by analyzer and kind.

**Start here** — call this first when asked about codebase structure,
architecture, or technology stack. Then use survey_list or survey_search
to drill into specific areas.

**What survey describes:** WHAT the codebase IS — its structure, modules,
entry points, tech stack, and hotspots. For code PROBLEMS (complexity,
security, duplication), use findings_stats instead.

If counts are zero, run 'aide survey run' or use survey_run to populate.`,
	}, s.handleSurveyStats)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "survey_run",
		Description: `Run codebase survey analyzers to populate structural information.

Analyzers scan the codebase to discover:
- **topology**: Modules, packages, workspaces, build systems, tech stack
- **entrypoints**: main() functions, HTTP handler mounts, CLI roots
- **churn**: Git history hotspots (files/dirs that change most often)

Run all analyzers (omit analyzer param) or a specific one.
Results are cached — re-run to refresh after significant changes.

**Note:** This is a survey of codebase STRUCTURE. For code health analysis
(complexity, security, duplication), use 'aide findings run' instead.`,
	}, s.handleSurveyRun)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "survey_graph",
		Description: `Build a call graph for a symbol showing callers and callees.

Performs BFS traversal over the code index starting from the named symbol.
Returns a graph of nodes (symbols) and edges (call/type_ref relationships).

**Use cases:**
- "Who calls this function?" → direction=callers
- "What does this function call?" → direction=callees
- "Show the call neighborhood" → direction=both (default)
- "What breaks if I change this?" → direction=callers, max_depth=3 (impact analysis)

**Impact analysis:** Use direction=callers with a higher max_depth to trace the
full blast radius of a change. All transitive callers are potential breakage points.

**How it works:**
- Callees: found by scanning references within the symbol's body line range
- Callers: found by looking up call-site references to the symbol name

**Parameters:**
- symbol (required): Function/method name to start from
- direction: "both" (default), "callers", "callees"
- max_depth: BFS hops from root (default 2, increase for impact analysis)
- max_nodes: Cap on total graph nodes (default 50)

**Note:** Requires the code index to be populated (run 'aide code index').
Computed on demand — not stored. Results reflect the current code index state.`,
	}, s.handleSurveyGraph)
}

// =============================================================================
// Survey MCP Tool Handlers
// =============================================================================

func (s *MCPServer) handleSurveySearch(ctx context.Context, _ *mcp.CallToolRequest, input SurveySearchInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: survey_search query=%q analyzer=%s kind=%s", input.Query, input.Analyzer, input.Kind)
	span := observe.FromContext(ctx)

	if s.surveyStore == nil {
		return errorResult("survey store not available"), nil, nil
	}

	opts := survey.SearchOptions{
		Analyzer: input.Analyzer,
		Kind:     input.Kind,
		FilePath: input.FilePath,
		Limit:    input.Limit,
	}

	results, err := s.surveyStore.SearchEntries(input.Query, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("No survey entries found."), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d entries:\n\n", len(results))
	entries := make([]*survey.Entry, 0, len(results))
	for _, r := range results {
		sb.WriteString(formatSurveyEntryLine(r.Entry))
		entries = append(entries, r.Entry)
	}

	respText := sb.String()
	counterfactual := survey.CounterfactualTokensForEntries(projectRoot(s.dbPath), entries)
	recordSurveySavings(span, respText, counterfactual)
	return textResult(respText), nil, nil
}

func (s *MCPServer) handleSurveyList(ctx context.Context, _ *mcp.CallToolRequest, input SurveyListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: survey_list analyzer=%s kind=%s file=%s", input.Analyzer, input.Kind, input.FilePath)
	span := observe.FromContext(ctx)

	if s.surveyStore == nil {
		return errorResult("survey store not available"), nil, nil
	}

	opts := survey.SearchOptions{
		Analyzer: input.Analyzer,
		Kind:     input.Kind,
		FilePath: input.FilePath,
		Limit:    input.Limit,
	}

	results, err := s.surveyStore.ListEntries(opts)
	if err != nil {
		return errorResult(fmt.Sprintf("list failed: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("No survey entries found."), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d entries:\n\n", len(results))
	for _, e := range results {
		sb.WriteString(formatSurveyEntryLine(e))
	}

	respText := sb.String()
	counterfactual := survey.CounterfactualTokensForEntries(projectRoot(s.dbPath), results)
	recordSurveySavings(span, respText, counterfactual)
	return textResult(respText), nil, nil
}

func (s *MCPServer) handleSurveyStats(_ context.Context, _ *mcp.CallToolRequest, input SurveyStatsInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: survey_stats")

	if s.surveyStore == nil {
		return errorResult("survey store not available"), nil, nil
	}

	stats, err := s.surveyStore.Stats(survey.SearchOptions{})
	if err != nil {
		return errorResult(fmt.Sprintf("stats failed: %v", err)), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Total survey entries: %d\n\n", stats.Total)

	if len(stats.ByAnalyzer) > 0 {
		sb.WriteString("By analyzer:\n")
		for name, count := range stats.ByAnalyzer {
			fmt.Fprintf(&sb, "  %-14s %d\n", name, count)
		}
		sb.WriteString("\n")
	}

	if len(stats.ByKind) > 0 {
		sb.WriteString("By kind:\n")
		for kind, count := range stats.ByKind {
			fmt.Fprintf(&sb, "  %-14s %d\n", kind, count)
		}
	}

	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleSurveyRun(_ context.Context, _ *mcp.CallToolRequest, input SurveyRunInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: survey_run analyzer=%s", input.Analyzer)

	if s.surveyStore == nil {
		return errorResult("survey store not available"), nil, nil
	}

	rootDir := projectRoot(s.dbPath)

	var sb strings.Builder
	analyzers := []string{input.Analyzer}
	if input.Analyzer == "" {
		analyzers = []string{survey.AnalyzerTopology, survey.AnalyzerEntrypoints, survey.AnalyzerChurn}
	}

	for _, name := range analyzers {
		switch name {
		case survey.AnalyzerTopology:
			result, err := survey.RunTopology(rootDir)
			if err != nil {
				fmt.Fprintf(&sb, "topology: error: %v\n", err)
				continue
			}
			if err := s.surveyStore.ReplaceEntriesForAnalyzer(survey.AnalyzerTopology, result.Entries); err != nil {
				fmt.Fprintf(&sb, "topology: store error: %v\n", err)
				continue
			}
			fmt.Fprintf(&sb, "topology: %d entries\n", len(result.Entries))

		case survey.AnalyzerEntrypoints:
			var cs survey.CodeSearcher
			if codeStore := s.getCodeStore(); codeStore != nil {
				cs = &codeSearcherAdapter{store: codeStore}
			}
			result, err := survey.RunEntrypoints(rootDir, cs)
			if err != nil {
				fmt.Fprintf(&sb, "entrypoints: error: %v\n", err)
				continue
			}
			if err := s.surveyStore.ReplaceEntriesForAnalyzer(survey.AnalyzerEntrypoints, result.Entries); err != nil {
				fmt.Fprintf(&sb, "entrypoints: store error: %v\n", err)
				continue
			}
			fmt.Fprintf(&sb, "entrypoints: %d entries\n", len(result.Entries))
			if cs == nil {
				sb.WriteString("  (code index not available — entrypoint detection limited)\n")
			}

		case survey.AnalyzerChurn:
			result, err := survey.RunChurn(rootDir, 0, 0)
			if err != nil {
				fmt.Fprintf(&sb, "churn: error: %v\n", err)
				continue
			}
			if err := s.surveyStore.ReplaceEntriesForAnalyzer(survey.AnalyzerChurn, result.Entries); err != nil {
				fmt.Fprintf(&sb, "churn: store error: %v\n", err)
				continue
			}
			fmt.Fprintf(&sb, "churn: %d entries\n", len(result.Entries))

		default:
			fmt.Fprintf(&sb, "unknown analyzer: %s\n", name)
		}
	}

	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleSurveyGraph(ctx context.Context, _ *mcp.CallToolRequest, input SurveyGraphInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: survey_graph symbol=%q direction=%s depth=%d nodes=%d", input.Symbol, input.Direction, input.MaxDepth, input.MaxNodes)
	span := observe.FromContext(ctx)

	if input.Symbol == "" {
		return errorResult("symbol name is required"), nil, nil
	}

	codeStore := s.getCodeStore()
	if codeStore == nil {
		return errorResult("code index not available — run 'aide code index' first"), nil, nil
	}

	cg := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: codeStore}}
	opts := survey.GraphOptions{
		MaxDepth:  input.MaxDepth,
		MaxNodes:  input.MaxNodes,
		Direction: input.Direction,
	}

	graph, err := survey.BuildCallGraph(cg, input.Symbol, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("graph traversal failed: %v", err)), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Call graph for %q (depth=%d, direction=%s)\n", graph.Root, graph.Depth, input.Direction)
	if input.Direction == "" {
		// Amend display since BuildCallGraph defaults "" to "both"
		sb.Reset()
		fmt.Fprintf(&sb, "Call graph for %q (depth=%d, direction=both)\n", graph.Root, graph.Depth)
	}
	fmt.Fprintf(&sb, "Nodes: %d, Edges: %d\n\n", len(graph.Nodes), len(graph.Edges))

	if len(graph.Nodes) > 0 {
		sb.WriteString("Nodes:\n")
		for _, n := range graph.Nodes {
			marker := " "
			if n.Name == graph.Root {
				marker = "*"
			}
			fmt.Fprintf(&sb, "  %s %-30s %-10s %s:%d\n", marker, n.Name, n.Kind, n.FilePath, n.Line)
		}
		sb.WriteString("\n")
	}

	if len(graph.Edges) > 0 {
		sb.WriteString("Edges:\n")
		for _, e := range graph.Edges {
			fmt.Fprintf(&sb, "  %s -> %s  (%s at %s:%d)\n", e.From, e.To, e.Kind, e.FilePath, e.Line)
		}
	}

	if len(graph.Nodes) == 0 {
		sb.WriteString("No call relationships found.\n")
	}

	// Counterfactual for a graph query: tokens the agent would have paid to
	// trace this neighbourhood manually via code_references + file reads.
	// Approximated by summing token estimates for distinct files appearing
	// in the graph's nodes — that's the read workload the graph replaces.
	counterfactual := graphCounterfactualTokens(projectRoot(s.dbPath), graph.Nodes)
	respText := sb.String()
	recordSurveySavings(span, respText, counterfactual)
	return textResult(respText), nil, nil
}

// =============================================================================
// Savings attribution for survey handlers
// =============================================================================

// recordSurveySavings stamps the span with response size (Tokens) and the
// counterfactual delta (Saved) — mirroring the pattern established by
// code_outline / code_read_symbol at cmd_mcp_code.go:477. Passing no file path
// to EstimateTokens falls back to the default chars-per-token ratio, which is
// appropriate for aide-generated response text.
func recordSurveySavings(span *observe.Span, respText string, counterfactual int) {
	respTokens := code.EstimateTokens("", len(respText))
	saved := counterfactual - respTokens
	if saved < 0 {
		saved = 0
	}
	span.Tokens(respTokens).Saved(saved)
}

// graphCounterfactualTokens estimates the cost of deriving a call graph
// manually — reading each distinct file the graph nodes reference. Graph
// nodes come from the code index rather than the survey store, so they
// never carry cached est_tokens; CounterfactualTokensForEntries falls back
// to a live file-stat for each one.
func graphCounterfactualTokens(rootDir string, nodes []survey.GraphNode) int {
	if len(nodes) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(nodes))
	entries := make([]*survey.Entry, 0, len(nodes))
	for _, n := range nodes {
		if n.FilePath == "" {
			continue
		}
		if _, dup := seen[n.FilePath]; dup {
			continue
		}
		seen[n.FilePath] = struct{}{}
		entries = append(entries, &survey.Entry{FilePath: n.FilePath})
	}
	return survey.CounterfactualTokensForEntries(rootDir, entries)
}

// =============================================================================
// CodeSearcher Adapter — bridges store.CodeIndexStore to survey.CodeSearcher
// =============================================================================

// codeSearcherAdapter wraps a store.CodeIndexStore to implement survey.CodeSearcher.
// This lives in cmd/aide/ because it can import both pkg/store and pkg/survey
// without creating an import cycle.
type codeSearcherAdapter struct {
	store store.CodeIndexStore
}

func (a *codeSearcherAdapter) FindSymbols(query string, kind string, limit int) ([]survey.SymbolHit, error) {
	opts := code.SearchOptions{
		Kind:  kind,
		Limit: limit,
	}
	results, err := a.store.SearchSymbols(query, opts)
	if err != nil {
		return nil, err
	}
	hits := make([]survey.SymbolHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, survey.SymbolHit{
			Name:     r.Symbol.Name,
			Kind:     r.Symbol.Kind,
			FilePath: r.Symbol.FilePath,
			Line:     r.Symbol.StartLine,
			Language: r.Symbol.Language,
		})
	}
	return hits, nil
}

func (a *codeSearcherAdapter) FindReferences(symbolName string, kind string, limit int) ([]survey.ReferenceHit, error) {
	opts := code.ReferenceSearchOptions{
		SymbolName: symbolName,
		Kind:       kind,
		Limit:      limit,
	}
	results, err := a.store.SearchReferences(opts)
	if err != nil {
		return nil, err
	}
	hits := make([]survey.ReferenceHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, survey.ReferenceHit{
			Symbol:   r.SymbolName,
			Kind:     r.Kind,
			FilePath: r.FilePath,
			Line:     r.Line,
		})
	}
	return hits, nil
}

// =============================================================================
// CodeGrapher Adapter — extends codeSearcherAdapter for call-graph traversal
// =============================================================================

// codeGrapherAdapter wraps a store.CodeIndexStore to implement survey.CodeGrapher.
// It embeds codeSearcherAdapter for FindSymbols/FindReferences, and adds
// GetFileReferences and GetContainingSymbol needed by BuildCallGraph.
type codeGrapherAdapter struct {
	codeSearcherAdapter
}

func (a *codeGrapherAdapter) GetFileReferences(filePath string) ([]survey.ReferenceHit, error) {
	refs, err := a.store.GetFileReferences(filePath)
	if err != nil {
		return nil, err
	}
	hits := make([]survey.ReferenceHit, 0, len(refs))
	for _, r := range refs {
		hits = append(hits, survey.ReferenceHit{
			Symbol:   r.SymbolName,
			Kind:     r.Kind,
			FilePath: r.FilePath,
			Line:     r.Line,
		})
	}
	return hits, nil
}

func (a *codeGrapherAdapter) GetContainingSymbol(filePath string, line int) (*survey.SymbolHit, error) {
	sym, err := a.store.GetContainingSymbol(filePath, line)
	if err != nil {
		return nil, err
	}
	if sym == nil {
		return nil, nil
	}
	return &survey.SymbolHit{
		Name:     sym.Name,
		Kind:     sym.Kind,
		FilePath: sym.FilePath,
		Line:     sym.StartLine,
		EndLine:  sym.EndLine,
		Language: sym.Language,
	}, nil
}

// =============================================================================
// Formatting Helpers
// =============================================================================

func formatSurveyEntryLine(e *survey.Entry) string {
	kind := strings.ToUpper(e.Kind)
	loc := e.Name
	if e.FilePath != "" {
		loc = e.FilePath
	}
	return fmt.Sprintf("[%s] %s - %s (%s)\n", kind, loc, e.Title, e.Analyzer)
}
