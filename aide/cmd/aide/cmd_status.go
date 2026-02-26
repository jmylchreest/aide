package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
)

type StatusOutput struct {
	Version   string            `json:"version"`
	Mode      string            `json:"mode,omitempty"`
	Project   string            `json:"project"`
	Timestamp time.Time         `json:"timestamp"`
	Watcher   *WatcherStatus    `json:"watcher,omitempty"`
	Code      *CodeStatus       `json:"codeIndexer,omitempty"`
	Findings  *FindingsStatus   `json:"findings,omitempty"`
	Stores    StoreStatus       `json:"stores"`
	Env       map[string]string `json:"environment,omitempty"`
}

type WatcherStatus struct {
	Enabled      bool     `json:"enabled"`
	Paths        []string `json:"paths,omitempty"`
	DirsWatched  int      `json:"dirsWatched"`
	Debounce     string   `json:"debounce"`
	PendingFiles int      `json:"pendingFiles"`
	Subscribers  []string `json:"subscribers,omitempty"`
}

type CodeStatus struct {
	Status     string `json:"status"`
	Symbols    int    `json:"symbols"`
	References int    `json:"references"`
	Files      int    `json:"files"`
}

type FindingsStatus struct {
	Total      int                       `json:"total"`
	ByAnalyzer map[string]int            `json:"byAnalyzer"`
	BySeverity map[string]int            `json:"bySeverity"`
	Analyzers  map[string]AnalyzerStatus `json:"analyzers,omitempty"`
}

type AnalyzerStatus struct {
	Status       string `json:"status"`
	Scope        string `json:"scope,omitempty"`
	LastRun      string `json:"lastRun,omitempty"`
	Findings     int    `json:"findings"`
	LastDuration string `json:"lastDuration,omitempty"`
}

type StoreStatus struct {
	Paths map[string]string `json:"paths"`
	Sizes map[string]int64  `json:"sizes"`
}

var statusUsage = `aide status - Show aide internal status

Usage:
  aide status [flags]

Flags:
  --json, -j    Output as JSON
  --help, -h    Show this help

Shows:
  - File watcher status
  - Code indexer statistics
  - Findings analyzer status
  - Store paths and sizes
  - Environment variables
`

