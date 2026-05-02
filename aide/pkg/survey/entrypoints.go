package survey

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// EntrypointsResult holds the output of the entrypoints analyzer.
type EntrypointsResult struct {
	Entries []*Entry
}

// RunEntrypoints uses the code index to find entry points in the codebase.
// If codeSearcher is nil, it falls back to file-scanning heuristics.
// Entry point patterns are loaded from grammar pack.json files (PackEntrypoints).
func RunEntrypoints(rootDir string, codeSearcher CodeSearcher) (*EntrypointsResult, error) {
	return RunEntrypointsWithRegistry(rootDir, codeSearcher, grammar.DefaultPackRegistry())
}

// RunEntrypointsWithRegistry is like RunEntrypoints but accepts a specific PackRegistry.
func RunEntrypointsWithRegistry(rootDir string, codeSearcher CodeSearcher, reg *grammar.PackRegistry) (*EntrypointsResult, error) {
	result := &EntrypointsResult{}

	// Iterate all packs with entrypoints defined.
	for _, name := range reg.All() {
		pack := reg.Get(name)
		if pack == nil || pack.Entrypoints == nil {
			continue
		}

		if codeSearcher != nil {
			// Code-index-based detection (preferred — more accurate).
			result.processSymbols(pack, codeSearcher)
			result.processRefs(pack, codeSearcher)
		} else {
			// Fallback: file-scan patterns.
			result.processFilePatterns(rootDir, pack)
		}
	}

	AnnotateEstTokens(rootDir, result.Entries)
	return result, nil
}

// processSymbols searches the code index for entry point symbols defined in a pack.
func (r *EntrypointsResult) processSymbols(pack *grammar.Pack, cs CodeSearcher) {
	// Build set of file extensions for this language to filter results.
	langExts := make(map[string]bool, len(pack.Meta.Extensions))
	for _, ext := range pack.Meta.Extensions {
		langExts[ext] = true
	}

	for _, sym := range pack.Entrypoints.Symbols {
		results, err := cs.FindSymbols(sym.Name, sym.Kind, 100)
		if err != nil {
			continue
		}

		// Compile exclude regex if present.
		var excludeRe *regexp.Regexp
		if sym.Exclude != "" {
			excludeRe, _ = regexp.Compile(sym.Exclude)
		}

		// Compile name_match regex if present.
		var nameMatchRe *regexp.Regexp
		if sym.NameMatch != "" {
			nameMatchRe, _ = regexp.Compile(sym.NameMatch)
		}

		// Compile file_match regex if present.
		var fileMatchRe *regexp.Regexp
		if sym.FileMatch != "" {
			fileMatchRe, _ = regexp.Compile(sym.FileMatch)
		}

		for _, hit := range results {
			// Exact name match (FindSymbols may return partial matches).
			if hit.Name != sym.Name {
				continue
			}
			// Kind filter.
			if sym.Kind != "" && hit.Kind != sym.Kind {
				continue
			}
			// Language filter: only process files belonging to this pack's language.
			if len(langExts) > 0 {
				ext := filepath.Ext(hit.FilePath)
				if !langExts[ext] {
					continue
				}
			}
			// Universal safety-net filters: always exclude test and generated files.
			if isTestFile(pack, hit.FilePath) || isGeneratedFile(pack, hit.FilePath) {
				continue
			}
			// Exclude filter (pack-specific patterns).
			if excludeRe != nil && excludeRe.MatchString(hit.FilePath) {
				continue
			}
			// Name match filter (on full symbol name).
			if nameMatchRe != nil && !nameMatchRe.MatchString(hit.Name) {
				continue
			}
			// File match filter.
			if fileMatchRe != nil && !fileMatchRe.MatchString(hit.FilePath) {
				continue
			}

			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:%s()", hit.FilePath, hit.Name),
				FilePath: hit.FilePath,
				Title:    sym.Label,
				Detail:   fmt.Sprintf("%s at %s:%d", hit.Name, hit.FilePath, hit.Line),
				Metadata: map[string]string{
					"language": pack.Name,
					"type":     sym.Type,
					"line":     fmt.Sprintf("%d", hit.Line),
				},
			})
		}
	}
}

