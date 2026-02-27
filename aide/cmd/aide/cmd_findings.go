package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/findings/clone"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// getFindingsStorePath returns the directory for findings data.
func getFindingsStorePath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "findings")
}

// cmdFindingsDispatcher routes findings subcommands.
func cmdFindingsDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printFindingsUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "run":
		return cmdFindingsRun(dbPath, subargs)
	case "search":
		return cmdFindingsSearch(dbPath, subargs)
	case "list":
		return cmdFindingsList(dbPath, subargs)
	case "stats":
		return cmdFindingsStats(dbPath)
	case "clear":
		return cmdFindingsClear(dbPath, subargs)
	case "help", "-h", "--help":
		printFindingsUsage()
		return nil
	default:
		return fmt.Errorf("unknown findings subcommand: %s", subcmd)
	}
}

func printFindingsUsage() {
	fmt.Println(`aide findings - Run analysers and manage static analysis findings

Usage:
  aide findings <subcommand> [arguments]

Subcommands:
  run        Run one or more static analysers
  search     Search findings by keyword
  list       List findings with optional filters
  stats      Show finding statistics
  clear      Clear findings (all or by analyser)

Options:
  run <analyser> [paths...]:
    Analysers: complexity, coupling, secrets, clones, all
    --threshold=N    Complexity threshold (default 10)
    --fan-out=N      Coupling fan-out threshold (default 15)
    --fan-in=N       Coupling fan-in threshold (default 20)
    --window=N       Clone window size in tokens (default 50)
    --min-lines=N    Clone minimum line span (default 6)
    --no-validate    Secrets: skip live validation (default)

  search <query>:
    --analyser=NAME  Filter by analyser (complexity, coupling, secrets, clones)
    --severity=LEVEL Filter by severity (critical, warning, info)
    --file=PATH      Filter by file path pattern (substring)
    --category=CAT   Filter by category
    --limit=N        Max results (default 20, 0 for no limit)
    --json           Output as JSON

  list:
    --analyser=NAME  Filter by analyser
    --severity=LEVEL Filter by severity
    --file=PATH      Filter by file path pattern
    --category=CAT   Filter by category
    --limit=N        Max results (default 100, 0 for no limit)
    --json           Output as JSON

  clear [--analyser=NAME]:
    Clears all findings, or only findings for the specified analyser.

Note: --analyzer is accepted as an alias for --analyser.

Examples:
  aide findings run complexity .
  aide findings run all src/
  aide findings run secrets --no-validate .
  aide findings stats
  aide findings list --analyser=complexity --severity=critical
  aide findings search "cyclomatic"
  aide findings list --file=src/auth
  aide findings clear --analyser=secrets`)
}

