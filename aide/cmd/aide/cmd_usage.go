// Package main provides the usage command for aide.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StatsCache represents Claude Code's stats-cache.json structure.
type StatsCache struct {
	Version          int                       `json:"version"`
	LastComputedDate string                    `json:"lastComputedDate"`
	TotalMessages    int                       `json:"totalMessages"`
	TotalSessions    int                       `json:"totalSessions"`
	FirstSessionDate string                    `json:"firstSessionDate"`
	DailyActivity    []DailyActivity           `json:"dailyActivity"`
	DailyModelTokens []DailyModelTokens        `json:"dailyModelTokens"`
	ModelUsage       map[string]ModelUsageInfo `json:"modelUsage"`
	LongestSession   *SessionInfo              `json:"longestSession"`
}

type ModelUsageInfo struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
}

type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

type DailyModelTokens struct {
	Date          string         `json:"date"`
	TokensByModel map[string]int `json:"tokensByModel"`
}

type SessionInfo struct {
	SessionID    string `json:"sessionId"`
	MessageCount int    `json:"messageCount"`
	Date         string `json:"date"`
}

// jsonlMessage represents a single line in a Claude Code JSONL session file.
type jsonlMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// UsageSummary is the structured JSON output for HUD and programmatic use.
type UsageSummary struct {
	Realtime   RealtimeUsage `json:"realtime"`
	Historical HistUsage     `json:"historical"`
	Timestamp  string        `json:"timestamp"`
}

type RealtimeUsage struct {
	Window5h      TokenBucket `json:"window_5h"`
	Today         TokenBucket `json:"today"`
	Messages5h    int         `json:"messages_5h"`
	MessagesToday int         `json:"messages_today"`
	WindowStart   string      `json:"window_start,omitempty"`  // ISO8601 timestamp of first msg in 5h window
	WindowResets  string      `json:"window_resets,omitempty"` // When the 5h window expires
	WindowRemain  string      `json:"window_remain,omitempty"` // Human-readable time remaining
}

type TokenBucket struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cache_read"`
	CacheCreate int64 `json:"cache_create"`
	Total       int64 `json:"total"`
}

type HistUsage struct {
	Week     int64  `json:"week_tokens"`
	AllTime  int64  `json:"all_time_tokens"`
	Messages int    `json:"all_time_messages"`
	Sessions int    `json:"all_time_sessions"`
	Since    string `json:"since,omitempty"`
}

// scanJSONLFiles reads all JSONL session files and sums token usage.
func scanJSONLFiles(home string) (RealtimeUsage, error) {
	var usage RealtimeUsage

	now := time.Now()
	todayStr := now.Format("2006-01-02")
	fiveHoursAgo := now.Add(-5 * time.Hour)
	var earliestIn5h time.Time

	projectsDir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return usage, nil // No projects yet
	}

	// Walk all project directories for JSONL files
	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Skip files not modified today (optimization)
		if info.ModTime().Format("2006-01-02") < todayStr {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		// Increase buffer for large lines (some messages are huge)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 2*1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()

			// Fast pre-filter: skip lines without "usage"
			if !strings.Contains(string(line), `"usage"`) {
				continue
			}

			var msg jsonlMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}

			// Only process assistant messages with usage data
			if msg.Message == nil || msg.Message.Role != "assistant" || msg.Message.Usage == nil {
				continue
			}

			// Parse timestamp
			ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
			if err != nil {
				ts, err = time.Parse(time.RFC3339, msg.Timestamp)
				if err != nil {
					continue
				}
			}

			// Check if this is from today
			if ts.Format("2006-01-02") != todayStr {
				continue
			}

			u := msg.Message.Usage
			usage.Today.Input += int64(u.InputTokens)
			usage.Today.Output += int64(u.OutputTokens)
			usage.Today.CacheRead += int64(u.CacheReadInputTokens)
			usage.Today.CacheCreate += int64(u.CacheCreationInputTokens)
			usage.MessagesToday++

			// Check if within 5h window
			if ts.After(fiveHoursAgo) {
				usage.Window5h.Input += int64(u.InputTokens)
				usage.Window5h.Output += int64(u.OutputTokens)
				usage.Window5h.CacheRead += int64(u.CacheReadInputTokens)
				usage.Window5h.CacheCreate += int64(u.CacheCreationInputTokens)
				usage.Messages5h++
				if earliestIn5h.IsZero() || ts.Before(earliestIn5h) {
					earliestIn5h = ts
				}
			}
		}

		return nil
	})

	// Calculate totals
	usage.Today.Total = usage.Today.Input + usage.Today.Output
	usage.Window5h.Total = usage.Window5h.Input + usage.Window5h.Output

	// Calculate window timing
	// Claude uses a fixed window that resets to zero (not a rolling window).
	// The window starts at the earliest message, resets at earliest + 5h.
	if !earliestIn5h.IsZero() {
		windowResets := earliestIn5h.Add(5 * time.Hour)
		usage.WindowStart = earliestIn5h.Format(time.RFC3339)
		usage.WindowResets = windowResets.Format(time.RFC3339)

		remaining := windowResets.Sub(now)
		if remaining > 0 {
			hours := int(remaining.Hours())
			mins := int(remaining.Minutes()) % 60
			switch {
			case hours > 0:
				usage.WindowRemain = fmt.Sprintf("%dh%dm", hours, mins)
			case mins > 0:
				usage.WindowRemain = fmt.Sprintf("%dm", mins)
			default:
				usage.WindowRemain = "<1m"
			}
		} else {
			usage.WindowRemain = "expired"
		}
	}

	return usage, err
}

