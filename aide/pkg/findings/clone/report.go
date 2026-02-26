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

	sb.WriteString(fmt.Sprintf("Files analyzed:  %d\n", result.FilesAnalyzed))
	sb.WriteString(fmt.Sprintf("Files skipped:   %d\n", result.FilesSkipped))
	sb.WriteString(fmt.Sprintf("Clone groups:    %d\n", result.CloneGroups))
	sb.WriteString(fmt.Sprintf("Findings:        %d\n", result.FindingsCount))
	sb.WriteString(fmt.Sprintf("Duration:        %s\n", result.Duration.Round(1_000_000)))

	if result.CloneGroups == 0 {
		sb.WriteString("\nNo code clones detected.\n")
	}

	return sb.String()
}

// DuplicationRatio estimates the fraction of analyzed code that is duplicated.
// It returns a value between 0.0 (no duplication) and 1.0 (all duplicated).
// This is a rough estimate based on finding count vs files analyzed.
func DuplicationRatio(result *Result) float64 {
	if result.FilesAnalyzed == 0 {
		return 0
	}
	// Each finding pair represents 2 findings (one per side).
	// A finding of N lines in a file of M files means N/M fraction.
	// We approximate: ratio = unique_clone_files / total_files.
	ratio := float64(result.FindingsCount) / float64(result.FilesAnalyzed*2)
	if ratio > 1.0 {
		ratio = 1.0
	}
	return ratio
}