// cmdFindingsRun runs one or more static analyzers and stores findings.
func cmdFindingsRun(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide findings run <analyser|all> [paths...] [options]")
	}

	analyzerName := args[0]
	subargs := args[1:]

	// Parse paths (non-flag arguments).
	var paths []string
	for _, arg := range subargs {
		if !strings.HasPrefix(arg, "--") {
			paths = append(paths, arg)
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Parse common options.
	// Defaults come from .aide/config/aide.json, falling back to hardcoded values.
	// CLI flags override everything.
	projectRoot := projectRoot(dbPath)
	cfg := loadFindingsConfig(projectRoot)

	threshold := 10
	if cfg.Complexity.Threshold > 0 {
		threshold = cfg.Complexity.Threshold
	}
	if t := parseFlag(subargs, "--threshold="); t != "" {
		fmt.Sscanf(t, "%d", &threshold)
	}
	fanOut := 15
	if cfg.Coupling.FanOut > 0 {
		fanOut = cfg.Coupling.FanOut
	}
	if f := parseFlag(subargs, "--fan-out="); f != "" {
		fmt.Sscanf(f, "%d", &fanOut)
	}
	fanIn := 20
	if cfg.Coupling.FanIn > 0 {
		fanIn = cfg.Coupling.FanIn
	}
	if f := parseFlag(subargs, "--fan-in="); f != "" {
		fmt.Sscanf(f, "%d", &fanIn)
	}
	windowSize := 50
	if cfg.Clones.WindowSize > 0 {
		windowSize = cfg.Clones.WindowSize
	}
	if w := parseFlag(subargs, "--window="); w != "" {
		fmt.Sscanf(w, "%d", &windowSize)
	}
	minCloneLines := 6
	if cfg.Clones.MinLines > 0 {
		minCloneLines = cfg.Clones.MinLines
	}
	if m := parseFlag(subargs, "--min-lines="); m != "" {
		fmt.Sscanf(m, "%d", &minCloneLines)
	}

	// Determine which analyzers to run.
	analyzers := []string{analyzerName}
	if analyzerName == "all" {
		analyzers = []string{
			findings.AnalyzerComplexity,
			findings.AnalyzerCoupling,
			findings.AnalyzerSecrets,
			findings.AnalyzerClones,
		}
	}

	// Open backend for storing results.
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// Load .aideignore from project root.
	ignore, err := aideignore.New(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load .aideignore: %w", err)
	}

	// Create a properly-configured grammar loader for analysers that need tree-sitter.
	loader := newGrammarLoader(dbPath, nil)

	totalFindings := 0

	for _, name := range analyzers {
		switch name {
		case findings.AnalyzerComplexity:
			n, err := runComplexityAnalyzer(backend, paths, threshold, ignore, loader)
			if err != nil {
				return fmt.Errorf("complexity analyser failed: %w", err)
			}
			totalFindings += n

		case findings.AnalyzerCoupling:
			n, err := runCouplingAnalyzer(backend, paths, fanOut, fanIn, ignore)
			if err != nil {
				return fmt.Errorf("coupling analyser failed: %w", err)
			}
			totalFindings += n

		case findings.AnalyzerSecrets:
			n, err := runSecretsAnalyzer(backend, paths, ignore)
			if err != nil {
				return fmt.Errorf("secrets analyser failed: %w", err)
			}
			totalFindings += n

		case findings.AnalyzerClones:
			n, err := runClonesAnalyzer(backend, paths, windowSize, minCloneLines, ignore, loader)
			if err != nil {
				return fmt.Errorf("clones analyser failed: %w", err)
			}
			totalFindings += n

		default:
			return fmt.Errorf("unknown analyser: %s (valid: complexity, coupling, secrets, clones, all)", name)
		}
	}

	fmt.Printf("\nTotal: %d findings stored\n", totalFindings)
	return nil
}

func runComplexityAnalyzer(backend *Backend, paths []string, threshold int, ignore *aideignore.Matcher, loader grammar.Loader) (int, error) {
	fmt.Printf("Running complexity analyser (threshold=%d)...\n", threshold)

	cfg := findings.ComplexityConfig{
		Threshold: threshold,
		Paths:     paths,
		Ignore:    ignore,
		Loader:    loader,
		ProgressFn: func(path string, count int) {
			if count > 0 {
				fmt.Printf("  %s: %d findings\n", path, count)
			}
		},
	}

	ff, result, err := findings.AnalyzeComplexity(cfg)
	if err != nil {
		return 0, err
	}

	fmt.Printf("  Analysed %d files, skipped %d, found %d issues (%s)\n",
		result.FilesAnalyzed, result.FilesSkipped, result.FindingsCount, result.Duration.Round(1_000_000))

	if err := backend.ReplaceFindingsForAnalyzer(findings.AnalyzerComplexity, ff); err != nil {
		return 0, fmt.Errorf("failed to store findings: %w", err)
	}

	return len(ff), nil
}

func runCouplingAnalyzer(backend *Backend, paths []string, fanOut, fanIn int, ignore *aideignore.Matcher) (int, error) {
	fmt.Printf("Running coupling analyser (fan-out=%d, fan-in=%d)...\n", fanOut, fanIn)

	cfg := findings.CouplingConfig{
		FanOutThreshold: fanOut,
		FanInThreshold:  fanIn,
		Paths:           paths,
		Ignore:          ignore,
	}

	ff, result, err := findings.AnalyzeCoupling(cfg)
	if err != nil {
		return 0, err
	}

	fmt.Printf("  Analysed %d files, found %d issues, %d cycles (%s)\n",
		result.FilesAnalyzed, result.FindingsCount, result.CyclesFound, result.Duration.Round(1_000_000))

	if err := backend.ReplaceFindingsForAnalyzer(findings.AnalyzerCoupling, ff); err != nil {
		return 0, fmt.Errorf("failed to store findings: %w", err)
	}

	return len(ff), nil
}

func runSecretsAnalyzer(backend *Backend, paths []string, ignore *aideignore.Matcher) (int, error) {
	fmt.Printf("Running secrets analyser...\n")

	cfg := findings.SecretsConfig{
		Paths:          paths,
		SkipValidation: true,
		Ignore:         ignore,
	}

	ff, result, err := findings.AnalyzeSecrets(cfg)
	if err != nil {
		return 0, err
	}

	fmt.Printf("  Scanned %d files (skipped %d), %d rules, found %d secrets (%s)\n",
		result.FilesScanned, result.FilesSkipped, result.RulesLoaded, result.FindingsCount, result.Duration.Round(1_000_000))

	if err := backend.ReplaceFindingsForAnalyzer(findings.AnalyzerSecrets, ff); err != nil {
		return 0, fmt.Errorf("failed to store findings: %w", err)
	}

	return len(ff), nil
}

func runClonesAnalyzer(backend *Backend, paths []string, windowSize, minLines int, ignore *aideignore.Matcher, loader grammar.Loader) (int, error) {
	fmt.Printf("Running clone detection (window=%d, min-lines=%d)...\n", windowSize, minLines)

	cfg := clone.Config{
		WindowSize:    windowSize,
		MinCloneLines: minLines,
		Paths:         paths,
		Ignore:        ignore,
		Loader:        loader,
	}

	ff, result, err := clone.DetectClones(cfg)
	if err != nil {
		return 0, err
	}

	fmt.Printf("  Analyzed %d files (skipped %d), %d clone groups, %d findings (%s)\n",
		result.FilesAnalyzed, result.FilesSkipped, result.CloneGroups, result.FindingsCount, result.Duration.Round(1_000_000))

	if err := backend.ReplaceFindingsForAnalyzer(findings.AnalyzerClones, ff); err != nil {
		return 0, fmt.Errorf("failed to store findings: %w", err)
	}

	return len(ff), nil
}

func cmdFindingsSearch(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide findings search <query> [--analyser=NAME] [--severity=LEVEL] [--limit=N]")
	}

	// Parse query (first non-flag argument)
	var query string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			query = arg
			break
		}
	}
	if query == "" {
		return fmt.Errorf("query is required")
	}

	// Parse options
	analyzer := parseFlag(args, "--analyzer=")
	severity := parseFlag(args, "--severity=")
	filePath := parseFlag(args, "--file=")
	category := parseFlag(args, "--category=")
	limit := 20
	if l := parseFlag(args, "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	jsonOutput := hasFlag(args, "--json")

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// limit=0 means no limit; negative values from the store mean unlimited.
	var storeLimit int
	if limit <= 0 {
		storeLimit = -1
	} else {
		storeLimit = limit + 1 // Fetch one extra to detect truncation.
	}

	opts := findings.SearchOptions{
		Analyzer: analyzer,
		Severity: severity,
		FilePath: filePath,
		Category: category,
		Limit:    storeLimit,
	}

	results, err := backend.SearchFindings(query, opts)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No findings found")
		return nil
	}

	truncated := limit > 0 && len(results) > limit
	if truncated {
		results = results[:limit]
	}

	if jsonOutput {
		fmt.Print("[")
		for i, r := range results {
			if i > 0 {
				fmt.Print(",")
			}
			f := r.Finding
			printFindingJSON(f)
		}
		fmt.Println("]")
	} else {
		if truncated {
			fmt.Printf("Found more than %d findings. Showing the first %d (use --limit=N to see more):\n\n", limit, limit)
		} else {
			fmt.Printf("Found %d findings:\n\n", len(results))
		}
		for _, r := range results {
			printFindingLine(r.Finding)
		}
	}

	return nil
}