// cmdUsage shows Claude Code usage statistics.
func cmdUsage(args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printUsageCommandUsage()
		return nil
	}

	// Parse flags
	showDaily := hasFlag(args, "--daily") || hasFlag(args, "-d")
	showWeek := hasFlag(args, "--week") || hasFlag(args, "-w")
	showAll := hasFlag(args, "--all") || hasFlag(args, "-a")
	jsonOutput := hasFlag(args, "--json")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Read stats cache for historical data
	var stats StatsCache
	cachePath := filepath.Join(home, ".claude", "stats-cache.json")
	if data, err := os.ReadFile(cachePath); err == nil {
		json.Unmarshal(data, &stats) // Best effort
	}

	// Scan JSONL files for real-time today + 5h data
	realtime, _ := scanJSONLFiles(home)

	// Calculate historical totals from stats cache
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var weekTokens, totalTokens int64

	for _, day := range stats.DailyModelTokens {
		for _, tokens := range day.TokensByModel {
			totalTokens += int64(tokens)
			if day.Date >= weekAgo {
				weekTokens += int64(tokens)
			}
		}
	}

	// Add today's real-time tokens to weekly total (stats cache may be stale)
	weekTokens += realtime.Today.Total

	var weekMessages int
	for _, day := range stats.DailyActivity {
		if day.Date >= weekAgo {
			weekMessages += day.MessageCount
		}
	}
	weekMessages += realtime.MessagesToday

	// JSON output
	if jsonOutput {
		summary := UsageSummary{
			Realtime: realtime,
			Historical: HistUsage{
				Week:     weekTokens,
				AllTime:  totalTokens + realtime.Today.Total,
				Messages: stats.TotalMessages + realtime.MessagesToday,
				Sessions: stats.TotalSessions,
				Since:    stats.FirstSessionDate,
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		output, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	// Print summary
	today := time.Now().Format("2006-01-02")
	fmt.Println("Claude Code Usage Statistics")
	fmt.Println("============================")
	fmt.Println()

	fmt.Printf("Last 5 Hours (rolling window):\n")
	fmt.Printf("  Input:    %s tokens\n", formatLargeNumber(realtime.Window5h.Input))
	fmt.Printf("  Output:   %s tokens\n", formatLargeNumber(realtime.Window5h.Output))
	fmt.Printf("  Total:    %s tokens\n", formatLargeNumber(realtime.Window5h.Total))
	fmt.Printf("  Messages: %d\n", realtime.Messages5h)
	if realtime.Window5h.CacheRead > 0 {
		fmt.Printf("  Cache:    %s read, %s created\n",
			formatLargeNumber(realtime.Window5h.CacheRead),
			formatLargeNumber(realtime.Window5h.CacheCreate))
	}
	if realtime.WindowRemain != "" && realtime.WindowRemain != "expired" {
		fmt.Printf("  Resets:   in %s\n", realtime.WindowRemain)
	} else if realtime.WindowRemain == "expired" {
		fmt.Printf("  Window:   expired (next message starts new window)\n")
	}
	fmt.Println()

	fmt.Printf("Today (%s):\n", today)
	fmt.Printf("  Input:    %s tokens\n", formatLargeNumber(realtime.Today.Input))
	fmt.Printf("  Output:   %s tokens\n", formatLargeNumber(realtime.Today.Output))
	fmt.Printf("  Total:    %s tokens\n", formatLargeNumber(realtime.Today.Total))
	fmt.Printf("  Messages: %d\n", realtime.MessagesToday)
	if realtime.Today.CacheRead > 0 {
		fmt.Printf("  Cache:    %s read, %s created\n",
			formatLargeNumber(realtime.Today.CacheRead),
			formatLargeNumber(realtime.Today.CacheCreate))
	}
	fmt.Println()

	fmt.Println("Last 7 Days:")
	fmt.Printf("  Tokens:   %s\n", formatLargeNumber(weekTokens))
	fmt.Printf("  Messages: %d\n", weekMessages)
	fmt.Println()

	fmt.Println("All Time:")
	fmt.Printf("  Tokens:   %s\n", formatLargeNumber(totalTokens+realtime.Today.Total))
	fmt.Printf("  Messages: %d\n", stats.TotalMessages+realtime.MessagesToday)
	fmt.Printf("  Sessions: %d\n", stats.TotalSessions)
	if stats.FirstSessionDate != "" {
		fmt.Printf("  Since:    %s\n", stats.FirstSessionDate)
	}
	fmt.Println()

	// Model breakdown from stats cache
	if len(stats.ModelUsage) > 0 {
		fmt.Println("By Model (historical):")
		for model, usage := range stats.ModelUsage {
			shortModel := shortenModelName(model)
			modelTotal := usage.InputTokens + usage.OutputTokens
			fmt.Printf("  %s:\n", shortModel)
			fmt.Printf("    Input:  %s tokens\n", formatNumber(usage.InputTokens))
			fmt.Printf("    Output: %s tokens\n", formatNumber(usage.OutputTokens))
			fmt.Printf("    Total:  %s tokens\n", formatNumber(modelTotal))
			if usage.CacheReadInputTokens > 0 {
				fmt.Printf("    Cache:  %s read\n", formatLargeNumber(usage.CacheReadInputTokens))
			}
		}
		fmt.Println()
	}

	// Show daily breakdown if requested
	if showDaily || showWeek || showAll {
		days := stats.DailyModelTokens
		if !showAll {
			var filtered []DailyModelTokens
			for _, day := range days {
				if day.Date >= weekAgo {
					filtered = append(filtered, day)
				}
			}
			days = filtered
		}

		sort.Slice(days, func(i, j int) bool {
			return days[i].Date > days[j].Date
		})

		fmt.Println("Daily Token Usage:")
		for _, day := range days {
			var total int
			for _, tokens := range day.TokensByModel {
				total += tokens
			}
			fmt.Printf("  %s: %s tokens\n", day.Date, formatNumber(total))
		}
	}

	return nil
}

// formatNumber formats a number with comma separators.
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d", n/1000000, (n/1000)%1000, n%1000)
}

// formatLargeNumber formats a large int64 with K/M/B suffixes.
func formatLargeNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}

func printUsageCommandUsage() {
	fmt.Println(`aide usage - Show Claude Code usage statistics

Usage:
  aide usage [options]

Options:
  --daily, -d        Show daily token breakdown
  --week, -w         Show last 7 days breakdown
  --all, -a          Show all-time daily breakdown
  --json             Output as JSON (for programmatic use)

Displays token usage across multiple time windows:
  - Last 5 hours (rolling rate-limit window)
  - Today
  - Last 7 days
  - All time

Examples:
  aide usage
  aide usage --json
  aide usage --daily
  aide usage --all`)
}

// shortenModelName shortens the model name for display.
func shortenModelName(model string) string {
	switch model {
	case "claude-opus-4-5-20251101":
		return "Opus 4.5"
	case "claude-opus-4-6-20260125":
		return "Opus 4.6"
	case "claude-sonnet-4-5-20250929":
		return "Sonnet 4.5"
	case "claude-sonnet-4-20250514":
		return "Sonnet 4"
	case "claude-3-5-sonnet-20241022":
		return "Sonnet 3.5"
	case "claude-3-opus-20240229":
		return "Opus 3"
	default:
		if len(model) > 25 {
			return model[:22] + "..."
		}
		return model
	}
}
