package survey

import (
	"fmt"
	"log"
)

// DefaultTopChurnFiles is the default number of high-churn files to report.
const DefaultTopChurnFiles = 50

// ChurnResult holds the output of the churn analyzer.
type ChurnResult struct {
	Entries []*Entry
}

// RunChurn analyzes git history to find high-churn files.
// Returns nil, nil if the directory is not a git repo (graceful skip).
func RunChurn(rootDir string, maxCommits int, topN int) (*ChurnResult, error) {
	gitRepo, err := OpenGitRepo(rootDir)
	if err != nil {
		return nil, fmt.Errorf("churn analyzer: %w", err)
	}
	if gitRepo == nil {
		// Not a git repo — graceful skip
		return &ChurnResult{}, nil
	}

	if topN <= 0 {
		topN = DefaultTopChurnFiles
	}

	// Get per-file churn statistics
	stats, err := gitRepo.FileChurnStats(maxCommits)
	if err != nil {
		log.Printf("survey: churn analysis warning: %v", err)
		return &ChurnResult{}, nil
	}

	// Get top N files by churn score
	topFiles := TopChurnFiles(stats, topN)

	result := &ChurnResult{}
	for _, cs := range topFiles {
		result.Entries = append(result.Entries, &Entry{
			Analyzer: AnalyzerChurn,
			Kind:     KindChurn,
			Name:     cs.FilePath,
			FilePath: cs.FilePath,
			Title:    fmt.Sprintf("High churn: %s (%d commits, %d lines changed)", cs.FilePath, cs.Commits, cs.LinesChanged),
			Detail:   fmt.Sprintf("File changed in %d commits with %d total lines modified", cs.Commits, cs.LinesChanged),
			Metadata: map[string]string{
				"commits":       fmt.Sprintf("%d", cs.Commits),
				"lines_changed": fmt.Sprintf("%d", cs.LinesChanged),
			},
		})
	}

	// Also detect submodules
	submodules, err := gitRepo.Submodules()
	if err != nil {
		// Non-fatal — just skip submodule detection
		log.Printf("survey: submodule detection warning: %v", err)
	} else {
		for _, sub := range submodules {
			result.Entries = append(result.Entries, &Entry{
				Analyzer: AnalyzerChurn, // Churn analyzer owns git-related discoveries
				Kind:     KindSubmodule,
				Name:     sub.Name,
				FilePath: sub.Path,
				Title:    fmt.Sprintf("Git submodule: %s", sub.Name),
				Detail:   fmt.Sprintf("Submodule %s at %s (URL: %s)", sub.Name, sub.Path, sub.URL),
				Metadata: map[string]string{
					"url":    sub.URL,
					"status": sub.Status,
				},
			})
		}
	}

	return result, nil
}