func cmdFindingsList(dbPath string, args []string) error {
	analyzer := parseFlag(args, "--analyzer=")
	severity := parseFlag(args, "--severity=")
	filePath := parseFlag(args, "--file=")
	category := parseFlag(args, "--category=")
	limit := 100
	if l := parseFlag(args, "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	jsonOutput := hasFlag(args, "--json")

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// limit=0 means no limit; negative values from the store mean unlimited.
	var storeLimit int
	if limit <= 0 {
		storeLimit = -1
	} else {
		storeLimit = limit + 1 // Fetch one extra to detect truncation.
	}

	opts := findings.SearchOptions{
		Analyzer: analyzer,
		Severity: severity,
		FilePath: filePath,
		Category: category,
		Limit:    storeLimit,
	}

	results, err := backend.ListFindings(opts)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No findings found")
		return nil
	}

	truncated := limit > 0 && len(results) > limit
	if truncated {
		results = results[:limit]
	}

	if jsonOutput {
		fmt.Print("[")
		for i, f := range results {
			if i > 0 {
				fmt.Print(",")
			}
			printFindingJSON(f)
		}
		fmt.Println("]")
	} else {
		if truncated {
			fmt.Printf("Found more than %d findings. Showing the first %d (use --limit=N to see more):\n\n", limit, limit)
		} else {
			fmt.Printf("Found %d findings:\n\n", len(results))
		}
		for _, f := range results {
			printFindingLine(f)
		}
	}

	return nil
}

