// Package main provides MCP server implementation for aide.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/findings/clone"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpLog logs to stderr (stdout is reserved for MCP JSON-RPC protocol)
var mcpLog = log.New(os.Stderr, "[aide-mcp] ", log.Ltime)

// MCPServer wraps the aide store for MCP tool access.
type MCPServer struct {
	store          store.Store
	codeStore      store.CodeIndexStore
	findingsStore  store.FindingsStore
	codeStoreMu    sync.RWMutex
	codeStoreReady atomic.Bool
	codeInitWg     sync.WaitGroup
	server         *mcp.Server
	grpcServer     *grpcapi.Server
	codeWatcher    *code.Watcher
	codeWatcherMu  sync.Mutex
	grammarLoader  *grammar.CompositeLoader

	unifiedWatcher   *watcher.Watcher
	findingsRunner   *findings.Runner
	unifiedWatcherMu sync.Mutex

	toolCounts sync.Map // map[string]*atomic.Int64
}

// getCodeStore safely returns the code store (may be nil during lazy init).
func (s *MCPServer) getCodeStore() store.CodeIndexStore {
	if !s.codeStoreReady.Load() {
		return nil
	}
	s.codeStoreMu.RLock()
	defer s.codeStoreMu.RUnlock()
	return s.codeStore
}

// setCodeStore safely sets the code store after lazy init.
func (s *MCPServer) setCodeStore(cs store.CodeIndexStore) {
	s.codeStoreMu.Lock()
	s.codeStore = cs
	s.codeStoreMu.Unlock()
	s.codeStoreReady.Store(true)
}

// incrementToolCount atomically increments the execution count for a tool.
func (s *MCPServer) incrementToolCount(name string) {
	v, _ := s.toolCounts.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// getToolCounts returns a snapshot of tool execution counts.
func (s *MCPServer) getToolCounts() map[string]int64 {
	counts := make(map[string]int64)
	s.toolCounts.Range(func(key, value any) bool {
		counts[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	return counts
}

// toolCountMiddleware returns MCP middleware that counts tool invocations.
func (s *MCPServer) toolCountMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
					s.incrementToolCount(params.Name)
				}
			}
			return next(ctx, method, req)
		}
	}
}

// mcpConfig holds parsed configuration for the MCP server.
type mcpConfig struct {
	codeWatch         bool
	codeWatchPath     string
	codeWatchDelayStr string
	codeStoreDisabled bool
	codeStoreLazy     bool
}

// parseMCPArgs validates flags and returns parsed config. Returns nil if help was printed.
func parseMCPArgs(args []string) (*mcpConfig, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printMCPUsage()
			return nil, nil
		}
		if err := validateMCPFlag(arg); err != nil {
			return nil, err
		}
	}

	cfg := &mcpConfig{
		codeWatch:         hasFlag(args, "--code-watch") || os.Getenv("AIDE_CODE_WATCH") == "1",
		codeWatchPath:     parseFlag(args, "--code-watch="),
		codeWatchDelayStr: parseFlag(args, "--code-watch-delay="),
		codeStoreDisabled: os.Getenv("AIDE_CODE_STORE_DISABLE") == "1",
		codeStoreLazy:     os.Getenv("AIDE_CODE_STORE_SYNC") != "1",
	}
	if cfg.codeWatchPath == "" {
		cfg.codeWatchPath = os.Getenv("AIDE_CODE_WATCH_PATHS")
	}
	if cfg.codeWatchDelayStr == "" {
		cfg.codeWatchDelayStr = os.Getenv("AIDE_CODE_WATCH_DELAY")
	}
	return cfg, nil
}

// validateMCPFlag checks that a flag argument is recognized.
func validateMCPFlag(arg string) error {
	if strings.HasPrefix(arg, "--") {
		known := []string{"--code-watch", "--code-watch=", "--code-watch-delay="}
		for _, k := range known {
			if arg == k || strings.HasPrefix(arg, k) {
				return nil
			}
		}
		return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
	}
	if strings.HasPrefix(arg, "-") {
		return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
	}
	return nil
}

