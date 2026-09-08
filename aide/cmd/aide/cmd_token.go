package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdTokenDispatcher(dbPath string, args []string) error {
	return dispatchSubcmd("token", args, printTokenUsage, []subcmd{
		{name: "summary", handler: func(a []string) error { return cmdTokenSummary(dbPath, a) }},
		{name: "stats", handler: func(a []string) error { return cmdTokenStats(dbPath, a) }},
		{name: "cleanup", handler: func(a []string) error { return cmdTokenCleanup(dbPath, a) }},
	})
}

func printTokenUsage() {
	fmt.Println(`aide token - Estimated token intelligence and tracking

Usage:
  aide token <subcommand> [arguments]

Subcommands:
  summary    Show estimated token summary for a session
  stats      Show estimated all-time token statistics
  cleanup    Remove old token events

Options:
  summary:
    --details        Include retrieval status and evidence limits
    --session=ID     Specific session (default: all)
    --limit=N        Most recent N events (default: 100)
    --last=N         Deprecated alias for --limit (historically limits events)
    --since=TIME     RFC3339 timestamp or duration ago (e.g. 24h)
    --until=TIME     RFC3339 upper bound (inclusive)
    --json           Output as JSON

  stats:
    --details        Include aide work, windows, coverage and historical breakdowns
    --session=ID     Filter by session
    --since=TIME     RFC3339 timestamp or duration ago
    --until=TIME     RFC3339 upper bound (inclusive)
    --json           Output as JSON

  cleanup:
    --max-age=DURATION  Max age (default: cleanup.token_max_age, 90d; 0 = keep all)

Note: Observed UTF-8 text uses a versioned byte/token estimate.
Legacy saved fields are comparison estimates, not verified or provider savings.
Token events are recorded automatically by hooks via the observe stream
(kind=injection / kind=tool_call); there is no manual record subcommand.

Examples:
  aide token stats
  aide token summary --limit=5
  aide token cleanup --max-age=168h`)
}

func getStoreOrFail(dbPath string) (*Backend, store.Store, error) {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create backend: %w", err)
	}
	st := backend.Store()
	if st == nil {
		backend.Close()
		return nil, nil, fmt.Errorf("store not available (MCP server may need restart)")
	}
	return backend, st, nil
}

