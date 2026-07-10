package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/jmylchreest/aide/aide/pkg/surveyrun"
)

// getSurveyStorePath returns the directory for survey data.
func getSurveyStorePath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "survey")
}

// cmdSurveyDispatcher routes survey subcommands.
func cmdSurveyDispatcher(dbPath string, args []string) error {
	return dispatchSubcmd("survey", args, printSurveyUsage, []subcmd{
		{name: "search", handler: func(a []string) error { return cmdSurveySearch(dbPath, a) }},
		{name: "list", handler: func(a []string) error { return cmdSurveyList(dbPath, a) }},
		{name: "stats", handler: func(a []string) error { return cmdSurveyStats(dbPath, a) }},
		{name: "run", handler: func(a []string) error { return cmdSurveyRun(dbPath, a) }},
		{name: "graph", handler: func(a []string) error { return cmdSurveyGraph(dbPath, a) }},
		{name: "clear", handler: func(a []string) error { return cmdSurveyClear(dbPath, a) }},
	})
}

// cmdSurveySearch searches survey entries by query.
func cmdSurveySearch(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide survey search <query> [--analyzer=<name>] [--kind=<kind>] [--file=<path>] [--limit=<n>] [--json]")
	}

	query := args[0]
	opts := survey.SearchOptions{
		Analyzer: parseFlag(args[1:], "--analyzer="),
		Kind:     parseFlag(args[1:], "--kind="),
		FilePath: parseFlag(args[1:], "--file="),
	}
	if v := parseFlag(args[1:], "--limit="); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.Limit = n
		}
	}
	jsonOutput := hasFlag(args[1:], "--json")

	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	results, err := b.SearchSurvey(query, opts)
	if err != nil {
		return err
	}

	if jsonOutput {
		entries := make([]*survey.Entry, 0, len(results))
		for _, r := range results {
			entries = append(entries, r.Entry)
		}
		return printJSON(entries)
	}

	if len(results) == 0 {
		fmt.Println("No survey entries found.")
		return nil
	}

	for _, r := range results {
		printSurveyEntry(r.Entry)
		fmt.Println()
	}
	fmt.Printf("(%d results)\n", len(results))
	return nil
}

// cmdSurveyList lists survey entries with optional filters.
func cmdSurveyList(dbPath string, args []string) error {
	opts := survey.SearchOptions{
		Analyzer: parseFlag(args, "--analyzer="),
		Kind:     parseFlag(args, "--kind="),
		FilePath: parseFlag(args, "--file="),
	}
	if v := parseFlag(args, "--limit="); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.Limit = n
		}
	}
	jsonOutput := hasFlag(args, "--json")

	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	entries, err := b.ListSurvey(opts)
	if err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(entries)
	}

	if len(entries) == 0 {
		fmt.Println("No survey entries found.")
		return nil
	}

	for _, e := range entries {
		printSurveyEntry(e)
		fmt.Println()
	}
	fmt.Printf("(%d entries)\n", len(entries))
	return nil
}