// initMCPCodeStore sets up the code store (lazy or sync) and returns a cleanup function.
func (s *MCPServer) initMCPCodeStore(dbPath string, cfg *mcpConfig, grpcServer *grpcapi.Server) func() {
	if cfg.codeStoreDisabled {
		mcpLog.Printf("code store: disabled")
		return nil
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	openCodeStore := func() (*store.CodeStore, error) {
		codeStart := time.Now()
		cs, err := store.NewCodeStore(indexPath, searchPath)
		if err != nil {
			return nil, err
		}
		mcpLog.Printf("code index opened in %v: %s", time.Since(codeStart), indexPath)
		return cs, nil
	}

	if cfg.codeStoreLazy {
		mcpLog.Printf("code store: lazy init enabled")
		s.codeInitWg.Add(1)
		go func() {
			defer s.codeInitWg.Done()
			time.Sleep(100 * time.Millisecond)
			cs, err := openCodeStore()
			if err != nil {
				mcpLog.Printf("WARNING: lazy code store init failed: %v", err)
				return
			}
			s.setCodeStore(cs)
			grpcServer.SetCodeStore(cs)
		}()
		return func() {
			s.codeInitWg.Wait() // Ensure lazy init completes before closing
			if cs := s.getCodeStore(); cs != nil {
				cs.Close()
			}
		}
	}

	// Synchronous initialization (AIDE_CODE_STORE_SYNC=1)
	cs, err := openCodeStore()
	if err != nil {
		mcpLog.Printf("WARNING: failed to open code store: %v (code tools disabled)", err)
		return nil
	}
	s.setCodeStore(cs)
	grpcServer.SetCodeStore(cs)
	return func() { cs.Close() }
}

// initMCPFindingsStore opens the findings store and registers it with gRPC.
func (s *MCPServer) initMCPFindingsStore(dbPath string, grpcServer *grpcapi.Server) func() {
	findingsDir := getFindingsStorePath(dbPath)

	findingsStart := time.Now()
	fs, err := store.NewFindingsStore(findingsDir)
	if err != nil {
		mcpLog.Printf("WARNING: failed to open findings store: %v (findings tools disabled)", err)
		return nil
	}
	mcpLog.Printf("findings store opened in %v: %s", time.Since(findingsStart), findingsDir)

	s.findingsStore = fs
	grpcServer.SetFindingsStore(fs)
	return func() { fs.Close() }
}

// startCodeWatcher launches the file watcher in the background.
// It reuses the MCPServer's existing code store to avoid double-opening bolt/bleve.
func (s *MCPServer) startCodeWatcher(dbPath string, cfg *mcpConfig) {
	if !cfg.codeWatch && cfg.codeWatchPath == "" {
		return
	}

	go func() {
		if cfg.codeStoreLazy {
			for i := 0; i < 50; i++ {
				if s.codeStoreReady.Load() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		var indexer *Indexer
		if cs := s.getCodeStore(); cs != nil {
			indexer = NewIndexerFromStore(cs, s.grammarLoader)
		} else {
			var err error
			indexer, err = NewIndexer(dbPath)
			if err != nil {
				mcpLog.Printf("WARNING: failed to create code indexer: %v", err)
				return
			}
		}

		debounceDelay := watcher.DefaultDebounceDelay
		if cfg.codeWatchDelayStr != "" {
			if d, err := time.ParseDuration(cfg.codeWatchDelayStr); err == nil {
				debounceDelay = d
			}
		}

		var watchPaths []string
		if cfg.codeWatchPath != "" {
			watchPaths = strings.Split(cfg.codeWatchPath, ",")
		}

		codeHandler := &codeIndexHandler{indexer: indexer}

		// Build handler list — always include code indexer, add findings runner if store is available
		handlers := []watcher.FileChangeHandler{codeHandler}

		var findingsRunner *findings.Runner
		if s.findingsStore != nil {
			// Load .aideignore from project root for findings filtering.
			projectRoot := projectRootFromDB(dbPath)
			ignore, err := aideignore.New(projectRoot)
			if err != nil {
				mcpLog.Printf("WARNING: failed to load .aideignore: %v (using defaults)", err)
				ignore = aideignore.NewFromDefaults()
			}

			// Load analyser thresholds from .aide/config/aide.json.
			fcfg := loadFindingsConfig(projectRoot)

			runnerConfig := findings.AnalyzerConfig{
				Paths:               watchPaths,
				Ignore:              ignore,
				ProjectRoot:         projectRoot,
				ComplexityThreshold: fcfg.Complexity.Threshold,
				FanOutThreshold:     fcfg.Coupling.FanOut,
				FanInThreshold:      fcfg.Coupling.FanIn,
				CloneWindowSize:     fcfg.Clones.WindowSize,
				CloneMinLines:       fcfg.Clones.MinLines,
			}
			findingsRunner = findings.NewRunner(s.findingsStore, runnerConfig)
			findingsRunner.SetClonesRunner(func(ctx context.Context, paths []string, windowSize, minLines int) ([]*findings.Finding, error) {
				cloneCfg := clone.Config{
					Paths:         paths,
					WindowSize:    windowSize,
					MinCloneLines: minLines,
					Ignore:        ignore,
				}
				f, _, err := clone.DetectClones(cloneCfg)
				return f, err
			})
			handlers = append(handlers, findingsRunner)
		} else {
			mcpLog.Printf("WARNING: findings store not available, findings analysis disabled in watcher")
		}

		w, err := watcher.New(watcher.Config{
			Paths:         watchPaths,
			DebounceDelay: debounceDelay,
			FileFilter:    code.SupportedFile,
		}, handlers...)
		if err != nil {
			mcpLog.Printf("WARNING: failed to create unified watcher: %v", err)
			return
		}

		if err := w.Start(); err != nil {
			mcpLog.Printf("WARNING: failed to start unified watcher: %v", err)
			return
		}

		s.unifiedWatcherMu.Lock()
		s.unifiedWatcher = w
		s.findingsRunner = findingsRunner
		s.unifiedWatcherMu.Unlock()

		// Expose watcher/runner to gRPC StatusService
		if s.grpcServer != nil {
			s.grpcServer.SetWatcher(w)
			s.grpcServer.SetFindingsRunner(findingsRunner)
		}

		if len(watchPaths) > 0 {
			mcpLog.Printf("unified watcher enabled for: %s (debounce: %v)", strings.Join(watchPaths, ", "), debounceDelay)
		} else {
			mcpLog.Printf("unified watcher enabled for current directory (debounce: %v)", debounceDelay)
		}
	}()
}

type codeIndexHandler struct {
	indexer *Indexer
}

func (h *codeIndexHandler) OnChanges(files map[string]fsnotify.Op) {
	for path, op := range files {
		if watcher.IsRemove(op) {
			if err := h.indexer.RemoveFile(path); err != nil {
				mcpLog.Printf("failed to remove %s: %v", path, err)
			} else {
				mcpLog.Printf("removed %s from index", path)
			}
		} else {
			count, err := h.indexer.IndexFile(path)
			if err != nil {
				mcpLog.Printf("failed to index %s: %v", path, err)
			} else {
				mcpLog.Printf("indexed %s: %d symbols", path, count)
			}
		}
	}
}

// stopCodeWatcher gracefully stops the file watcher if running.
func (s *MCPServer) stopCodeWatcher() {
	s.unifiedWatcherMu.Lock()
	w := s.unifiedWatcher
	runner := s.findingsRunner
	s.unifiedWatcher = nil
	s.findingsRunner = nil
	s.unifiedWatcherMu.Unlock()

	if runner != nil {
		runner.Stop()
	}
	if w != nil {
		if err := w.Stop(); err != nil {
			mcpLog.Printf("WARNING: watcher stop error: %v", err)
		}
	}
}

// cmdMCP starts the MCP server over stdio.
// It first attempts to connect to an existing gRPC socket (client mode),
// allowing multiple MCP instances to share a single BoltDB. If no socket
// exists, it opens BoltDB directly and becomes the primary (server mode).
func cmdMCP(dbPath string, args []string) error {
	cfg, err := parseMCPArgs(args)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil // --help was printed
	}

	startTime := time.Now()

	if os.Getenv("AIDE_PPROF_ENABLE") == "1" {
		initPprof()
		defer stopPprof()
	}

	mcpLog.Printf("aide MCP server starting")
	mcpLog.Printf("version: %s", version.String())
	mcpLog.Printf("database: %s", dbPath)

	grammarLoader := newGrammarLoader(dbPath)
	socketPath := grpcapi.SocketPathFromDB(dbPath)

	// Try client mode first: connect to existing primary via gRPC socket
	if grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			pingErr := client.Ping(ctx)
			cancel()
			if pingErr == nil {
				mcpLog.Printf("client mode: connected to existing primary via %s", socketPath)
				adapter := newGRPCStoreAdapter(client)
				findingsAdapter := newGRPCFindingsAdapter(client)
				mcpServer := &MCPServer{store: adapter, findingsStore: findingsAdapter, grammarLoader: grammarLoader}
				mcpLog.Printf("MCP server ready in %v (client mode), listening on stdio", time.Since(startTime))
				return mcpServer.Run()
			}
			client.Close()
			mcpLog.Printf("existing socket unhealthy, becoming primary")
			// Remove stale socket so we can become primary
			os.Remove(socketPath)
		} else {
			mcpLog.Printf("socket exists but connection failed: %v, becoming primary", err)
			os.Remove(socketPath)
		}
	}

	// Primary mode: open CombinedStore (bolt + bleve) directly and serve gRPC for other instances
	mcpLog.Printf("primary mode: opening database directly")
	storeStart := time.Now()
	st, err := store.NewCombinedStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer st.Close()
	mcpLog.Printf("database opened in %v (bolt + bleve search)", time.Since(storeStart))

	mcpServer := &MCPServer{store: st, grammarLoader: grammarLoader}

	grpcServer := grpcapi.NewServer(st, dbPath, socketPath, grammarLoader)
	mcpServer.grpcServer = grpcServer
	mcpLog.Printf("gRPC socket: %s", socketPath)

	// Initialize stores BEFORE starting gRPC server.
	// grpcServer.Start() registers service implementations that capture store
	// references at registration time, so stores must be set first.
	if cleanup := mcpServer.initMCPCodeStore(dbPath, cfg, grpcServer); cleanup != nil {
		defer cleanup()
	}

	if cleanup := mcpServer.initMCPFindingsStore(dbPath, grpcServer); cleanup != nil {
		defer cleanup()
	}

	go func() {
		if err := grpcServer.Start(); err != nil {
			mcpLog.Printf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

	mcpServer.startCodeWatcher(dbPath, cfg)
	defer mcpServer.stopCodeWatcher()

	mcpLog.Printf("MCP server ready in %v (primary mode), listening on stdio", time.Since(startTime))
	return mcpServer.Run()
}

// Run starts the MCP server and registers all tools.
func (s *MCPServer) Run() error {
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "aide",
			Version: version.Short(),
		},
		nil, // Use default capabilities
	)
	s.server = srv

	// Track tool execution counts
	srv.AddReceivingMiddleware(s.toolCountMiddleware())

	// Register tools - only data layer, not orchestration
	// Task management and state mutations are handled by hooks/skills
	s.registerMemoryTools()
	s.registerStateReadTools() // Read-only state access
	s.registerDecisionTools()
	s.registerMessageTools()
	s.registerCodeTools()     // Code indexing and search
	s.registerFindingsTools() // Findings search and stats

	// Expose registered MCP tools and count getter to gRPC StatusService
	if s.grpcServer != nil {
		s.grpcServer.SetMCPTools(mcpToolList())
		s.grpcServer.SetToolCountFunc(s.getToolCounts)
	}

	// Run over stdio
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}

