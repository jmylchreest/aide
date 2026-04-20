package main

import (
	"encoding/json"
	"fmt"
	"os"
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
  list       List recent events (newest first)
  summary    Aggregate counts by kind / category
  cleanup    Remove events older than --age (default 30d)

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
		dur, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --since (duration): %w", err)
		}
		filter.Since = time.Now().Add(-dur)
	}

	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	events, err := st.ListObserveEvents(filter)
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
		dur, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		filter.Since = time.Now().Add(-dur)
	}

	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	events, err := st.ListObserveEvents(filter)
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

func cmdObserveCleanup(dbPath string, args []string) error {
	age := 30 * 24 * time.Hour
	if v := parseFlag(args, "--age="); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --age: %w", err)
		}
		age = dur
	}
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.CleanupObserveEvents(age)
	if err != nil {
		return err
	}
	fmt.Printf("Deleted %d events older than %v.\n", n, age)
	return nil
}
