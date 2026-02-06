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
	// Parse socket path from args
	socketPath := grpcapi.DefaultSocketPath()
	for i, arg := range args {
		if arg == "--socket" && i+1 < len(args) {
			socketPath = args[i+1]
		}
	}

	// Check if daemon is already running
	if grpcapi.SocketExists() {
		client, err := grpcapi.NewClient()
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

	// Create gRPC server
	server := grpcapi.NewServer(st, dbPath, socketPath)

	// Set code store if available
	if codeStore != nil {
		server.SetCodeStore(codeStore)
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
