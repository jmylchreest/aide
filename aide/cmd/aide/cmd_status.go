package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

type StatusOutput struct {
	Version       string            `json:"version"`
	Mode          string            `json:"mode,omitempty"`
	Project       string            `json:"project"`
	Timestamp     time.Time         `json:"timestamp"`
	ServerRunning bool              `json:"serverRunning"`
	Uptime        string            `json:"uptime,omitempty"`
	Watcher       *WatcherStatus    `json:"watcher,omitempty"`
	Code          *CodeStatus       `json:"codeIndexer,omitempty"`
	Findings      *FindingsStatus   `json:"findings,omitempty"`
	MCPTools      []MCPToolStatus   `json:"mcpTools,omitempty"`
	Stores        StoreStatus       `json:"stores"`
	Env           map[string]string `json:"environment,omitempty"`
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

type MCPToolStatus struct {
	Name           string `json:"name"`
	Category       string `json:"category"`
	ExecutionCount int64  `json:"executionCount"`
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
  - Server status (connected via gRPC or standalone)
  - File watcher status
  - Code indexer statistics
  - Findings analyser status
  - MCP tools with execution counts
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

	// Try gRPC first — query the running MCP server for live status
	if grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if err == nil {
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Status.GetStatus(ctx, &grpcapi.StatusRequest{})
			if err == nil {
				populateFromGRPC(&status, resp)
			}
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	printStatusTable(status)
	return nil
}

// populateFromGRPC fills the StatusOutput from a gRPC StatusResponse.
func populateFromGRPC(status *StatusOutput, resp *grpcapi.StatusResponse) {
	status.ServerRunning = resp.ServerRunning
	status.Uptime = resp.Uptime

	// Override version with server version if available
	if resp.Version != "" {
		status.Version = resp.Version
	}

	// Watcher
	if w := resp.Watcher; w != nil {
		status.Watcher = &WatcherStatus{
			Enabled:      w.Enabled,
			Paths:        w.Paths,
			DirsWatched:  int(w.DirsWatched),
			Debounce:     w.Debounce,
			PendingFiles: int(w.PendingFiles),
			Subscribers:  w.Subscribers,
		}
	}

	// Code indexer
	if ci := resp.CodeIndexer; ci != nil && ci.Available {
		status.Code = &CodeStatus{
			Status:     ci.Status,
			Symbols:    int(ci.Symbols),
			References: int(ci.References),
			Files:      int(ci.Files),
		}
	}

	// Findings
	if f := resp.Findings; f != nil && f.Available {
		byAnalyzer := make(map[string]int, len(f.ByAnalyzer))
		for k, v := range f.ByAnalyzer {
			byAnalyzer[k] = int(v)
		}
		bySeverity := make(map[string]int, len(f.BySeverity))
		for k, v := range f.BySeverity {
			bySeverity[k] = int(v)
		}
		status.Findings = &FindingsStatus{
			Total:      int(f.Total),
			ByAnalyzer: byAnalyzer,
			BySeverity: bySeverity,
		}
		if len(f.Analyzers) > 0 {
			analyzers := make(map[string]AnalyzerStatus, len(f.Analyzers))
			for name, a := range f.Analyzers {
				analyzers[name] = AnalyzerStatus{
					Status:       a.Status,
					Scope:        a.Scope,
					LastRun:      a.LastRun,
					Findings:     int(a.Findings),
					LastDuration: a.LastDuration,
				}
			}
			status.Findings.Analyzers = analyzers
		}
	}

	// MCP tools
	if len(resp.McpTools) > 0 {
		tools := make([]MCPToolStatus, len(resp.McpTools))
		for i, t := range resp.McpTools {
			tools[i] = MCPToolStatus{
				Name:           t.Name,
				Category:       t.Category,
				ExecutionCount: t.ExecutionCount,
			}
		}
		status.MCPTools = tools
	}
}

func printStatusTable(status StatusOutput) {
	fmt.Println("AIDE Status")
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println()

	// Header: version and project on separate lines since version string can be long
	fmt.Printf("  Version:  %s\n", status.Version)
	fmt.Printf("  Project:  %s\n", status.Project)
	if status.Mode != "" {
		fmt.Printf("  Mode:     %s\n", status.Mode)
	}
	fmt.Printf("  Time:     %s\n", status.Timestamp.Format("2006-01-02 15:04:05"))
	if status.ServerRunning {
		fmt.Printf("  Server:   running (uptime: %s)\n", status.Uptime)
	} else {
		fmt.Printf("  Server:   not running\n")
	}
	fmt.Println()

	printWatcherStatus(status.Watcher)
	printCodeStatus(status.Code)
	printFindingsStatus(status.Findings)
	printMCPToolsStatus(status.MCPTools)
	printStoresStatus(status.Stores)

	// Environment
	if len(status.Env) > 0 {
		fmt.Println("ENVIRONMENT")
		keys := make([]string, 0, len(status.Env))
		for k := range status.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s=%s\n", k, status.Env[k])
		}
	}
}

func printWatcherStatus(w *WatcherStatus) {
	fmt.Println("FILE WATCHER")
	if w != nil && w.Enabled {
		paths := strings.Join(w.Paths, ", ")
		if paths == "" {
			paths = "."
		}
		fmt.Printf("  Status:       enabled\n")
		fmt.Printf("  Paths:        %s\n", paths)
		fmt.Printf("  Dirs:         %-20d Debounce:    %s\n", w.DirsWatched, w.Debounce)
		fmt.Printf("  Pending:      %-20d Subscribers: %s\n", w.PendingFiles, strings.Join(w.Subscribers, ", "))
	} else {
		fmt.Println("  Status:       disabled")
	}
	fmt.Println()
}

func printCodeStatus(c *CodeStatus) {
	fmt.Println("CODE INDEXER")
	if c != nil {
		fmt.Printf("  Status:       %-20s Symbols:     %d\n", c.Status, c.Symbols)
		fmt.Printf("  Files:        %-20d References:  %d\n", c.Files, c.References)
	} else {
		fmt.Println("  Status:       not available")
	}
	fmt.Println()
}

func printFindingsStatus(f *FindingsStatus) {
	fmt.Println("FINDINGS ANALYSERS")
	if f != nil {
		fmt.Printf("  Total: %d", f.Total)
		if len(f.ByAnalyzer) > 0 {
			names := make([]string, 0, len(f.ByAnalyzer))
			for name := range f.ByAnalyzer {
				names = append(names, name)
			}
			sort.Strings(names)
			parts := make([]string, 0, len(names))
			for _, name := range names {
				parts = append(parts, fmt.Sprintf("%s: %d", name, f.ByAnalyzer[name]))
			}
			fmt.Printf("  (%s)", strings.Join(parts, ", "))
		}
		fmt.Println()
		if len(f.Analyzers) > 0 {
			fmt.Println()
			fmt.Println("  Analyser     Scope            Status    Findings  Last Run")
			fmt.Println("  ───────────  ───────────────  ────────  ────────  ─────────")
			anames := make([]string, 0, len(f.Analyzers))
			for name := range f.Analyzers {
				anames = append(anames, name)
			}
			sort.Strings(anames)
			for _, name := range anames {
				s := f.Analyzers[name]
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
	} else {
		fmt.Println("  Status:       not available")
	}
	fmt.Println()
}

func printMCPToolsStatus(tools []MCPToolStatus) {
	fmt.Println("MCP TOOLS")
	if len(tools) > 0 {
		// Group tools by category, track counts
		type toolInfo struct {
			name  string
			count int64
		}
		categories := make(map[string][]toolInfo)
		var totalCalls int64
		for _, t := range tools {
			categories[t.Category] = append(categories[t.Category], toolInfo{t.Name, t.ExecutionCount})
			totalCalls += t.ExecutionCount
		}
		cats := make([]string, 0, len(categories))
		for c := range categories {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			catTools := categories[cat]
			sort.Slice(catTools, func(i, j int) bool { return catTools[i].name < catTools[j].name })
			var toolStrs []string
			for _, t := range catTools {
				if t.count > 0 {
					toolStrs = append(toolStrs, fmt.Sprintf("%s(%d)", t.name, t.count))
				} else {
					toolStrs = append(toolStrs, t.name)
				}
			}
			fmt.Printf("  %-12s %s\n", cat+":", strings.Join(toolStrs, ", "))
		}
		fmt.Printf("  Total: %d tools, %d calls", len(tools), totalCalls)
		if totalCalls > 0 {
			// List every tool with calls > 0
			var calledTools []string
			for _, cat := range cats {
				catTools := categories[cat]
				for _, t := range catTools {
					if t.count > 0 {
						calledTools = append(calledTools, fmt.Sprintf("%s: %d", t.name, t.count))
					}
				}
			}
			fmt.Printf(" (%s)", strings.Join(calledTools, ", "))
		}
		fmt.Println()
	} else {
		fmt.Println("  Not available (server not running)")
	}
	fmt.Println()
}

func printStoresStatus(stores StoreStatus) {
	fmt.Println("STORES")
	storeOrder := []string{"memory.db", "memory.bleve", "code.db", "code.bleve", "findings.db", "findings.bleve"}
	printed := make(map[string]bool)
	for _, name := range storeOrder {
		path, ok := stores.Paths[name]
		if !ok {
			continue
		}
		size := stores.Sizes[name]
		fmt.Printf("  %-16s %s (%s)\n", name+":", path, formatBytes(size))
		printed[name] = true
	}
	// Any stores not in the predefined order
	var extra []string
	for name := range stores.Paths {
		if !printed[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		size := stores.Sizes[name]
		fmt.Printf("  %-16s %s (%s)\n", name+":", stores.Paths[name], formatBytes(size))
	}
	fmt.Println()
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

func getStoreStatus(dbPath string) StoreStatus {
	status := StoreStatus{
		Paths: make(map[string]string),
		Sizes: make(map[string]int64),
	}

	// dbPath is .aide/memory/memory.db (or legacy .aide/memory/store.db)
	memoryDir := filepath.Dir(dbPath)

	// memory.db — main memory store
	if info, err := os.Stat(dbPath); err == nil {
		status.Paths["memory.db"] = dbPath
		status.Sizes["memory.db"] = info.Size()
	}

	// memory search.bleve — memory full-text index
	memorySearchPath := filepath.Join(memoryDir, "search.bleve")
	if size, err := dirSize(memorySearchPath); err == nil {
		status.Paths["memory.bleve"] = memorySearchPath
		status.Sizes["memory.bleve"] = size
	}

	// code.db — code symbol index
	codePath := filepath.Join(memoryDir, "code", "index.db")
	if info, err := os.Stat(codePath); err == nil {
		status.Paths["code.db"] = codePath
		status.Sizes["code.db"] = info.Size()
	}

	// code search.bleve — code full-text index
	codeSearchPath := filepath.Join(memoryDir, "code", "search.bleve")
	if size, err := dirSize(codeSearchPath); err == nil {
		status.Paths["code.bleve"] = codeSearchPath
		status.Sizes["code.bleve"] = size
	}

	// findings.db — findings store
	findingsPath := filepath.Join(memoryDir, "findings", "findings.db")
	if info, err := os.Stat(findingsPath); err == nil {
		status.Paths["findings.db"] = findingsPath
		status.Sizes["findings.db"] = info.Size()
	}

	// findings search index — check both search.bleve (new) and findings.idx (legacy)
	findingsSearchPath := filepath.Join(memoryDir, "findings", "search.bleve")
	if _, err := os.Stat(findingsSearchPath); os.IsNotExist(err) {
		legacyFindingsSearch := filepath.Join(memoryDir, "findings", "findings.idx")
		if _, err := os.Stat(legacyFindingsSearch); err == nil {
			findingsSearchPath = legacyFindingsSearch
		}
	}
	if size, err := dirSize(findingsSearchPath); err == nil {
		status.Paths["findings.bleve"] = findingsSearchPath
		status.Sizes["findings.bleve"] = size
	}

	return status
}

// dirSize returns the total size of all files in a directory tree.
func dirSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	_ = filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, nil
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
