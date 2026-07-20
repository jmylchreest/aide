// Package main provides backend abstraction for CLI commands.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/memory"
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
// When using direct DB, it opens a CombinedStore (bolt + bleve) so that
// memory search uses full-text search instead of substring matching.
func NewBackend(dbPath string) (*Backend, error) {
	b := &Backend{dbPath: dbPath}

	// Try gRPC first — if the MCP server is running, route through it
	// to avoid BoltDB file-lock contention.
	if grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if errors.Is(err, grpcapi.ErrSandboxDenied) {
			// A daemon socket is present but this process's sandbox blocks
			// connect(2). The daemon behind it likely holds the store locks,
			// so falling through to a direct open would stall for the BoltDB
			// lock timeout and then fail — fatal for hooks on a ~3s budget.
			return nil, fmt.Errorf(
				"aide daemon socket %s is unreachable from this sandboxed shell: %w (rerun outside the sandbox, or allow it in Codex with [sandbox_workspace_write] network_access = true)",
				grpcapi.SocketPathFromDB(dbPath), err)
		}
		if err == nil {
			// Verify connection works
			ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
			defer cancel()
			if err := client.Ping(ctx); err == nil {
				b.grpcClient = client
				// Backend.Store() must work in gRPC mode too: wrap the
				// client in a StoreAdapter so callers can use the unified
				// Store interface without caring about transport.
				b.store = adapter.NewStoreAdapter(client)
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

// Store returns the underlying store (gRPC adapter or direct BoltStore).
func (b *Backend) Store() store.Store {
	return b.store
}

// InstinctStore returns the proposal store, routed via gRPC when the daemon
// is up, or via direct BoltStore otherwise. Separate from Store() because
// InstinctProposalStore isn't on the Store interface (see interfaces.go).
func (b *Backend) InstinctStore() store.InstinctProposalStore {
	if b.useGRPC && b.grpcClient != nil {
		return adapter.NewInstinctAdapter(b.grpcClient)
	}
	if b.combined != nil {
		return b.combined
	}
	return nil
}

// TombstoneStore returns the tombstone surface, routed via gRPC when the daemon
// is up (so share export/import can list and record tombstones mid-session), or
// via the direct CombinedStore otherwise. Returns nil only when neither a gRPC
// client nor a direct DB handle is available.
func (b *Backend) TombstoneStore() store.TombstoneStore {
	if b.useGRPC && b.grpcClient != nil {
		return adapter.NewTombstoneAdapter(b.grpcClient)
	}
	if b.combined != nil {
		return b.combined
	}
	return nil
}

// rpcCtx returns a context with a 10-second deadline for gRPC calls.
// This matches the timeout used by grpcStoreAdapter.rpcCtx.
func (b *Backend) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), adapter.RPCTimeout)
}

// lastRetentionSweepKey records when the direct-mode retention sweep last ran,
// rate-limiting it so rapid session starts (subagent swarms) don't rescan the
// buckets on every init.
const lastRetentionSweepKey = "lastRetentionSweep"

// retentionSweepMinInterval is the minimum gap between direct-mode sweeps.
const retentionSweepMinInterval = time.Hour

// RetentionSweep runs one retention pass directly against the store.
// In gRPC mode it is a no-op: the daemon behind the socket runs the cleanup
// loop on its own interval. Returns per-bucket pruned counts (empty when
// skipped); errors are best-effort and never fail the caller.
func (b *Backend) RetentionSweep() map[string]int {
	if b.useGRPC || b.store == nil {
		return nil
	}
	cfg := config.Get().Cleanup
	if !cfg.Enabled {
		return nil
	}

	if st, err := b.store.GetState(lastRetentionSweepKey); err == nil && st != nil {
		if last, perr := time.Parse(time.RFC3339, st.Value); perr == nil {
			if time.Since(last) < retentionSweepMinInterval {
				return nil
			}
		}
	}

	counts, _ := retentionSweepOnce(b.store, cfg)
	_ = b.store.SetState(&memory.State{
		Key:   lastRetentionSweepKey,
		Value: time.Now().UTC().Format(time.RFC3339),
	})
	return counts
}

// openCodeStore opens the code store for direct access.
func (b *Backend) openCodeStore() (store.CodeIndexStore, error) {
	indexPath, searchPath := getCodeStorePaths(b.dbPath)
	return store.NewCodeStore(indexPath, searchPath)
}
