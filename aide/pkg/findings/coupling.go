package findings

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// compiledPattern holds a pre-compiled regex and its capture group index.
type compiledPattern struct {
	re      *regexp.Regexp
	group   int
	context string
}

// importPatternCache caches compiled regex patterns keyed by language name.
var (
	importPatternCache   = make(map[string][]compiledPattern)
	importPatternCacheMu sync.RWMutex
)

// CouplingConfig configures the coupling analyzer.
type CouplingConfig struct {
	// FanOutThreshold is the max imports before a file is flagged (default 15).
	FanOutThreshold int
	// FanInThreshold is the max reverse-dependencies before a file is flagged (default 20).
	FanInThreshold int
	// Paths to analyze (default: current directory).
	Paths []string
	// ProgressFn is called after each file is analyzed.
	ProgressFn func(path string, imports int)
	// Ignore is the aideignore matcher for filtering files/directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
}

// CouplingResult holds the output of a coupling analysis run.
type CouplingResult struct {
	FilesAnalyzed int
	FindingsCount int
	CyclesFound   int
	Duration      time.Duration
}

// importGraph represents the file-level import graph.
type importGraph struct {
	// edges maps file -> set of files it imports
	edges map[string]map[string]bool
	// reverse maps file -> set of files that import it
	reverse map[string]map[string]bool
}

func newImportGraph() *importGraph {
	return &importGraph{
		edges:   make(map[string]map[string]bool),
		reverse: make(map[string]map[string]bool),
	}
}

func (g *importGraph) addEdge(from, to string) {
	if g.edges[from] == nil {
		g.edges[from] = make(map[string]bool)
	}
	g.edges[from][to] = true

	if g.reverse[to] == nil {
		g.reverse[to] = make(map[string]bool)
	}
	g.reverse[to][from] = true
}

// AnalyzeCoupling analyzes import coupling between files.
// It reports files with high fan-out (too many imports), high fan-in
// (too many dependents), and import cycles.
func AnalyzeCoupling(cfg CouplingConfig) ([]*Finding, *CouplingResult, error) {
	if cfg.FanOutThreshold <= 0 {
		cfg.FanOutThreshold = 15
	}
	if cfg.FanInThreshold <= 0 {
		cfg.FanInThreshold = 20
	}
	if len(cfg.Paths) == 0 {
		cfg.Paths = []string{"."}
	}

	start := time.Now()
	result := &CouplingResult{}
	graph := newImportGraph()

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	// Phase 1: Build the import graph
	for _, root := range cfg.Paths {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, nil, fmt.Errorf("abs path %s: %w", root, err)
		}
		shouldSkip := ignore.WalkFunc(absRoot)

		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
				return nil
			}
			if !code.SupportedFile(path) {
				return nil
			}

			lang := code.DetectLanguage(path, nil)
			if lang == "" {
				return nil
			}

			relPath := path
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, path); err == nil {
					relPath = rel
				}
			}

			imports := extractImports(path, lang)
			for _, imp := range imports {
				graph.addEdge(relPath, imp)
			}

			result.FilesAnalyzed++

			if cfg.ProgressFn != nil {
				cfg.ProgressFn(relPath, len(imports))
			}

			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("walk %s: %w", root, err)
		}
	}

	var allFindings []*Finding

	// Phase 2: Detect high fan-out
	for file, imports := range graph.edges {
		fanOut := len(imports)
		if fanOut >= cfg.FanOutThreshold {
			severity := SevWarning
			if fanOut >= cfg.FanOutThreshold*2 {
				severity = SevCritical
			}

			importList := make([]string, 0, len(imports))
			for imp := range imports {
				importList = append(importList, imp)
			}
			sort.Strings(importList)
			detail := fmt.Sprintf("File imports %d modules (threshold: %d). High fan-out suggests this file has too many responsibilities. Consider splitting it.\n\nImports:\n", fanOut, cfg.FanOutThreshold)
			for _, imp := range importList {
				detail += fmt.Sprintf("  - %s\n", imp)
			}

			allFindings = append(allFindings, &Finding{
				Analyzer: AnalyzerCoupling,
				Severity: severity,
				Category: "fan-out",
				FilePath: file,
				Line:     1,
				Title:    fmt.Sprintf("High fan-out: %d imports", fanOut),
				Detail:   detail,
				Metadata: map[string]string{
					"fan_out":   strconv.Itoa(fanOut),
					"threshold": strconv.Itoa(cfg.FanOutThreshold),
				},
				CreatedAt: time.Now(),
			})
		}
	}

	// Phase 3: Detect high fan-in
	for file, dependents := range graph.reverse {
		fanIn := len(dependents)
		if fanIn >= cfg.FanInThreshold {
			severity := SevInfo
			if fanIn >= cfg.FanInThreshold*2 {
				severity = SevWarning
			}

			depList := make([]string, 0, len(dependents))
			for dep := range dependents {
				depList = append(depList, dep)
			}
			sort.Strings(depList)
			detail := fmt.Sprintf("File is imported by %d other files (threshold: %d). High fan-in means changes to this file have wide impact.\n\nDepended on by:\n", fanIn, cfg.FanInThreshold)
			for _, dep := range depList {
				detail += fmt.Sprintf("  - %s\n", dep)
			}

			allFindings = append(allFindings, &Finding{
				Analyzer: AnalyzerCoupling,
				Severity: severity,
				Category: "fan-in",
				FilePath: file,
				Line:     1,
				Title:    fmt.Sprintf("High fan-in: %d dependents", fanIn),
				Detail:   detail,
				Metadata: map[string]string{
					"fan_in":    strconv.Itoa(fanIn),
					"threshold": strconv.Itoa(cfg.FanInThreshold),
				},
				CreatedAt: time.Now(),
			})
		}
	}

	// Phase 4: Detect cycles using Tarjan's algorithm
	cycles := findCycles(graph)
	result.CyclesFound = len(cycles)

	for i, cycle := range cycles {
		if i >= 50 { // Cap at 50 cycle findings
			break
		}
		cycleStr := strings.Join(cycle, " -> ") + " -> " + cycle[0]

		allFindings = append(allFindings, &Finding{
			Analyzer: AnalyzerCoupling,
			Severity: SevWarning,
			Category: "cycle",
			FilePath: cycle[0],
			Line:     1,
			Title:    fmt.Sprintf("Import cycle (%d files)", len(cycle)),
			Detail:   fmt.Sprintf("Circular dependency detected:\n  %s\n\nCycles make code harder to understand, test, and refactor.", cycleStr),
			Metadata: map[string]string{
				"cycle_length": strconv.Itoa(len(cycle)),
				"cycle_files":  strings.Join(cycle, ","),
			},
			CreatedAt: time.Now(),
		})
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)
	return allFindings, result, nil
}