// mcpToolList returns the static list of MCP tools registered by the server.
func mcpToolList() []*grpcapi.StatusMCPTool {
	return []*grpcapi.StatusMCPTool{
		{Name: "memory_search", Category: "memory"},
		{Name: "memory_list", Category: "memory"},
		{Name: "state_get", Category: "state"},
		{Name: "state_list", Category: "state"},
		{Name: "decision_get", Category: "decision"},
		{Name: "decision_history", Category: "decision"},
		{Name: "decision_list", Category: "decision"},
		{Name: "message_list", Category: "message"},
		{Name: "message_send", Category: "message"},
		{Name: "message_ack", Category: "message"},
		{Name: "code_search", Category: "code"},
		{Name: "code_symbols", Category: "code"},
		{Name: "code_stats", Category: "code"},
		{Name: "code_references", Category: "code"},
		{Name: "code_outline", Category: "code"},
		{Name: "findings_search", Category: "findings"},
		{Name: "findings_list", Category: "findings"},
		{Name: "findings_stats", Category: "findings"},
	}
}

// printMCPUsage prints help for the mcp command.
func printMCPUsage() {
	fmt.Printf(`aide mcp - Start MCP server for Claude Code plugin integration

Usage:
  aide mcp [flags]

Flags:
  --code-watch           Enable file watching for code index updates
  --code-watch=<paths>   Comma-separated paths to watch
  --code-watch-delay=<d> Debounce delay for watcher (e.g., 30s)
  --help, -h             Show this help

Environment Variables:
  AIDE_CODE_WATCH=1         Enable file watching
  AIDE_CODE_WATCH_PATHS     Comma-separated paths to watch
  AIDE_CODE_WATCH_DELAY     Debounce delay (default: 30s)
  AIDE_CODE_STORE_DISABLE=1 Disable code store entirely
  AIDE_CODE_STORE_SYNC=1    Force synchronous code store init (default: lazy)
  AIDE_PPROF_ENABLE=1       Enable pprof profiling (requires -tags pprof build)
  AIDE_PPROF_ADDR           pprof server address (default: localhost:6060)

The MCP server communicates over stdio using JSON-RPC protocol.
It is typically started by Claude Code via the plugin configuration.
`)
}
