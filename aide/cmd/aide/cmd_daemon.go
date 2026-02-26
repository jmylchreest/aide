// Package main provides the daemon command for aide.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

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

	// Create gRPC server
	server := grpcapi.NewServer(st, dbPath, socketPath)

	// Set code store if available
	if codeStore != nil {
		server.SetCodeStore(codeStore)
	}

	// Set findings store if available
	if findingsStore != nil {
		server.SetFindingsStore(findingsStore)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		server.Stop()
	}()

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