func cmdFindingsStats(dbPath string) error {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	stats, err := backend.GetFindingsStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Printf("Findings Statistics:\n")
	fmt.Printf("  Total: %d\n\n", stats.Total)

	if len(stats.ByAnalyzer) > 0 {
		fmt.Printf("  By analyser:\n")
		for name, count := range stats.ByAnalyzer {
			fmt.Printf("    %-12s %d\n", name, count)
		}
		fmt.Println()
	}

	if len(stats.BySeverity) > 0 {
		fmt.Printf("  By severity:\n")
		for sev, count := range stats.BySeverity {
			fmt.Printf("    %-12s %d\n", sev, count)
		}
	}

	return nil
}

func cmdFindingsClear(dbPath string, args []string) error {
	analyzer := parseFlag(args, "--analyzer=")

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	if analyzer != "" {
		count, err := backend.ClearFindingsAnalyzer(analyzer)
		if err != nil {
			return fmt.Errorf("failed to clear analyser findings: %w", err)
		}
		fmt.Printf("Cleared %d findings for analyser '%s'\n", count, analyzer)
	} else {
		if err := backend.ClearFindings(); err != nil {
			return fmt.Errorf("failed to clear findings: %w", err)
		}
		fmt.Println("All findings cleared")
	}

	return nil
}

// printFindingLine prints a human-readable single-line summary of a finding.
func printFindingLine(f *findings.Finding) {
	sev := strings.ToUpper(f.Severity)
	loc := f.FilePath
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.FilePath, f.Line)
	}
	sevPad := padString(fmt.Sprintf("[%s]", sev), 12)
	locPad := padString(loc, 40)
	fmt.Printf("  %s %s %s (%s)\n", sevPad, locPad, f.Title, f.Analyzer)
}

// printFindingJSON prints a single finding as a JSON object (no trailing newline).
func printFindingJSON(f *findings.Finding) {
	fmt.Printf(`{"id":"%s","analyzer":"%s","severity":"%s","file":"%s","line":%d,"title":"%s"}`,
		escapeJSON(f.ID),
		escapeJSON(f.Analyzer),
		escapeJSON(f.Severity),
		escapeJSON(f.FilePath),
		f.Line,
		escapeJSON(f.Title))
}
