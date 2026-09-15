package main

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func formatTransformationSummary(report *memory.TokenTransformations, details bool) string {
	var out strings.Builder
	if report == nil {
		return "  Paired output changes: unavailable from this server.\n"
	}
	for _, row := range []struct{ stage, label string }{{"rewrite_candidate", "Proposed rewrites"}, {"adapter_change", "Adapter changes"}} {
		q := report.ByStage[row.stage]
		if q == nil || q.Events == 0 {
			fmt.Fprintf(&out, "  %s: unknown (no measured pairs)\n", row.label)
			continue
		}
		fmt.Fprintf(&out, "  %s: %d → %d bytes; reduction %+d bytes (~%+d tokens), %d pairs\n", row.label, q.BeforeBytes, q.AfterBytes, q.DeltaBytes, q.EstimatedTokenDelta, q.Events)
	}
	out.WriteString("  Positive means less text; negative means overhead. Neither stage confirms final delivery.\n")
	if details {
		fmt.Fprintf(&out, "  Window coverage: %d unwindowed; %d invalid pairs\n", report.UnwindowedEvents, report.InvalidEvents)
		for _, w := range report.Windows {
			fmt.Fprintf(&out, "    %s session=%s actor=%s window=%s %s: %+d bytes (~%+d tokens), %d pairs\n", w.Host, w.SessionID, w.ActorID, w.Epoch, w.Stage, w.Change.DeltaBytes, w.Change.EstimatedTokenDelta, w.Change.Events)
		}
		if report.WindowsLimited {
			out.WriteString("  Additional windows omitted; stage totals include all selected pairs.\n")
		}
	}
	return out.String()
}