// processRefs searches the code index for entry point references defined in a pack.
func (r *EntrypointsResult) processRefs(pack *grammar.Pack, cs CodeSearcher) {
	// Build set of file extensions for this language to filter results.
	langExts := make(map[string]bool, len(pack.Meta.Extensions))
	for _, ext := range pack.Meta.Extensions {
		langExts[ext] = true
	}

	for _, ref := range pack.Entrypoints.Refs {
		refKind := ref.RefKind
		if refKind == "" {
			refKind = "call"
		}

		results, err := cs.FindReferences(ref.Name, refKind, 100)
		if err != nil {
			continue
		}

		// Compile qualifier regex if present.
		var qualifierRe *regexp.Regexp
		if ref.Qualifier != "" {
			qualifierRe, _ = regexp.Compile(ref.Qualifier)
		}

		// Compile name_match regex if present.
		var nameMatchRe *regexp.Regexp
		if ref.NameMatch != "" {
			nameMatchRe, _ = regexp.Compile(ref.NameMatch)
		}

		// Compile exclude regex if present.
		var excludeRe *regexp.Regexp
		if ref.Exclude != "" {
			excludeRe, _ = regexp.Compile(ref.Exclude)
		}

		// Dedup strategy: when name_match is present, deduplicate by matched symbol name
		// (e.g., gRPC RegisterUserServiceServer registered in multiple files counts as one).
		// Otherwise, deduplicate by file:line to avoid duplicate entries for the same call site.
		seen := make(map[string]bool)

		for _, hit := range results {
			// Language filter: only process files belonging to this pack's language.
			if len(langExts) > 0 {
				ext := filepath.Ext(hit.FilePath)
				if !langExts[ext] {
					continue
				}
			}

			// Universal safety-net filters: always exclude test and generated files.
			if isTestFile(pack, hit.FilePath) || isGeneratedFile(pack, hit.FilePath) {
				continue
			}

			// Exclude filter (pack-specific patterns).
			if excludeRe != nil && excludeRe.MatchString(hit.FilePath) {
				continue
			}

			// Name match: apply regex to the full symbol name.
			if nameMatchRe != nil && !nameMatchRe.MatchString(hit.Symbol) {
				continue
			}

			// Extract base name and qualifier from symbol (e.g., "http.HandleFunc" -> qualifier="http", base="HandleFunc").
			baseName := hit.Symbol
			qualifierStr := ""
			if idx := strings.LastIndex(hit.Symbol, "."); idx >= 0 {
				qualifierStr = hit.Symbol[:idx]
				baseName = hit.Symbol[idx+1:]
			}

			// Check base name matches the ref name (exact match for the base).
			if baseName != ref.Name && (nameMatchRe == nil) {
				continue
			}

			// Qualifier filter: if specified, the qualifier portion must match.
			if qualifierRe != nil {
				if qualifierStr == "" {
					// No qualifier present — for "Handle" with qualifier filter, accept standalone calls.
					// This matches the old isHTTPSymbol behavior.
				} else if !qualifierRe.MatchString(qualifierStr) {
					continue
				}
			}

			// Dedup: by symbol name when name_match is present, otherwise by file:line.
			var key string
			if nameMatchRe != nil {
				key = hit.Symbol
			} else {
				key = fmt.Sprintf("%s:%d", hit.FilePath, hit.Line)
			}
			if seen[key] {
				continue
			}
			seen[key] = true

			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:%s", hit.FilePath, hit.Symbol),
				FilePath: hit.FilePath,
				Title:    fmt.Sprintf("%s: %s", ref.Label, hit.Symbol),
				Detail:   fmt.Sprintf("%s at %s:%d", hit.Symbol, hit.FilePath, hit.Line),
				Metadata: map[string]string{
					"language": pack.Name,
					"type":     ref.Type,
					"line":     fmt.Sprintf("%d", hit.Line),
				},
			})
		}
	}
}

