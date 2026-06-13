// Package main provides the daemon command for aide.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/registry"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// runCleanupLoop prunes stale entries from buckets that don't self-clean:
// agent-specific state (cleared via SubagentStop but lingers when sessions
// crash) and observe events (high-volume, no per-write TTL). Messages
// already self-prune on read.
//
// Returns when ctx is cancelled. Errors are logged but never fatal — a
// daemon should never die from a cleanup hiccup.
func runCleanupLoop(ctx context.Context, st store.Store) {
	cfg := config.Get().Cleanup
	if !cfg.Enabled {
		fmt.Println("cleanup loop disabled (cleanup.enabled=false)")
		return
	}

	interval := cfg.IntervalDuration()
	stateAge := cfg.StateMaxAgeDuration()
	observeAge := cfg.ObserveMaxAgeDuration()
	taskAge := cfg.TaskMaxAgeDuration()
	tokenAge := cfg.TokenMaxAgeDuration()
	fmt.Printf("cleanup loop: every %s — state>%s, observe>%s, done-tasks>%s, token-events>%s\n", interval, stateAge, observeAge, taskAge, tokenAge)

	// Stagger the first run by a few seconds so daemon startup logs stay
	// readable. After that, tick on the configured interval.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if n, err := st.CleanupStaleState(stateAge); err != nil {
			fmt.Printf("cleanup: state error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("cleanup: pruned %d stale state entries\n", n)
		}

		if n, err := st.CleanupObserveEvents(observeAge); err != nil {
			fmt.Printf("cleanup: observe error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("cleanup: pruned %d stale observe events\n", n)
		}

		if n, err := st.PruneMessages(); err != nil {
			fmt.Printf("cleanup: message error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("cleanup: pruned %d expired messages\n", n)
		}

		if n, err := st.PruneCompletedTasks(taskAge); err != nil {
			fmt.Printf("cleanup: task error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("cleanup: pruned %d completed tasks\n", n)
		}

		if n, err := st.CleanupTokenEvents(tokenAge); err != nil {
			fmt.Printf("cleanup: token error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("cleanup: pruned %d token events\n", n)
		}

		timer.Reset(interval)
	}
}

// cmdDaemon starts the gRPC daemon.
func cmdDaemon(dbPath string, args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printDaemonUsage()
		return nil
	}

	// Parse socket path from args
	socketPath := grpcapi.SocketPathFromDB(dbPath)
	for i, arg := range args {
		if arg == "--socket" && i+1 < len(args) {
			socketPath = args[i+1]
		}
	}

	// Check if daemon is already running
	if grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if err == nil {
			client.Close()
			return fmt.Errorf("daemon already running at %s", socketPath)
		}
		// Socket exists but not responding, remove it
		os.Remove(socketPath)
	}

	// Registered first so it runs last — after every store Close below has run,
	// leaving the bolt files unlocked for compaction. No-op unless enabled.
	defer compactStoresOnExit(dbPath)

	// Open the store
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer st.Close()

	// Open code store for code indexing
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		fmt.Printf("WARNING: failed to open code store: %v (code tools disabled)\n", err)
	} else {
		defer codeStore.Close()
	}

	// Open findings store for static analysis
	findingsDir := getFindingsStorePath(dbPath)
	findingsStore, err := store.NewFindingsStore(findingsDir)
	if err != nil {
		fmt.Printf("WARNING: failed to open findings store: %v (findings tools disabled)\n", err)
	} else {
		defer findingsStore.Close()
	}

	// Open survey store for codebase survey
	surveyDir := getSurveyStorePath(dbPath)
	surveyStore, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		fmt.Printf("WARNING: failed to open survey store: %v (survey tools disabled)\n", err)
	} else {
		defer surveyStore.Close()
	}

	// Create gRPC server
	server := grpcapi.NewServer(st, dbPath, socketPath, newGrammarLoader(dbPath, nil))

	// Set code store if available
	if codeStore != nil {
		server.SetCodeStore(codeStore)
	}

	// Set findings store if available
	if findingsStore != nil {
		server.SetFindingsStore(findingsStore)
	}

	// Set survey store if available
	if surveyStore != nil {
		server.SetSurveyStore(surveyStore)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
		server.Stop()
	}()

	// Background bucket-prune loop. Survives the daemon's lifetime;
	// stopped via ctx on shutdown.
	go runCleanupLoop(ctx, st)

	// Register instance for discovery by aide-web
	projRoot := projectRoot(dbPath)
	if err := registry.Register(projRoot, socketPath, dbPath); err != nil {
		fmt.Printf("WARNING: failed to register instance: %v\n", err)
	} else {
		defer func() {
			if err := registry.Unregister(projRoot); err != nil {
				fmt.Printf("WARNING: failed to unregister instance: %v\n", err)
			}
		}()
	}

	fmt.Printf("aide daemon starting on %s\n", socketPath)
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Println("Press Ctrl+C to stop")

	// Start serving
	return server.Start()
}

func printDaemonUsage() {
	fmt.Println(`aide daemon - Start gRPC daemon for IPC

Usage:
  aide daemon [options]

Options:
  --socket PATH    Unix socket path (default: auto-detected)

The daemon provides a persistent gRPC server that multiple CLI invocations
can connect to, avoiding repeated database open/close overhead.

Examples:
  aide daemon
  aide daemon --socket /tmp/aide.sock`)
}
