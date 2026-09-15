package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// formatTokenWork describes recorded service operations without treating a
// returned response as evidence of task success or converting time to tokens.
func formatTokenWork(w *memory.TokenWork) string {
	var b strings.Builder
	b.WriteString("\nAide work (recorded MCP service calls)\n")
	if w == nil || w.Version != 1 {
		b.WriteString("  Work accounting unavailable from this server.\n")
		return b.String()
	}
	if w.Calls == 0 {
		b.WriteString("  No eligible service calls in this selection.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  %d calls: %d returned; %d reported errors; %d unknown outcomes\n", w.Calls, w.Returned, w.ReportedErrors, w.UnknownOutcomes)
	b.WriteString("  Returned means no reported tool error, not task success or output quality.\n")
	b.WriteString("  Measured handler wall time can overlap; not CPU time or avoided model work.\n")
	b.WriteString("  Returned text is the server_result boundary, not additional text.\n")
	fmt.Fprintf(&b, "  Unassigned sessions: %d; background work and unseen calls are unknown.\n", w.UnassignedSessions)
	b.WriteString("  Coverage shown as [measured/calls].\n")
	fmt.Fprintf(&b, "  %-24s %7s %7s %7s %7s  %-22s %s\n", "Tool", "Calls", "Return", "Errors", "Unknown", "Wall time", "Returned text")
	row := func(name string, c *memory.TokenWorkCounters) {
		if c == nil {
			return
		}
		duration := "unknown"
		if c.MeasuredDurations > 0 {
			duration = fmt.Sprintf("%d ms", c.ElapsedMs)
		}
		payload := "unknown"
		if c.ReturnedText.Events > 0 {
			payload = fmt.Sprintf("%d B", c.ReturnedText.Bytes)
		}
		fmt.Fprintf(&b, "  %-24s %7d %7d %7d %7d  %-22s %s [%d/%d]\n", name, c.Calls, c.Returned, c.ReportedErrors, c.UnknownOutcomes, fmt.Sprintf("%s [%d/%d]", duration, c.MeasuredDurations, c.Calls), payload, c.ReturnedText.Events, c.Calls)
	}
	row("Total", &w.TokenWorkCounters)
	names := make([]string, 0, len(w.ByTool))
	for name := range w.ByTool {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		row(name, w.ByTool[name])
	}
	return b.String()
}