// processFilePatterns walks the filesystem for entry point patterns when no code index
// is available. Each pack's FilePatterns define glob matches and optional content regexes.
// Directory pruning is delegated to aideignore (which loads .aideignore plus
// BuiltinDefaults, covering vendor, node_modules, target, etc.) plus a hidden-dir guard.
func (r *EntrypointsResult) processFilePatterns(rootDir string, pack *grammar.Pack) {
	ignore, _ := aideignore.New(rootDir)
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}
	shouldSkip := ignore.WalkFunc(rootDir)

	for _, fp := range pack.Entrypoints.FilePatterns {
		// Compile content regex if present.
		var contentRe *regexp.Regexp
		if fp.Content != "" {
			contentRe, _ = regexp.Compile(fp.Content)
		}

		// Compile pre_content regex if present.
		var preContentRe *regexp.Regexp
		if fp.PreContent != "" {
			preContentRe, _ = regexp.Compile(fp.PreContent)
		}

		_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				base := info.Name()
				if base != "." && strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				return nil
			}

			relPath, _ := filepath.Rel(rootDir, path)
			if relPath == "" {
				relPath = path
			}

			// Exclude test and generated files.
			if isTestFile(pack, relPath) || isGeneratedFile(pack, relPath) {
				return nil
			}

			// Match file pattern (simple glob: *.go, main.rs, index.js, etc.)
			matched, _ := filepath.Match(fp.FileMatch, filepath.Base(path))
			if !matched {
				return nil
			}

			// If no content regex, the file's existence is sufficient (e.g., main.rs).
			if contentRe == nil {
				r.Entries = append(r.Entries, &Entry{
					Analyzer: AnalyzerEntrypoints,
					Kind:     KindEntrypoint,
					Name:     fmt.Sprintf("%s:entrypoint", relPath),
					FilePath: relPath,
					Title:    fp.Label,
					Detail:   fmt.Sprintf("%s at %s (detected by file scan)", fp.Label, relPath),
					Metadata: map[string]string{
						"language":  pack.Name,
						"type":      fp.Type,
						"detection": "file_scan",
					},
				})
				return nil
			}

			// Scan file content for the pattern.
			r.scanFileForPattern(relPath, path, pack.Name, fp, contentRe, preContentRe)
			return nil
		})
	}
}

// scanFileForPattern scans a file line-by-line for a content regex match.
// If preContentRe is non-nil, the content match is only valid if preContentRe
// matched at least one earlier line in the file (e.g., "package main" before "func main()").
func (r *EntrypointsResult) scanFileForPattern(relPath, absPath, language string, fp grammar.EntrypointFilePattern, contentRe *regexp.Regexp, preContentRe *regexp.Regexp) {
	f, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	preContentSatisfied := preContentRe == nil // satisfied by default if no precondition
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check precondition.
		if preContentRe != nil && !preContentSatisfied {
			if preContentRe.MatchString(line) {
				preContentSatisfied = true
			}
		}

		if contentRe.MatchString(line) {
			if !preContentSatisfied {
				continue // Content matched but precondition not yet satisfied.
			}
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:%s", relPath, fp.Type),
				FilePath: relPath,
				Title:    fp.Label,
				Detail:   fmt.Sprintf("%s at %s:%d (detected by file scan)", fp.Label, relPath, lineNum),
				Metadata: map[string]string{
					"language":  language,
					"type":      fp.Type,
					"line":      fmt.Sprintf("%d", lineNum),
					"detection": "file_scan",
				},
			})
			return // One match per file
		}
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// isGeneratedFile returns true when the path matches a pack-declared
// generated-file glob, or a universal heuristic ("generated" path component,
// "vendor/" tree). Pack is optional — when nil only the universals fire.
func isGeneratedFile(pack *grammar.Pack, path string) bool {
	if pack != nil && pack.Files != nil {
		for _, p := range pack.Files.GeneratedFilePatterns {
			if matched, _ := doublestar.PathMatch(p, path); matched {
				return true
			}
		}
	}
	if strings.Contains(path, "generated") || strings.Contains(path, "vendor/") {
		return true
	}
	return false
}

// isTestFile returns true when the path matches a pack-declared test-file
// glob, or sits under a universal testdata/ component. Pack is optional —
// when nil only the universal fallback fires.
func isTestFile(pack *grammar.Pack, path string) bool {
	if pack != nil && pack.Deadcode != nil {
		for _, p := range pack.Deadcode.TestFilePatterns {
			if matched, _ := doublestar.PathMatch(p, path); matched {
				return true
			}
		}
	}
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "testdata" {
			return true
		}
	}
	return false
}
