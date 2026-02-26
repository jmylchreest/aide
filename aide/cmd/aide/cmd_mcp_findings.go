package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Findings MCP Tool Input Types
// =============================================================================

type FindingsSearchInput struct {
	Query    string `json:"query" jsonschema:"Search query for finding titles and details. Supports Bleve query syntax."`
	Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: complexity, coupling, secrets, clones"`
	Severity string `json:"severity,omitempty" jsonschema:"Filter by severity: critical, warning, info"`
	FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Category string `json:"category,omitempty" jsonschema:"Filter by category"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 20)"`
}

type FindingsListInput struct {
	Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: complexity, coupling, secrets, clones"`
	Severity string `json:"severity,omitempty" jsonschema:"Filter by severity: critical, warning, info"`
	FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
	Category string `json:"category,omitempty" jsonschema:"Filter by category"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

type FindingsStatsInput struct{}

// =============================================================================
// Findings MCP Tool Registration
// =============================================================================

func (s *MCPServer) registerFindingsTools() {
	mcpLog.Printf("findings tools: registered")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "findings_search",
		Description: `Search static analysis findings by keyword using full-text search.

Searches finding titles and details — use when you need to find findings
about a specific function, pattern, or issue type by name.

**Examples:**
- "AWS" → finds hardcoded AWS credentials
- "complexity" → finds high-complexity functions
- "clone" → finds duplicated code regions

Filter by analyzer (complexity, coupling, secrets, clones),
severity (critical, warning, info), file path, or category.

**Tip:** Use findings_list instead when browsing by category without a specific keyword.
Findings are populated by the file watcher or by running 'aide findings run'.`,
	}, s.handleFindingsSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "findings_list",
		Description: `List static analysis findings with optional filters.

Returns findings filtered by analyzer, severity, file path, or category.
Does not require a search query — use this to browse or get all findings for a file.

**When to use:**
- "What issues are in src/auth?" → filter by file
- "Show me all critical findings" → filter by severity
- "Any secrets in the codebase?" → filter by analyzer=secrets
- "What's duplicated?" → filter by analyzer=clones

**Analyzers:** complexity, coupling, secrets, clones
**Severities:** critical (act now), warning (should fix), info (consider)`,
	}, s.handleFindingsList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "findings_stats",
		Description: `Get a health overview of the codebase from static analysis.

Returns total finding count with breakdowns by analyzer and severity.
Analyzers detect: complexity hotspots, hardcoded secrets, code duplication, and import coupling.

**Start here** — call this first when asked about code quality, technical debt,
security concerns, or before a code review. Then use findings_list or findings_search
to drill into specific areas.

If counts are zero, findings need to be generated — they are populated automatically
by the file watcher, or manually via 'aide findings run <analyzer>'.`,
	}, s.handleFindingsStats)
}

// =============================================================================
// Findings MCP Tool Handlers
// =============================================================================

func (s *MCPServer) handleFindingsSearch(_ context.Context, _ *mcp.CallToolRequest, input FindingsSearchInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: findings_search query=%q analyzer=%s severity=%s", input.Query, input.Analyzer, input.Severity)

	if s.findingsStore == nil {
		return errorResult("findings store not available"), nil, nil
	}

	opts := findings.SearchOptions{
		Analyzer: input.Analyzer,
		Severity: input.Severity,
		FilePath: input.FilePath,
		Category: input.Category,
		Limit:    input.Limit,
	}

	results, err := s.findingsStore.SearchFindings(input.Query, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("No findings found."), nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d findings:\n\n", len(results)))
	for _, r := range results {
		f := r.Finding
		sb.WriteString(formatFindingLine(f))
	}

	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleFindingsList(_ context.Context, _ *mcp.CallToolRequest, input FindingsListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: findings_list analyzer=%s severity=%s file=%s", input.Analyzer, input.Severity, input.FilePath)

	if s.findingsStore == nil {
		return errorResult("findings store not available"), nil, nil
	}

	opts := findings.SearchOptions{
		Analyzer: input.Analyzer,
		Severity: input.Severity,
		FilePath: input.FilePath,
		Category: input.Category,
		Limit:    input.Limit,
	}

	results, err := s.findingsStore.ListFindings(opts)
	if err != nil {
		return errorResult(fmt.Sprintf("list failed: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("No findings found."), nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d findings:\n\n", len(results)))
	for _, f := range results {
		sb.WriteString(formatFindingLine(f))
	}

	return textResult(sb.String()), nil, nil
}

func (s *MCPServer) handleFindingsStats(_ context.Context, _ *mcp.CallToolRequest, _ FindingsStatsInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: findings_stats")

	if s.findingsStore == nil {
		return errorResult("findings store not available"), nil, nil
	}

	stats, err := s.findingsStore.Stats()
	if err != nil {
		return errorResult(fmt.Sprintf("stats failed: %v", err)), nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Total findings: %d\n\n", stats.Total))

	if len(stats.ByAnalyzer) > 0 {
		sb.WriteString("By analyser:\n")
		for name, count := range stats.ByAnalyzer {
			sb.WriteString(fmt.Sprintf("  %-12s %d\n", name, count))
		}
		sb.WriteString("\n")
	}

	if len(stats.BySeverity) > 0 {
		sb.WriteString("By severity:\n")
		for sev, count := range stats.BySeverity {
			sb.WriteString(fmt.Sprintf("  %-12s %d\n", sev, count))
		}
	}

	return textResult(sb.String()), nil, nil
}

// =============================================================================
// Formatting Helpers
// =============================================================================

func formatFindingLine(f *findings.Finding) string {
	severity := strings.ToUpper(f.Severity)
	loc := f.FilePath
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.FilePath, f.Line)
	}
	return fmt.Sprintf("[%s] %s - %s (%s)\n", severity, loc, f.Title, f.Analyzer)
}