// cmdTokenSummary shows token event summary.
func cmdTokenSummary(dbPath string, args []string) error {
	jsonOutput := hasFlag(args, "--json")
	sessionID := parseFlag(args, "--session=")

	backend, st, err := getStoreOrFail(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	limit := 100
	value := parseFlag(args, "--limit=")
	if value == "" {
		value = parseFlag(args, "--last=")
	}
	if value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 {
			return fmt.Errorf("event limit must be a positive integer")
		}
	}
	since, until, err := tokenTimeRange(args, time.Now())
	if err != nil {
		return err
	}
	events, err := st.ListTokenEvents(sessionID, limit, since, until)
	if err != nil {
		return fmt.Errorf("failed to list events: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}

	if len(events) == 0 {
		fmt.Println("No token events recorded.")
		return nil
	}

	fmt.Println("Estimated Token Events (most recent first)")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-20s %-16s %-8s %8s %8s  %s\n",
		"Time", "Tool", "Type", "~Tokens", "~Legacy", "File")
	fmt.Println(strings.Repeat("-", 80))

	for _, e := range events {
		ts := e.Timestamp.Format("2006-01-02 15:04:05")
		savedStr := ""
		if e.TokensSaved > 0 {
			savedStr = fmt.Sprintf("%d", e.TokensSaved)
		}
		tokenText := "unknown"
		if _, ok := memory.MeasuredBytes(e.Attrs, "payload_bytes"); ok || e.Tokens > 0 {
			tokenText = fmt.Sprintf("~%d", e.Tokens)
		}
		fmt.Printf("%-20s %-16s %-8s %8s %8s  %s\n", ts, e.Tool, e.EventType, tokenText, savedStr, e.FilePath)
		if e.Attrs["accounting_version"] == "1" {
			bytes := e.Attrs["payload_bytes"]
			if bytes == "" {
				bytes = "unknown"
			}
			invocation := e.Attrs["invocation_id"]
			if invocation == "" {
				invocation = "unknown"
			}
			fmt.Printf("  %s: %s UTF-8 text bytes; estimator=%s; invocation=%s\n", e.Attrs["observation_stage"], bytes, memory.TextEstimator, invocation)
			if hasFlag(args, "--details") {
				fmt.Print(formatRetrievalEvidence(e.Attrs))
			}
		} else {
			fmt.Println("  Legacy estimate; measurement and delivery coverage unknown")
		}
	}

	return nil
}

// cmdTokenStats shows aggregate token statistics.
func cmdTokenStats(dbPath string, args []string) error {
	jsonOutput := hasFlag(args, "--json")
	sessionID := parseFlag(args, "--session=")

	backend, st, err := getStoreOrFail(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	since, until, err := tokenTimeRange(args, time.Now())
	if err != nil {
		return err
	}
	stats, err := st.TokenStats(sessionID, since, until)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}

	fmt.Println("Token Accounting")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("  Recorded observations: %d; known sessions: %d\n", stats.EventCount, stats.Sessions)
	if a := stats.Accounting; a != nil && a.Version == 1 {
		fmt.Printf("  Estimator: %s (UTF-8 text only)\n", a.Estimator)
		for _, stage := range []string{"host_result", "server_result"} {
			printTokenQuantity(stage, a.ByStage[stage])
		}
		printTokenQuantity("generated_arguments", &a.Arguments)
		fmt.Printf("  Coverage: %d legacy; %d missing text; %d missing identity\n", a.LegacyEvents, a.MissingPayload, a.MissingIdentity)
		fmt.Println("  Stages can overlap; do not sum. Unseen calls and final delivery are unknown.")
		fmt.Print(formatTransformationSummary(a.Transformations, hasFlag(args, "--details")))
		fmt.Print(formatRetrievalWindows(a.Retrievals, hasFlag(args, "--details")))
		if hasFlag(args, "--details") {
			printTokenQuantity("prepared_aide_context", a.ByStage["aide_context"])
			fmt.Println("  Prepared source or appended text; not full prompt usage or confirmed delivery. Source excerpts can omit formatting; repeated preparations can overlap.")
			fmt.Print(formatTokenWork(a.Work))
		}
	} else {
		fmt.Println("  Accounting unavailable from this server.")
	}
	fmt.Println("  Provider savings / inferred avoided calls: unavailable")
	if !hasFlag(args, "--details") {
		fmt.Println("  Use --details for window and historical breakdowns, or --json for all evidence.")
		return nil
	}
	fmt.Println("\nHistorical and compatibility estimates (mixed methods)")
	fmt.Printf("  Result tokens: ~%d; generated tokens: ~%d; guidance: ~%d\n", stats.TotalRead, stats.TotalWritten, stats.TotalDelivered)
	fmt.Printf("  Legacy comparison estimate: ~%d (not verified savings; may overlap)\n", stats.TotalSaved)

	if len(stats.ByTool) > 0 {
		fmt.Println()
		fmt.Println("  By Tool (est. tokens):")
		tools := make([]string, 0, len(stats.ByTool))
		for k := range stats.ByTool {
			tools = append(tools, k)
		}
		sort.Strings(tools)
		for _, t := range tools {
			fmt.Printf("    %-20s %d\n", t, stats.ByTool[t])
		}
	}

	if len(stats.BySavingType) > 0 {
		fmt.Println()
		fmt.Println("  Legacy comparison estimates:")
		types := make([]string, 0, len(stats.BySavingType))
		for k := range stats.BySavingType {
			types = append(types, k)
		}
		sort.Strings(types)
		for _, t := range types {
			fmt.Printf("    %-20s %d\n", t, stats.BySavingType[t])
		}
	}

	fmt.Println()
	fmt.Println("Note: Observed UTF-8 text uses a versioned byte/token estimate. Legacy saved fields are comparison estimates, not verified or provider savings.")

	return nil
}

// printTokenQuantity distinguishes absent measurements from known zero.
func printTokenQuantity(label string, q *memory.TokenQuantity) {
	if q == nil || q.Events == 0 {
		fmt.Printf("  %s: unknown (no measured text)\n", label)
		return
	}
	fmt.Printf("  %s: %d bytes; ~%d tokens; %d observations\n", label, q.Bytes, q.EstimatedTokens, q.Events)
}

func tokenTimeRange(args []string, now time.Time) (since, until time.Time, err error) {
	if value := parseFlag(args, "--since="); value != "" {
		since, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			var duration time.Duration
			duration, err = parseDurationDays(value)
			if err != nil || duration < 0 {
				return since, until, fmt.Errorf("invalid --since: %s", value)
			}
			since = now.Add(-duration)
		}
	}
	if value := parseFlag(args, "--until="); value != "" {
		until, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return since, until, fmt.Errorf("invalid --until: %w", err)
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return since, until, fmt.Errorf("since must not be after until")
	}
	return since, until, nil
}

// cmdTokenCleanup removes old token events.
func cmdTokenCleanup(dbPath string, args []string) error {
	maxAge := config.Get().Cleanup.TokenMaxAgeDuration()

	if d := parseFlag(args, "--max-age="); d != "" {
		parsed, err := time.ParseDuration(d)
		if err != nil {
			return fmt.Errorf("invalid --max-age duration: %w", err)
		}
		maxAge = parsed
	}

	if maxAge <= 0 {
		fmt.Println("Token retention is disabled (max-age 0): no token events pruned.")
		return nil
	}

	backend, st, err := getStoreOrFail(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	count, err := st.CleanupTokenEvents(maxAge)
	if err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}

	if count > 0 {
		fmt.Printf("Cleaned up %d token events older than %s\n", count, maxAge)
	} else {
		fmt.Println("No stale token events to clean up")
	}

	return nil
}