func cmdStatus(dbPath string, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json", "-j":
			jsonOutput = true
		case "--help", "-h":
			fmt.Print(statusUsage)
			return nil
		default:
			fmt.Printf("Unknown flag: %s\n", arg)
			fmt.Print(statusUsage)
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	projectRoot := getProjectRoot()

	status := StatusOutput{
		Version:   version.String(),
		Mode:      os.Getenv("AIDE_MODE"),
		Project:   projectRoot,
		Timestamp: time.Now(),
		Env:       getAideEnvVars(),
		Stores:    getStoreStatus(dbPath),
	}

	if globalWatcher != nil {
		stats := globalWatcher.Stats()
		status.Watcher = &WatcherStatus{
			Enabled:      stats.Enabled,
			Paths:        stats.Paths,
			DirsWatched:  stats.DirsWatched,
			Debounce:     stats.Debounce.String(),
			PendingFiles: stats.PendingFiles,
		}
		if globalCodeStore != nil {
			status.Watcher.Subscribers = append(status.Watcher.Subscribers, "code-indexer")
		}
		if globalFindingsRunner != nil {
			status.Watcher.Subscribers = append(status.Watcher.Subscribers, "findings")
		}
	}

	if globalCodeStore != nil {
		stats, err := globalCodeStore.Stats()
		if err == nil {
			status.Code = &CodeStatus{
				Status:     "idle",
				Symbols:    stats.Symbols,
				References: stats.References,
				Files:      stats.Files,
			}
		}
	}

	if globalFindingsStore != nil {
		stats, err := globalFindingsStore.Stats()
		if err == nil {
			status.Findings = &FindingsStatus{
				Total:      stats.Total,
				ByAnalyzer: stats.ByAnalyzer,
				BySeverity: stats.BySeverity,
			}
		}
		if globalFindingsRunner != nil {
			runnerStatus := globalFindingsRunner.GetStatus()
			if status.Findings != nil {
				status.Findings.Analyzers = make(map[string]AnalyzerStatus)
				for name, s := range runnerStatus {
					status.Findings.Analyzers[name] = AnalyzerStatus{
						Status:       s.Status,
						Scope:        s.Scope,
						LastRun:      formatTimeAgo(s.LastRun),
						Findings:     s.Findings,
						LastDuration: s.LastDuration.String(),
					}
				}
			}
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	return printStatusTable(status)
}

func printStatusTable(status StatusOutput) error {
	fmt.Println("AIDE Status")
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println()

	fmt.Printf("Version: %-20s Project: %s\n", status.Version, status.Project)
	if status.Mode != "" {
		fmt.Printf("Mode: %-22s", status.Mode)
	}
	fmt.Printf("Time: %s\n\n", status.Timestamp.Format("2006-01-02 15:04:05"))

	fmt.Println("FILE WATCHER")
	if status.Watcher != nil && status.Watcher.Enabled {
		fmt.Printf("  Status: enabled              Dirs: %d\n", status.Watcher.DirsWatched)
		paths := strings.Join(status.Watcher.Paths, ", ")
		if paths == "" {
			paths = "."
		}
		fmt.Printf("  Paths: %-20s Debounce: %s\n", paths, status.Watcher.Debounce)
		fmt.Printf("  Pending: %-4d              Subscribers: %s\n\n", status.Watcher.PendingFiles, strings.Join(status.Watcher.Subscribers, ", "))
	} else {
		fmt.Println("  Status: disabled")
		fmt.Println()
	}

	fmt.Println("CODE INDEXER")
	if status.Code != nil {
		fmt.Printf("  Status: %-18s Symbols: %d\n", status.Code.Status, status.Code.Symbols)
		fmt.Printf("  References: %-14d Files: %d\n\n", status.Code.References, status.Code.Files)
	} else {
		fmt.Println("  Status: not available")
		fmt.Println()
	}

	fmt.Println("FINDINGS ANALYZERS")
	if status.Findings != nil {
		fmt.Printf("  Total: %-20d By Analyzer:\n", status.Findings.Total)
		for a, c := range status.Findings.ByAnalyzer {
			fmt.Printf("    %-20s %d\n", a, c)
		}
		if len(status.Findings.Analyzers) > 0 {
			fmt.Println("\n  Analyzer     Scope            Status    Findings  Last Run")
			fmt.Println("  ───────────  ───────────────  ────────  ────────  ─────────")
			var names []string
			for name := range status.Findings.Analyzers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				s := status.Findings.Analyzers[name]
				scope := s.Scope
				if scope == "" {
					scope = "<project>"
				}
				if len(scope) > 16 {
					scope = scope[:13] + "..."
				}
				fmt.Printf("  %-12s %-16s %-9s %-8d %s\n", name, scope, s.Status, s.Findings, s.LastRun)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("  Status: not available")
		fmt.Println()
	}

	fmt.Println("STORES")
	for name, path := range status.Stores.Paths {
		size := status.Stores.Sizes[name]
		fmt.Printf("  %-12s %s (%s)\n", name+":", path, formatBytes(size))
	}
	fmt.Println()

	if len(status.Env) > 0 {
		fmt.Println("ENVIRONMENT")
		var keys []string
		for k := range status.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s=%s\n", k, status.Env[k])
		}
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func getStoreStatus(dbPath string) StoreStatus {
	status := StoreStatus{
		Paths: make(map[string]string),
		Sizes: make(map[string]int64),
	}

	aideDir := filepath.Dir(filepath.Dir(dbPath))

	mainPath := dbPath
	if info, err := os.Stat(mainPath); err == nil {
		status.Paths["aide.db"] = mainPath
		status.Sizes["aide.db"] = info.Size()
	}

	codePath := filepath.Join(aideDir, "code", "index.db")
	if info, err := os.Stat(codePath); err == nil {
		status.Paths["code.db"] = codePath
		status.Sizes["code.db"] = info.Size()
	}

	findingsPath := filepath.Join(aideDir, "findings", "findings.db")
	if info, err := os.Stat(findingsPath); err == nil {
		status.Paths["findings.db"] = findingsPath
		status.Sizes["findings.db"] = info.Size()
	}

	return status
}

func getAideEnvVars() map[string]string {
	vars := make(map[string]string)
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "AIDE_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				vars[parts[0]] = parts[1]
			}
		}
	}
	return vars
}

func getProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

var (
	globalWatcher   *watcher.Watcher
	globalCodeStore interface {
		Stats() (*code.IndexStats, error)
	}
	globalFindingsStore interface {
		Stats() (*findings.Stats, error)
	}
	globalFindingsRunner *findings.Runner
)
