package clone

import (
	"fmt"
	"strings"
)

// Report generates a human-readable summary of clone detection results.
func Report(result *Result) string {
	var sb strings.Builder

	sb.WriteString("Clone Detection Report\n")
	sb.WriteString("======================\n\n")

	fmt.Fprintf(&sb, "Files analyzed:  %d\n", result.FilesAnalyzed)
	fmt.Fprintf(&sb, "Files skipped:   %d\n", result.FilesSkipped)
	fmt.Fprintf(&sb, "Clone groups:    %d\n", result.CloneGroups)
	fmt.Fprintf(&sb, "Findings:        %d\n", result.FindingsCount)
	fmt.Fprintf(&sb, "Duration:        %s\n", result.Duration.Round(1_000_000))

	if result.CloneGroups == 0 {
		sb.WriteString("\nNo code clones detected.\n")
	}

	return sb.String()
}
