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

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpLog logs to stderr (stdout is reserved for MCP JSON-RPC protocol)
var mcpLog = log.New(os.Stderr, "[aide-mcp] ", log.Ltime)

// MCPServer wraps the aide store for MCP tool access.
type MCPServer struct {
	store          store.Store
	codeStore      store.CodeIndexStore
	codeStoreMu    sync.RWMutex // Protects codeStore during lazy init
	codeStoreReady atomic.Bool  // Fast check if codeStore is ready
	server         *mcp.Server
	grpcServer     *grpcapi.Server
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

// cmdMCP starts the MCP server over stdio.
func cmdMCP(dbPath string, args []string) error {
	// Handle --help and unknown flags before starting the server
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printMCPUsage()
			return nil
		}
		// Check for unknown flags (must start with --)
		if strings.HasPrefix(arg, "--") {
			// Known flags
			known := []string{
				"--code-watch",
				"--code-watch=",
				"--code-watch-delay=",
			}
			isKnown := false
			for _, k := range known {
				if arg == k || strings.HasPrefix(arg, k) {
					isKnown = true
					break
				}
			}
			if !isKnown {
				return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
			}
		} else if strings.HasPrefix(arg, "-") {
			// Short flags (only -h is supported)
			return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
		}
	}

	startTime := time.Now()

	// Start pprof server if enabled (requires build with -tags pprof)
	if os.Getenv("AIDE_PPROF_ENABLE") == "1" {
		initPprof()
	}

	// Parse flags (CLI flags override environment variables)
	codeWatch := hasFlag(args, "--code-watch") || os.Getenv("AIDE_CODE_WATCH") == "1"
	codeWatchPath := parseFlag(args, "--code-watch=")
	if codeWatchPath == "" {
		codeWatchPath = os.Getenv("AIDE_CODE_WATCH_PATHS")
	}
	codeWatchDelayStr := parseFlag(args, "--code-watch-delay=")
	if codeWatchDelayStr == "" {
		codeWatchDelayStr = os.Getenv("AIDE_CODE_WATCH_DELAY")
	}

	// Check if code store should be disabled entirely
	codeStoreDisabled := os.Getenv("AIDE_CODE_STORE_DISABLE") == "1"
	// Code store is lazy by default for faster MCP startup.
	// Set AIDE_CODE_STORE_SYNC=1 to force synchronous initialization.
	// Legacy AIDE_CODE_STORE_LAZY=1 is now the default behavior.
	codeStoreSync := os.Getenv("AIDE_CODE_STORE_SYNC") == "1"
	codeStoreLazy := !codeStoreSync

	// Print startup banner to stderr
	mcpLog.Printf("aide MCP server starting")
	mcpLog.Printf("version: %s", version.String())
	mcpLog.Printf("database: %s", dbPath)

	// Open the store (critical path - must be synchronous)
	storeStart := time.Now()
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer st.Close()
	mcpLog.Printf("database opened in %v", time.Since(storeStart))

	// Create MCP server early so we can start serving while other components init
	mcpServer := &MCPServer{store: st}

	// Create gRPC server for CLI access
	socketPath := grpcapi.SocketPathFromDB(dbPath)
	grpcServer := grpcapi.NewServer(st, dbPath, socketPath)
	mcpServer.grpcServer = grpcServer
	mcpLog.Printf("gRPC socket: %s", socketPath)

	// Start gRPC server in background (non-blocking)
	go func() {
		if err := grpcServer.Start(); err != nil {
			mcpLog.Printf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Initialize code store (either sync, lazy, or disabled)
	var codeStoreCleanup func()
	if codeStoreDisabled {
		mcpLog.Printf("code store: disabled")
	} else {
		indexPath, searchPath := getCodeStorePaths(dbPath)

		initCodeStore := func() (*store.CodeStore, error) {
			codeStart := time.Now()
			cs, err := store.NewCodeStore(indexPath, searchPath)
			if err != nil {
				return nil, err
			}
			mcpLog.Printf("code index opened in %v: %s", time.Since(codeStart), indexPath)
			return cs, nil
		}

		if codeStoreLazy {
			// Lazy initialization - start after MCP server is ready
			mcpLog.Printf("code store: lazy init enabled")
			go func() {
				// Small delay to ensure MCP server is accepting connections
				time.Sleep(100 * time.Millisecond)
				cs, err := initCodeStore()
				if err != nil {
					mcpLog.Printf("WARNING: lazy code store init failed: %v", err)
					return
				}
				mcpServer.setCodeStore(cs)
				grpcServer.SetCodeStore(cs)
			}()
			codeStoreCleanup = func() {
				if cs := mcpServer.getCodeStore(); cs != nil {
					cs.Close()
				}
			}
		} else {
			// Synchronous initialization (requires AIDE_CODE_STORE_SYNC=1)
			codeStore, err := initCodeStore()
			if err != nil {
				mcpLog.Printf("WARNING: failed to open code store: %v (code tools disabled)", err)
			} else {
				mcpServer.setCodeStore(codeStore)
				grpcServer.SetCodeStore(codeStore)
				codeStoreCleanup = func() { codeStore.Close() }
			}
		}
	}
	if codeStoreCleanup != nil {
		defer codeStoreCleanup()
	}

	// Start code watcher if enabled (always in background)
	if codeWatch || codeWatchPath != "" {
		go func() {
			// Wait for code store if using lazy init
			if codeStoreLazy {
				for i := 0; i < 50; i++ { // Wait up to 5 seconds
					if mcpServer.codeStoreReady.Load() {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			indexer, err := NewIndexer(dbPath)
			if err != nil {
				mcpLog.Printf("WARNING: failed to create code indexer: %v", err)
				return
			}

			// Parse debounce delay
			debounceDelay := code.DefaultDebounceDelay // 30s default
			if codeWatchDelayStr != "" {
				if d, err := time.ParseDuration(codeWatchDelayStr); err == nil {
					debounceDelay = d
				}
			}

			// Determine paths to watch
			var watchPaths []string
			if codeWatchPath != "" {
				watchPaths = strings.Split(codeWatchPath, ",")
			}

			config := code.WatcherConfig{
				Enabled:       true,
				Paths:         watchPaths,
				DebounceDelay: debounceDelay,
			}

			codeWatcher, err := code.WatchAndIndex(config, indexer.IndexFile, indexer.RemoveFile)
			if err != nil {
				mcpLog.Printf("WARNING: failed to create code watcher: %v", err)
				return
			}
			if err := codeWatcher.Start(); err != nil {
				mcpLog.Printf("WARNING: failed to start code watcher: %v", err)
				return
			}
			if len(watchPaths) > 0 {
				mcpLog.Printf("code watcher enabled for: %s (debounce: %v)", strings.Join(watchPaths, ", "), debounceDelay)
			} else {
				mcpLog.Printf("code watcher enabled for current directory (debounce: %v)", debounceDelay)
			}
		}()
	}

	mcpLog.Printf("MCP server ready in %v, listening on stdio", time.Since(startTime))
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

	// Register tools - only data layer, not orchestration
	// Task management and state mutations are handled by hooks/skills
	s.registerMemoryTools()
	s.registerStateReadTools() // Read-only state access
	s.registerDecisionTools()
	s.registerMessageTools()
	s.registerCodeTools() // Code indexing and search

	// Run over stdio
	return srv.Run(context.Background(), &mcp.StdioTransport{})
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