// findCycles finds all strongly connected components with size > 1 using Tarjan's algorithm.
func findCycles(graph *importGraph) [][]string {
	var (
		index    int
		stack    []string
		onStack  = make(map[string]bool)
		indices  = make(map[string]int)
		lowlinks = make(map[string]int)
		sccs     [][]string
	)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlinks[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for w := range graph.edges[v] {
			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlinks[w] < lowlinks[v] {
					lowlinks[v] = lowlinks[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlinks[v] {
					lowlinks[v] = indices[w]
				}
			}
		}

		// If v is a root node, pop the SCC
		if lowlinks[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			if len(scc) > 1 { // Only report actual cycles
				sort.Strings(scc)
				sccs = append(sccs, scc)
			}
		}
	}

	// Collect all nodes
	nodes := make(map[string]bool)
	for k := range graph.edges {
		nodes[k] = true
	}
	for k := range graph.reverse {
		nodes[k] = true
	}

	for node := range nodes {
		if _, visited := indices[node]; !visited {
			strongConnect(node)
		}
	}

	return sccs
}

// =============================================================================
// Import extraction per language
// =============================================================================

// extractImports returns a list of import paths from a file.
// Import paths are normalised to just the module/package name, not full paths.
// Uses pack registry import patterns for extraction.
func extractImports(filePath, lang string) []string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	if pack := grammar.DefaultPackRegistry().Get(lang); pack != nil && pack.Imports != nil && len(pack.Imports.Patterns) > 0 {
		return extractImportsFromPack(content, lang, pack.Imports)
	}

	return nil
}

// getCachedPatterns returns compiled regex patterns for a language, caching them
// so they are only compiled once per language rather than once per file.
func getCachedPatterns(lang string, imports *grammar.PackImports) []compiledPattern {
	importPatternCacheMu.RLock()
	if cached, ok := importPatternCache[lang]; ok {
		importPatternCacheMu.RUnlock()
		return cached
	}
	importPatternCacheMu.RUnlock()

	importPatternCacheMu.Lock()
	defer importPatternCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := importPatternCache[lang]; ok {
		return cached
	}

	compiled := make([]compiledPattern, 0, len(imports.Patterns))
	for _, p := range imports.Patterns {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledPattern{re: re, group: p.Group, context: p.Context})
	}
	importPatternCache[lang] = compiled
	return compiled
}

// extractImportsFromPack uses pack.json import patterns to extract imports
// generically. Handles block_start/block_end for languages like Go that have
// multi-line import blocks. Compiled regex patterns are cached per language.
func extractImportsFromPack(content []byte, lang string, imports *grammar.PackImports) []string {
	compiled := getCachedPatterns(lang, imports)
	if len(compiled) == 0 {
		return nil
	}

	var result []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()

		// Handle block boundaries if defined.
		if imports.BlockStart != "" {
			if !inBlock && strings.Contains(line, imports.BlockStart) {
				inBlock = true
				continue
			}
			if inBlock && strings.TrimSpace(line) == imports.BlockEnd {
				inBlock = false
				continue
			}
		}

		for _, cp := range compiled {
			// Filter by context: "single" patterns only match outside blocks,
			// "block" patterns only match inside blocks, empty matches anywhere.
			if cp.context == "single" && inBlock {
				continue
			}
			if cp.context == "block" && !inBlock {
				continue
			}

			m := cp.re.FindStringSubmatch(line)
			if m != nil && cp.group < len(m) {
				result = append(result, m[cp.group])
				break // first match wins per line
			}
		}
	}
	return result
}
