package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// Config configures the clone detection analyzer.
type Config struct {
	// WindowSize is the number of tokens in the sliding hash window (default 50).
	WindowSize int
	// MinCloneLines is the minimum number of source lines for a clone to be reported (default 6).
	MinCloneLines int
	// Paths to analyze (default: current directory).
	Paths []string
	// ProgressFn is called after each file is tokenized. May be nil.
	ProgressFn func(path string, tokens int)
	// Ignore is the aideignore matcher for filtering files/directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
	// Loader is the grammar loader for tree-sitter languages.
	// If nil, a default CompositeLoader is created.
	Loader grammar.Loader
}

// Result holds the output of a clone detection run.
type Result struct {
	FilesAnalyzed int
	FilesSkipped  int
	CloneGroups   int
	FindingsCount int
	Duration      time.Duration
}

// defaultWindowSize returns the configured or default window size.
func defaultWindowSize(size int) int {
	if size > 0 {
		return size
	}
	return 50
}

// defaultMinCloneLines returns the configured or default minimum clone lines.
func defaultMinCloneLines(lines int) int {
	if lines > 0 {
		return lines
	}
	return 6
}

// DetectClones analyzes source files for code clones using Rabin-Karp hashing.
// It returns findings for each clone group and a result summary.
func DetectClones(cfg Config) ([]*findings.Finding, *Result, error) {
	start := time.Now()
	windowSize := defaultWindowSize(cfg.WindowSize)
	minLines := defaultMinCloneLines(cfg.MinCloneLines)
	paths := cfg.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	if cfg.Loader == nil {
		cfg.Loader = grammar.NewCompositeLoader()
	}

	result := &Result{}
	index := NewCloneIndex()
	hasher := NewRollingHash(windowSize)

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	// Phase 1: Tokenize all files and build the hash index.
	for _, root := range paths {
		absRoot, _ := filepath.Abs(root)
		shouldSkip := ignore.WalkFunc(absRoot)

		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				result.FilesSkipped++
				return nil
			}
			if info.IsDir() {
				return nil
			}

			// Only process supported source files.
			ext := strings.ToLower(filepath.Ext(path))
			lang, ok := code.LangExtensions[ext]
			if !ok {
				result.FilesSkipped++
				return nil
			}

			// Skip very large files (> 500KB).
			if info.Size() > 512*1024 {
				result.FilesSkipped++
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				result.FilesSkipped++
				return nil
			}

			relPath, _ := filepath.Rel(".", path)
			if relPath == "" {
				relPath = path
			}

			seq, err := Tokenize(cfg.Loader, relPath, content, lang)
			if err != nil || seq == nil || len(seq.Tokens) < windowSize {
				result.FilesSkipped++
				return nil
			}

			result.FilesAnalyzed++

			// Compute rolling hashes and add to index.
			hashes := hasher.ComputeHashes(seq.Tokens)
			index.AddFile(relPath, hashes)

			if cfg.ProgressFn != nil {
				cfg.ProgressFn(relPath, len(seq.Tokens))
			}

			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error walking %s: %w", root, err)
		}
	}

	// Phase 2: Find clone groups.
	groups := index.ClonePairs(windowSize)
	result.CloneGroups = len(groups)

	// Phase 3: Convert clone groups to findings.
	// We merge groups that share file-pairs to avoid redundant findings.
	pairMap := mergeCloneGroups(groups, minLines)

	var allFindings []*findings.Finding
	for pair, info := range pairMap {
		// Report a finding for each file in the pair.
		for _, side := range []struct {
			file      string
			startLine int
			endLine   int
			otherFile string
			otherLine int
		}{
			{pair.FileA, info.StartLineA, info.EndLineA, pair.FileB, info.StartLineB},
			{pair.FileB, info.StartLineB, info.EndLineB, pair.FileA, info.StartLineA},
		} {
			severity := findings.SevInfo
			lineSpan := side.endLine - side.startLine + 1
			if lineSpan >= 50 {
				severity = findings.SevWarning
			}
			if lineSpan >= 100 {
				severity = findings.SevCritical
			}

			f := &findings.Finding{
				Analyzer: findings.AnalyzerClones,
				Severity: severity,
				Category: "duplicate",
				FilePath: side.file,
				Line:     side.startLine,
				EndLine:  side.endLine,
				Title:    fmt.Sprintf("Code clone detected (~%d lines)", lineSpan),
				Detail:   fmt.Sprintf("Duplicated code block (lines %d-%d) also found in %s:%d. %d matching hash windows.", side.startLine, side.endLine, side.otherFile, side.otherLine, info.MatchCount),
				Metadata: map[string]string{
					"clone_file":  side.otherFile,
					"clone_line":  fmt.Sprintf("%d", side.otherLine),
					"line_span":   fmt.Sprintf("%d", lineSpan),
					"match_count": fmt.Sprintf("%d", info.MatchCount),
				},
				CreatedAt: time.Now(),
			}
			allFindings = append(allFindings, f)
		}
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)

	return allFindings, result, nil
}

// filePair is a canonical pair of files (FileA < FileB lexicographically).
type filePair struct {
	FileA string
	FileB string
}

// clonePairInfo aggregates clone information for a file pair.
type clonePairInfo struct {
	StartLineA int
	EndLineA   int
	StartLineB int
	EndLineB   int
	MatchCount int
}

// mergeCloneGroups consolidates clone groups into file pairs with
// aggregated line ranges and match counts.
func mergeCloneGroups(groups []CloneGroup, minLines int) map[filePair]*clonePairInfo {
	pairMap := make(map[filePair]*clonePairInfo)

	for _, g := range groups {
		// For each pair of locations in the group.
		for i := 0; i < len(g.Locations); i++ {
			for j := i + 1; j < len(g.Locations); j++ {
				a := g.Locations[i]
				b := g.Locations[j]

				// Canonical order.
				if a.FilePath > b.FilePath || (a.FilePath == b.FilePath && a.StartLine > b.StartLine) {
					a, b = b, a
				}

				pair := filePair{FileA: a.FilePath, FileB: b.FilePath}
				info, exists := pairMap[pair]
				if !exists {
					info = &clonePairInfo{
						StartLineA: a.StartLine,
						EndLineA:   a.EndLine,
						StartLineB: b.StartLine,
						EndLineB:   b.EndLine,
					}
					pairMap[pair] = info
				}

				// Expand line ranges.
				if a.StartLine < info.StartLineA {
					info.StartLineA = a.StartLine
				}
				if a.EndLine > info.EndLineA {
					info.EndLineA = a.EndLine
				}
				if b.StartLine < info.StartLineB {
					info.StartLineB = b.StartLine
				}
				if b.EndLine > info.EndLineB {
					info.EndLineB = b.EndLine
				}
				info.MatchCount++
			}
		}
	}

	// Filter out pairs below the minimum line threshold.
	for pair, info := range pairMap {
		spanA := info.EndLineA - info.StartLineA + 1
		spanB := info.EndLineB - info.StartLineB + 1
		if spanA < minLines && spanB < minLines {
			delete(pairMap, pair)
		}
	}

	return pairMap
}
