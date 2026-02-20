package main

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// MCP result helpers
// ============================================================================

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Error: " + message},
		},
		IsError: true,
	}
}

// ============================================================================
// Memory formatting
// ============================================================================

func formatMemoriesMarkdown(memories []*memory.Memory) string {
	if len(memories) == 0 {
		return "No memories found."
	}

	var sb strings.Builder
	sb.WriteString("# Memories\n\n")
	sb.WriteString("_Note: When multiple memories exist for the same topic, prefer the most recent (latest timestamp)._\n\n")

	categories := map[memory.Category][]*memory.Memory{}
	for _, m := range memories {
		categories[m.Category] = append(categories[m.Category], m)
	}

	for cat, mems := range categories {
		fmt.Fprintf(&sb, "## %s\n\n", titleCase(string(cat)))
		for _, m := range mems {
			fmt.Fprintf(&sb, "- **[%s]** %s", m.CreatedAt.Format("2006-01-02 15:04:05"), m.Content)
			if len(m.Tags) > 0 {
				fmt.Fprintf(&sb, " _(tags: %s)_", strings.Join(m.Tags, ", "))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ============================================================================
// Decision formatting
// ============================================================================

func formatDecisionMarkdown(d *memory.Decision) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Decision: %s\n\n", d.Topic)
	fmt.Fprintf(&sb, "**Decision:** %s\n\n", d.Decision)

	if d.Rationale != "" {
		fmt.Fprintf(&sb, "**Rationale:** %s\n\n", d.Rationale)
	}

	if d.Details != "" {
		sb.WriteString("## Details\n\n")
		sb.WriteString(d.Details)
		sb.WriteString("\n\n")
	}

	if len(d.References) > 0 {
		sb.WriteString("## References\n\n")
		for _, ref := range d.References {
			fmt.Fprintf(&sb, "- %s\n", ref)
		}
		sb.WriteString("\n")
	}

	if d.DecidedBy != "" {
		fmt.Fprintf(&sb, "_Decided by: %s_\n", d.DecidedBy)
	}
	fmt.Fprintf(&sb, "_Date: %s_\n", d.CreatedAt.Format("2006-01-02 15:04:05"))

	return sb.String()
}

func formatDecisionHistoryMarkdown(topic string, decisions []*memory.Decision) string {
	if len(decisions) == 0 {
		return fmt.Sprintf("No decisions found for topic: %s", topic)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Decision History: %s\n\n", topic)

	for i, d := range decisions {
		fmt.Fprintf(&sb, "## %d. %s\n\n", i+1, d.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&sb, "**Decision:** %s\n\n", d.Decision)

		if d.Rationale != "" {
			fmt.Fprintf(&sb, "**Rationale:** %s\n\n", d.Rationale)
		}

		if d.Details != "" {
			sb.WriteString("### Details\n\n")
			sb.WriteString(d.Details)
			sb.WriteString("\n\n")
		}

		if len(d.References) > 0 {
			sb.WriteString("### References\n\n")
			for _, ref := range d.References {
				fmt.Fprintf(&sb, "- %s\n", ref)
			}
			sb.WriteString("\n")
		}

		if d.DecidedBy != "" {
			fmt.Fprintf(&sb, "_Decided by: %s_\n\n", d.DecidedBy)
		}
		sb.WriteString("---\n\n")
	}

	return sb.String()
}

func formatDecisionsMarkdown(decisions []*memory.Decision) string {
	if len(decisions) == 0 {
		return "No decisions recorded."
	}

	latest := make(map[string]*memory.Decision)
	for _, d := range decisions {
		if existing, ok := latest[d.Topic]; !ok || d.CreatedAt.After(existing.CreatedAt) {
			latest[d.Topic] = d
		}
	}

	var sb strings.Builder
	sb.WriteString("# Decisions\n\n")

	for topic, d := range latest {
		fmt.Fprintf(&sb, "## %s\n\n", topic)
		fmt.Fprintf(&sb, "**Decision:** %s\n\n", d.Decision)

		if d.Rationale != "" {
			fmt.Fprintf(&sb, "**Rationale:** %s\n\n", d.Rationale)
		}

		if d.Details != "" {
			sb.WriteString("### Details\n\n")
			sb.WriteString(d.Details)
			sb.WriteString("\n\n")
		}

		if len(d.References) > 0 {
			sb.WriteString("### References\n\n")
			for _, ref := range d.References {
				fmt.Fprintf(&sb, "- %s\n", ref)
			}
			sb.WriteString("\n")
		}

		fmt.Fprintf(&sb, "_Date: %s_\n\n", d.CreatedAt.Format("2006-01-02 15:04:05"))
		sb.WriteString("---\n\n")
	}

	return sb.String()
}
