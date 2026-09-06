package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// electionRoot lays out a project the way SocketPathFromDB and
// ProjectRootFromDB expect (<root>/.aide/memory/memory.db) and sandboxes the
// instance registry, which otherwise writes under the real home directory.
func electionRoot(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".aide", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, ".aide", "memory", "memory.db")
}

// join, then register the teardown so a failing test doesn't leave the bolt
// lock held for the rest of the package.
func mustJoin(t *testing.T, dbPath string) (*MCPServer, func()) {
	t.Helper()
	s := newMCPServer(nil)
	s.grammarLoader = newGrammarLoader(dbPath, mcpLog)
	s.dbPath = dbPath

	teardown, err := s.join(dbPath, &mcpConfig{})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	return s, teardown
}

func TestJoinBecomesPrimaryWhenNoneRunning(t *testing.T) {
	dbPath := electionRoot(t)

	s, teardown := mustJoin(t, dbPath)
	defer teardown()

	if s.grpcClient() != nil {
		t.Error("joined as client with no primary running, want primary")
	}
	if s.store() == nil {
		t.Error("primary has no store")
	}
}

func TestJoinAttachesWhenPrimaryHoldsTheStore(t *testing.T) {
	dbPath := electionRoot(t)

	primary, stopPrimary := mustJoin(t, dbPath)
	defer stopPrimary()
	if primary.grpcClient() != nil {
		t.Fatal("first join did not become primary")
	}

	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()

	if client.grpcClient() == nil {
		t.Error("second join became primary too; the bolt lock should have refused it")
	}
	if client.store() == nil {
		t.Error("client has no store adapter")
	}
}

// The bolt lock is the election, so concurrency is the case that matters:
// whatever the interleaving, exactly one process may hold the store.
func TestElectionYieldsExactlyOnePrimary(t *testing.T) {
	dbPath := electionRoot(t)

	const contenders = 4
	var (
		wg        sync.WaitGroup
		primaries atomic.Int32
		clients   atomic.Int32
		mu        sync.Mutex
		teardowns []func()
	)

	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := newMCPServer(nil)
			s.grammarLoader = newGrammarLoader(dbPath, mcpLog)
			s.dbPath = dbPath

			teardown, err := s.join(dbPath, &mcpConfig{})
			if err != nil {
				t.Errorf("join: %v", err)
				return
			}
			mu.Lock()
			teardowns = append(teardowns, teardown)
			mu.Unlock()

			if s.grpcClient() == nil {
				primaries.Add(1)
			} else {
				clients.Add(1)
			}
		}()
	}
	wg.Wait()
	defer func() {
		for _, fn := range teardowns {
			fn()
		}
	}()

	if got := primaries.Load(); got != 1 {
		t.Errorf("primaries = %d, want exactly 1", got)
	}
	if got := clients.Load(); got != contenders-1 {
		t.Errorf("clients = %d, want %d", got, contenders-1)
	}
}

// The bug this guards: before failover, a client whose primary exited was
// stranded for the rest of its life.
func TestSupervisorPromotesAfterPrimaryExits(t *testing.T) {
	dbPath := electionRoot(t)

	_, stopPrimary := mustJoin(t, dbPath)
	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()

	if client.grpcClient() == nil {
		t.Fatal("second join did not attach as a client")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var swapped atomic.Value
	swapped.Store(func() {})
	go client.supervisePrimary(ctx, dbPath, &mcpConfig{}, func(next func()) {
		swapped.Store(next)
	})

	stopPrimary()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if client.grpcClient() == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer func() { swapped.Load().(func())() }()

	if client.grpcClient() != nil {
		t.Fatal("client was not promoted after the primary exited")
	}
	if client.store() == nil {
		t.Error("promoted client has no store")
	}
	// Run has long since returned its wiring pass, so the status service is
	// only populated if becomePrimary does it.
	if client.grpcSrv() == nil {
		t.Error("promoted client did not publish a gRPC server")
	}
}
