package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdObserveDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printObserveUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return cmdObserveList(dbPath, args[1:])
	case "summary":
		return cmdObserveSummary(dbPath, args[1:])
	case "efficiency":
		return cmdObserveEfficiency(dbPath, args[1:])
	case "cleanup":
		return cmdObserveCleanup(dbPath, args[1:])
	case "--help", "-h":
		printObserveUsage()
		return nil
	default:
		return fmt.Errorf("unknown observe subcommand: %s", args[0])
	}
}

func printObserveUsage() {
	fmt.Println(`aide observe - Query the unified observability event store

Usage:
  aide observe <subcommand> [options]

Subcommands:
  list        List recent events (newest first)
  summary     Aggregate counts by kind / category
  efficiency  Token efficiency: counterfactual vs actual reads
  cleanup     Remove events older than --age (default 30d)

Options for list / summary:
  --kind=K         Filter by kind: tool_call | span | hook | injection | session
  --name=N         Filter by event name (e.g. "code_outline", "AnalyzeDeadCode")
  --category=C     Filter by category (consume / navigate / search / modify / execute / network / coordinate / inject / analyzer / indexer)
  --session=ID     Filter by session ID
  --since=DUR      Only events newer than DUR (e.g. 1h, 24h)
  --limit=N        Max results (default 50; 0 = no limit)
  --json           Machine-readable output

cleanup options:
  --age=DUR        Delete events older than DUR (default 720h / 30d)`)
}

func cmdObserveList(dbPath string, args []string) error {
	jsonOut := hasFlag(args, "--json")
	filter := store.ObserveFilter{
		Kind:      observe.Kind(parseFlag(args, "--kind=")),
		Name:      parseFlag(args, "--name="),
		Category:  parseFlag(args, "--category="),
		SessionID: parseFlag(args, "--session="),
		Limit:     50,
	}
	if v := parseFlag(args, "--limit="); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid --limit: %w", err)
		}
		filter.Limit = n
	}
	if v := parseFlag(args, "--since="); v != "" {
		dur, err := parseDurationDays(v)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		filter.Since = time.Now().Add(-dur)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	events, err := backend.Store().ListObserveEvents(filter)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}

	if len(events) == 0 {
		fmt.Println("No events.")
		return nil
	}
	for _, e := range events {
		extras := ""
		if e.FilePath != "" {
			extras += " " + e.FilePath
		}
		if e.Tokens > 0 || e.TokensSaved > 0 {
			extras += fmt.Sprintf(" tokens=%d saved=%d", e.Tokens, e.TokensSaved)
		}
		fmt.Printf("%s %-9s %-25s %-12s %4dms%s\n",
			e.Timestamp.Format("15:04:05"),
			string(e.Kind),
			e.Name,
			e.Category,
			e.DurationMs,
			extras,
		)
	}
	return nil
}

func cmdObserveSummary(dbPath string, args []string) error {
	filter := store.ObserveFilter{Limit: 0}
	if v := parseFlag(args, "--since="); v != "" {
		dur, err := parseDurationDays(v)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		filter.Since = time.Now().Add(-dur)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	events, err := backend.Store().ListObserveEvents(filter)
	if err != nil {
		return err
	}

	byKind := map[string]*bucket{}
	byCategory := map[string]*bucket{}
	byName := map[string]*bucket{}

	addTo := func(m map[string]*bucket, key string, e *observe.Event) {
		if key == "" {
			return
		}
		b, ok := m[key]
		if !ok {
			b = &bucket{}
			m[key] = b
		}
		b.count++
		b.totalMs += e.DurationMs
		b.tokens += e.Tokens
		b.saved += e.TokensSaved
	}
	for _, e := range events {
		addTo(byKind, string(e.Kind), e)
		addTo(byCategory, e.Category, e)
		addTo(byName, e.Name, e)
	}

	if hasFlag(args, "--json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"total":       len(events),
			"by_kind":     byKind,
			"by_category": byCategory,
			"by_name":     byName,
		})
	}

	fmt.Printf("Total events: %d\n\n", len(events))
	printBuckets("By kind", byKind)
	printBuckets("By category", byCategory)
	printBuckets("By name", byName)
	return nil
}

func printBuckets(title string, m map[string]*bucket) {
	if len(m) == 0 {
		return
	}
	fmt.Println(title + ":")
	for k, b := range m {
		extra := ""
		if b.tokens > 0 || b.saved > 0 {
			extra = fmt.Sprintf(" tokens=%d saved=%d", b.tokens, b.saved)
		}
		fmt.Printf("  %-25s %5d  %6dms%s\n", k, b.count, b.totalMs, extra)
	}
	fmt.Println()
}