// cmdSurveyStats shows survey statistics.
func cmdSurveyStats(dbPath string, _ []string) error {
	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	stats, err := b.GetSurveyStats(survey.SearchOptions{})
	if err != nil {
		return err
	}

	fmt.Printf("Survey Statistics:\n")
	fmt.Printf("  Total entries: %d\n", stats.Total)

	if len(stats.ByAnalyzer) > 0 {
		fmt.Println("  By analyzer:")
		for k, v := range stats.ByAnalyzer {
			fmt.Printf("    %-15s %d\n", k, v)
		}
	}

	if len(stats.ByKind) > 0 {
		fmt.Println("  By kind:")
		for k, v := range stats.ByKind {
			fmt.Printf("    %-15s %d\n", k, v)
		}
	}

	if lines := surveyFreshnessLines(projectRoot(dbPath), stats.ByAnalyzer, func(analyzer string) []*survey.Entry {
		entries, err := b.ListSurvey(survey.SearchOptions{Analyzer: analyzer, Limit: 1})
		if err != nil {
			return nil
		}
		return entries
	}); len(lines) > 0 {
		fmt.Println("  Freshness (vs git HEAD):")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

// cmdSurveyClear clears survey entries, optionally filtered by analyzer.
func cmdSurveyClear(dbPath string, args []string) error {
	analyzer := parseFlag(args, "--analyzer=")

	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	if analyzer != "" {
		count, err := b.ClearSurveyAnalyzer(analyzer)
		if err != nil {
			return err
		}
		fmt.Printf("Cleared %d survey entries from analyzer %q.\n", count, analyzer)
		return nil
	}

	if err := b.ClearSurvey(); err != nil {
		return err
	}
	fmt.Println("All survey entries cleared.")
	return nil
}

// printSurveyEntry formats and prints a single survey entry.
func printSurveyEntry(e *survey.Entry) {
	fmt.Printf("[%s] %s (%s/%s)\n", e.ID, e.Title, e.Analyzer, e.Kind)
	if e.Name != "" {
		fmt.Printf("  Name: %s\n", e.Name)
	}
	if e.FilePath != "" {
		fmt.Printf("  File: %s\n", e.FilePath)
	}
	if e.Detail != "" {
		fmt.Printf("  Detail: %s\n", e.Detail)
	}
	if len(e.Metadata) > 0 {
		parts := make([]string, 0, len(e.Metadata))
		for k, v := range e.Metadata {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("  Metadata: %s\n", strings.Join(parts, ", "))
	}
}

// cmdSurveyGraph builds and displays a call graph for a symbol.
func cmdSurveyGraph(dbPath string, args []string) error {
	symbol := parseFlag(args, "--symbol=")
	direction := parseFlag(args, "--direction=")
	jsonOutput := hasFlag(args, "--json")

	maxDepth := 2
	if v := parseFlag(args, "--max-depth="); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxDepth = n
		}
	}
	maxNodes := 50
	if v := parseFlag(args, "--max-nodes="); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxNodes = n
		}
	}

	// If --symbol= wasn't used, take the first positional argument.
	if symbol == "" {
		for _, arg := range args {
			if !strings.HasPrefix(arg, "--") {
				symbol = arg
				break
			}
		}
	}
	if symbol == "" {
		return fmt.Errorf("usage: aide survey graph <symbol> [--direction=<both|callers|callees>] [--max-depth=<n>] [--max-nodes=<n>] [--json]")
	}

	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	cg, cleanup, err := b.CodeGrapher()
	if err != nil {
		return fmt.Errorf("code index not available — run 'aide code index' first: %w", err)
	}
	defer cleanup()

	opts := survey.GraphOptions{
		MaxDepth:  maxDepth,
		MaxNodes:  maxNodes,
		Direction: direction,
	}

	graph, err := survey.BuildCallGraph(cg, symbol, opts)
	if err != nil {
		return fmt.Errorf("graph traversal failed: %w", err)
	}

	if jsonOutput {
		data, jsonErr := json.Marshal(graph)
		if jsonErr != nil {
			return fmt.Errorf("json encoding failed: %w", jsonErr)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output — match MCP handler format.
	dir := direction
	if dir == "" {
		dir = "both"
	}
	fmt.Printf("Call graph for %q (depth=%d, direction=%s)\n", graph.Root, graph.Depth, dir)
	fmt.Printf("Nodes: %d, Edges: %d\n\n", len(graph.Nodes), len(graph.Edges))

	if len(graph.Nodes) > 0 {
		fmt.Println("Nodes:")
		for _, n := range graph.Nodes {
			marker := " "
			if n.Name == graph.Root {
				marker = "*"
			}
			fmt.Printf("  %s %-30s %-10s %s:%d\n", marker, n.Name, n.Kind, n.FilePath, n.Line)
		}
		fmt.Println()
	}

	if len(graph.Edges) > 0 {
		fmt.Println("Edges:")
		for _, e := range graph.Edges {
			fmt.Printf("  %s -> %s  (%s at %s:%d)\n", e.From, e.To, e.Kind, e.FilePath, e.Line)
		}
	}

	if len(graph.Nodes) == 0 {
		fmt.Println("No call relationships found.")
	}

	return nil
}

// cmdSurveyRun runs survey analyzers from the CLI. Execution is delegated
// to the shared surveyrun package (or the daemon over gRPC) so every entry
// point produces identical results.
func cmdSurveyRun(dbPath string, args []string) error {
	analyzer := parseFlag(args, "--analyzer=")

	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()

	results, err := b.SurveyRun(analyzer)
	if err != nil {
		return fmt.Errorf("survey run failed: %w", err)
	}
	fmt.Print(surveyrun.FormatResults(results))
	return nil
}

// printSurveyUsage prints help text for the survey command.
func printSurveyUsage() {
	fmt.Print(`aide survey - Query and manage codebase survey data

Usage:
  aide survey <command> [arguments]

Commands:
  run             Run survey analyzers to populate data
  search <query>  Search survey entries by text
  list            List survey entries with optional filters
  stats           Show survey statistics
  graph <symbol>  Build a call graph for a symbol
  clear           Clear survey entries

Flags (run):
  --analyzer=<name>  Run only a specific analyzer: topology, entrypoints, churn, modules

Flags (search, list):
  --analyzer=<name>  Filter by analyzer: topology, entrypoints, churn
  --kind=<kind>      Filter by kind: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern
  --file=<path>      Filter by file path pattern
  --limit=<n>        Maximum results
  --json             Output as JSON

Flags (graph):
  --symbol=<name>        Symbol name (or pass as first positional argument)
  --direction=<dir>      Traversal direction: both (default), callers, callees
  --max-depth=<n>        Max BFS hops (default 2)
  --max-nodes=<n>        Max graph nodes (default 50)
  --json                 Output as JSON

Flags (clear):
  --analyzer=<name>  Clear only entries from a specific analyzer

Note: --analyser= is accepted as an alias for --analyzer= in all commands.

Examples:
  aide survey run
  aide survey run --analyzer=topology
  aide survey search "auth"
  aide survey search "auth" --json
  aide survey list --analyzer=topology
  aide survey list --kind=entrypoint --json
  aide survey stats
  aide survey graph BuildCallGraph
  aide survey graph --symbol=main --direction=callees --json
  aide survey clear --analyzer=churn
`)
}
