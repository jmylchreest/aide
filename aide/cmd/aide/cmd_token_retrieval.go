package main

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func formatRetrievalWindows(report *memory.TokenRetrievals, details bool) string {
	if report == nil {
		return "  Retrieval grouping: unavailable from this server\n"
	}
	count := 0
	for _, w := range report.Windows {
		if w.Comparison != nil {
			count++
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "  Retrievals: %d windows; %d conditional comparison(s); %d unwindowed observations\n", len(report.Windows), count, report.UnwindowedEvents)
	if report.WindowsLimited {
		out.WriteString("  Retrieval report limited to 64 windows.\n")
	}
	if !details {
		return out.String()
	}
	out.WriteString("\nRetrieval sequences by context window (partial collection coverage)\n")
	for _, w := range report.Windows {
		fmt.Fprintf(&out, "  %s / %s / %s: %d observations; %d full-file results; %s\n", w.Host, w.SessionID, w.Epoch, w.Events, w.FullReadEvents, w.Boundary)
		if w.Comparison != nil {
			q := w.Comparison
			direction := "fewer"
			bytes, tokens := q.DeltaBytes, q.EstimatedTokenDelta
			if bytes < 0 {
				direction = "more"
				bytes = -bytes
			}
			tokenDirection := "fewer"
			if tokens < 0 {
				tokenDirection = "more"
				tokens = -tokens
			}
			fmt.Fprintf(&out, "    Conditional comparison: %d %s bytes; ~%d %s tokens vs one full-file reference per version\n", bytes, direction, tokens, tokenDirection)
		} else {
			fmt.Fprintf(&out, "    comparison unavailable: %s\n", strings.Join(w.Issues, ", "))
		}
		fmt.Fprintf(&out, "    %d measured retrieval bytes; %d unassigned shell bytes; %d missing text results\n", w.Observed.Bytes, w.Unattributed.Bytes, w.MissingPayload)
		if w.FullReadEvents == 0 {
			out.WriteString("    No full-file result observed in these records; coverage is partial.\n")
		}
	}
	out.WriteString("  These are conditional text comparisons, not avoided calls or provider savings. Event sequences and source hashes are available in --json and web Details.\n")
	return out.String()
}

func formatRetrievalEvidence(attrs map[string]string) string {
	labels := map[string]string{
		"full_file":          "full-file text matched",
		"range":              "source range matched",
		"referenced":         "source receipt matched",
		"unverified":         "source delivery unverified",
		"failed":             "retrieval failed",
		"pending":            "retrieval still running",
		"search":             "search observed; file delivery unverified",
		"unclassified_shell": "shell retrieval coverage unknown",
	}
	label := labels[attrs["retrieval_status"]]
	if label == "" {
		if attrs["source_references"] == "" {
			return ""
		}
		label = "server source reference; host delivery unknown"
	}
	text := "  Retrieval: " + label
	if method := attrs["retrieval_method"]; method != "" {
		text += "; method=" + method
	}
	if attrs["source_verification"] == "current_range_match" {
		text += fmt.Sprintf("; lines %s-%s matched; undisplayed source version unverified", attrs["delivered_start_line"], attrs["delivered_end_line"])
	}
	if attrs["source_references"] != "" {
		text += "; conditional references in --json (not avoided reads)"
	}
	return text + "\n"
}
