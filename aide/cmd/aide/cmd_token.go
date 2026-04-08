package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdTokenDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printTokenUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "record":
		return cmdTokenRecord(dbPath, subargs)
	case "summary":
		return cmdTokenSummary(dbPath, subargs)
	case "stats":
		return cmdTokenStats(dbPath, subargs)
	case "cleanup":
		return cmdTokenCleanup(dbPath, subargs)
	case "help", "-h", "--help":
		printTokenUsage()
		return nil
	default:
		return fmt.Errorf("unknown token subcommand: %s", subcmd)
	}
}

func printTokenUsage() {
	fmt.Println(`aide token - Estimated token intelligence and tracking

Usage:
  aide token <subcommand> [arguments]

Subcommands:
  record     Record a token event (used by hooks)
  summary    Show estimated token summary for a session
  stats      Show estimated all-time token statistics
  cleanup    Remove old token events

Options:
  record <event_type> <tool> <file> <tokens> [saved]:
    Event types: read, outline_used, read_avoided, write, edit

  summary:
    --session=ID     Specific session (default: all)
    --last=N         Last N sessions
    --json           Output as JSON

  stats:
    --session=ID     Filter by session
    --json           Output as JSON

  cleanup:
    --max-age=DURATION  Max age (default: 720h = 30 days)

Note: All token counts are estimates based on calibrated per-language ratios.

Examples:
  aide token record read Read src/auth.ts 1200
  aide token record outline_used code_outline src/auth.ts 150 1050
  aide token stats
  aide token summary --last=5
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

// cmdTokenRecord records a single token event. Called by TS hooks.
func cmdTokenRecord(dbPath string, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: aide token record <event_type> <tool> <file> <tokens> [saved]")
	}

	eventType := args[0]
	tool := args[1]
	filePath := args[2]

	var tokens, saved int
	fmt.Sscanf(args[3], "%d", &tokens)
	if len(args) > 4 {
		fmt.Sscanf(args[4], "%d", &saved)
	}

	sessionID := parseFlag(args, "--session=")

	backend, st, err := getStoreOrFail(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	return st.AddTokenEvent(&memory.TokenEvent{
		SessionID:   sessionID,
		EventType:   eventType,
		Tool:        tool,
		FilePath:    filePath,
		Tokens:      tokens,
		TokensSaved: saved,
	})
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
	if l := parseFlag(args, "--last="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	events, err := st.ListTokenEvents(sessionID, limit, time.Time{}, time.Time{})
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
		"Time", "Tool", "Type", "~Tokens", "~Saved", "File")
	fmt.Println(strings.Repeat("-", 80))

	for _, e := range events {
		ts := e.Timestamp.Format("2006-01-02 15:04:05")
		savedStr := ""
		if e.TokensSaved > 0 {
			savedStr = fmt.Sprintf("%d", e.TokensSaved)
		}
		fmt.Printf("%-20s %-16s %-8s %8d %8s  %s\n",
			ts, e.Tool, e.EventType, e.Tokens, savedStr, e.FilePath)
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

	stats, err := st.TokenStats(sessionID, time.Time{}, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}

	fmt.Println("Estimated Token Statistics")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("  Events:              %d\n", stats.EventCount)
	fmt.Printf("  Sessions:            %d\n", stats.Sessions)
	fmt.Printf("  Est. tokens read:    %d\n", stats.TotalRead)
	fmt.Printf("  Est. tokens written: %d\n", stats.TotalWritten)
	fmt.Printf("  Est. tokens saved:   %d\n", stats.TotalSaved)

	if stats.TotalRead+stats.TotalSaved > 0 {
		pct := float64(stats.TotalSaved) / float64(stats.TotalRead+stats.TotalSaved) * 100
		fmt.Printf("  Est. savings:        ~%.1f%%\n", pct)
	}

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
		fmt.Println("  By Saving Type (est. tokens saved):")
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
	fmt.Println("Note: All token counts are estimates based on calibrated per-language ratios.")

	return nil
}

// cmdTokenCleanup removes old token events.
func cmdTokenCleanup(dbPath string, args []string) error {
	maxAge := 30 * 24 * time.Hour // 30 days default

	if d := parseFlag(args, "--max-age="); d != "" {
		parsed, err := time.ParseDuration(d)
		if err != nil {
			return fmt.Errorf("invalid --max-age duration: %w", err)
		}
		maxAge = parsed
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