type bucket struct {
	count   int
	totalMs int64
	tokens  int
	saved   int
}

// cmdObserveEfficiency prints a token-efficiency summary: for every consume
// event (code_outline, code_read_symbol, raw reads) it compares what a raw
// file Read would have cost (Tokens + TokensSaved) against what was actually
// consumed (Tokens). Token estimates come from calibrated per-language ratios
// in pkg/code/tokens.go — measured against the Anthropic count_tokens API so
// the "would have been X" number is grounded rather than a guess.
func cmdObserveEfficiency(dbPath string, args []string) error {
	jsonOut := hasFlag(args, "--json")
	filter := store.ObserveFilter{Kind: observe.KindToolCall, Category: "consume", Limit: 0}
	if v := parseFlag(args, "--since="); v != "" {
		dur, err := parseDurationDays(v)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		filter.Since = time.Now().Add(-dur)
	}
	if v := parseFlag(args, "--session="); v != "" {
		filter.SessionID = v
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	events, err := backend.Store().ListObserveEvents(filter)
	if err != nil {
		return err
	}

	type toolStats struct {
		Calls          int `json:"calls"`
		Counterfactual int `json:"counterfactual_tokens"`
		Actual         int `json:"actual_tokens"`
		Saved          int `json:"saved_tokens"`
	}
	byTool := map[string]*toolStats{}
	var total toolStats

	for _, e := range events {
		b, ok := byTool[e.Name]
		if !ok {
			b = &toolStats{}
			byTool[e.Name] = b
		}
		b.Calls++
		b.Actual += e.Tokens
		b.Saved += e.TokensSaved
		b.Counterfactual += e.Tokens + e.TokensSaved
		total.Calls++
		total.Actual += e.Tokens
		total.Saved += e.TokensSaved
		total.Counterfactual += e.Tokens + e.TokensSaved
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"since":   filter.Since,
			"total":   total,
			"by_tool": byTool,
		})
	}

	if total.Calls == 0 {
		fmt.Println("No consume events recorded yet.")
		fmt.Println("Run some MCP code tools (code_outline, code_read_symbol) or file Reads to populate.")
		return nil
	}

	fmt.Println("Token efficiency")
	fmt.Println("================")
	fmt.Println()
	fmt.Println("Estimates use calibrated per-language char/token ratios")
	fmt.Println("(see pkg/code/tokens.go — measured against Anthropic count_tokens).")
	fmt.Println()
	fmt.Printf("Would have read:  %s tokens  (if every call had been a raw Read)\n", fmtInt(total.Counterfactual))
	fmt.Printf("Actually read:    %s tokens\n", fmtInt(total.Actual))
	fmt.Printf("Saved:            %s tokens", fmtInt(total.Saved))
	if total.Counterfactual > 0 {
		ratio := float64(total.Saved) / float64(total.Counterfactual) * 100
		fmt.Printf("  (%.1f%% efficiency)", ratio)
	}
	fmt.Println()
	fmt.Println()

	// Per-tool table
	names := make([]string, 0, len(byTool))
	for n := range byTool {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return byTool[names[i]].Saved > byTool[names[j]].Saved })

	fmt.Printf("%-20s %6s  %14s  %14s  %14s  %6s\n", "tool", "calls", "would have", "actual", "saved", "eff")
	for _, n := range names {
		b := byTool[n]
		eff := ""
		if b.Counterfactual > 0 {
			eff = fmt.Sprintf("%.1f%%", float64(b.Saved)/float64(b.Counterfactual)*100)
		}
		fmt.Printf("%-20s %6d  %14s  %14s  %14s  %6s\n",
			n, b.Calls, fmtInt(b.Counterfactual), fmtInt(b.Actual), fmtInt(b.Saved), eff)
	}
	return nil
}

// parseDurationDays extends time.ParseDuration to accept "d" (days) since
// time.ParseDuration tops out at hours ("h").
func parseDurationDays(s string) (time.Duration, error) {
	if len(s) > 1 && s[len(s)-1] == 'd' {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// fmtInt adds thousand separators to an int for readability.
func fmtInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for i, c := range s {
		if i != 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

func cmdObserveCleanup(dbPath string, args []string) error {
	age := 30 * 24 * time.Hour
	if v := parseFlag(args, "--age="); v != "" {
		dur, err := parseDurationDays(v)
		if err != nil {
			return fmt.Errorf("invalid --age: %w", err)
		}
		age = dur
	}
	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()
	n, err := backend.Store().CleanupObserveEvents(age)
	if err != nil {
		return err
	}
	fmt.Printf("Deleted %d events older than %v.\n", n, age)
	return nil
}
