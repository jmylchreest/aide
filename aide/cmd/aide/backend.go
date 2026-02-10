// Package main provides backend abstraction for CLI commands.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// Backend provides an interface for CLI commands to access aide,
// either via gRPC (when socket exists) or direct DB access.
type Backend struct {
	grpcClient *grpcapi.Client
	store      store.Store
	combined   *store.CombinedStore // non-nil when using direct DB (for scored search)
	dbPath     string
	useGRPC    bool
}

// NewBackend creates a new backend, preferring gRPC if available.
// If AIDE_MEMORY_DB is set, always use direct DB access (for testing isolation).
// When using direct DB, it opens a CombinedStore (bolt + bleve) so that
// memory search uses full-text search instead of substring matching.
func NewBackend(dbPath string) (*Backend, error) {
	b := &Backend{dbPath: dbPath}

	// Skip gRPC when AIDE_MEMORY_DB is set (testing isolation)
	useDirectDB := os.Getenv("AIDE_MEMORY_DB") != ""

	// Try gRPC first (unless testing with custom DB path)
	if !useDirectDB && grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if err == nil {
			// Verify connection works
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := client.Ping(ctx); err == nil {
				b.grpcClient = client
				b.useGRPC = true
				return b, nil
			}
			client.Close()
		}
	}

	// Fall back to direct DB with CombinedStore (bolt + bleve search)
	cs, err := store.NewCombinedStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	b.store = cs    // CombinedStore implements Store
	b.combined = cs // Keep typed reference for scored search
	b.useGRPC = false
	return b, nil
}

// Close closes the backend.
func (b *Backend) Close() error {
	if b.grpcClient != nil {
		return b.grpcClient.Close()
	}
	if b.store != nil {
		return b.store.Close()
	}
	return nil
}

// UsingGRPC returns true if using gRPC backend.
func (b *Backend) UsingGRPC() bool {
	return b.useGRPC
}

// openCodeStore opens the code store for direct access.
func (b *Backend) openCodeStore() (store.CodeIndexStore, error) {
	indexPath, searchPath := getCodeStorePaths(b.dbPath)
	return store.NewCodeStore(indexPath, searchPath)
}
