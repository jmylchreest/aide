package grpcapi_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// newTombstoneFixture starts an in-process gRPC server over a real Unix socket,
// backed by a temp BoltStore, and returns a connected client adapter
// implementing store.TombstoneStore. Mirrors the daemon-mode wiring: the server
// holds the single-writer DB, the client routes through gRPC.
func newTombstoneFixture(t *testing.T) store.TombstoneStore {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	socketPath := filepath.Join(dir, "aide.sock")

	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := grpcapi.NewServer(st, dbPath, socketPath, grammar.NewCompositeLoader())
	go func() {
		// Start blocks on Serve until Stop; ignore the ErrServerStopped that
		// surfaces on graceful shutdown.
		_ = srv.Start()
	}()
	t.Cleanup(srv.Stop)

	client := waitForClient(t, socketPath)
	t.Cleanup(func() { client.Close() })

	return adapter.NewTombstoneAdapter(client)
}

// waitForClient retries the socket connect until the goroutine-started server
// is listening (Start creates the socket asynchronously).
func waitForClient(t *testing.T, socketPath string) *grpcapi.Client {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		client, err := grpcapi.NewClientWithSocket(socketPath)
		if err == nil {
			return client
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not come up: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTombstoneService_RoundTrip exercises the full gRPC path
// (client adapter -> server handler -> BoltStore) for every operation,
// asserting fidelity including the DeletedAt timestamp.
func TestTombstoneService_RoundTrip(t *testing.T) {
	ts := newTombstoneFixture(t)

	deletedAt := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	mem := &memory.Tombstone{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: deletedAt,
	}
	dec := &memory.Tombstone{
		ID:        "auth-strategy",
		Kind:      memory.TombstoneKindDecisionTopic,
		DeletedAt: deletedAt.Add(time.Hour),
	}

	// Add both.
	if err := ts.AddTombstone(mem); err != nil {
		t.Fatalf("AddTombstone(mem): %v", err)
	}
	if err := ts.AddTombstone(dec); err != nil {
		t.Fatalf("AddTombstone(dec): %v", err)
	}

	// List returns both with full fidelity.
	list, err := ts.ListTombstones()
	if err != nil {
		t.Fatalf("ListTombstones: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListTombstones len = %d, want 2", len(list))
	}
	byKey := map[string]*memory.Tombstone{}
	for _, tomb := range list {
		byKey[tomb.Kind+":"+tomb.ID] = tomb
	}
	gotMem := byKey[mem.Kind+":"+mem.ID]
	if gotMem == nil {
		t.Fatal("memory tombstone missing from list")
	}
	if !gotMem.DeletedAt.Equal(deletedAt) {
		t.Errorf("memory DeletedAt = %v, want %v", gotMem.DeletedAt, deletedAt)
	}

	// Get found.
	got, err := ts.GetTombstone(mem.Kind, mem.ID)
	if err != nil {
		t.Fatalf("GetTombstone(found): %v", err)
	}
	if got.ID != mem.ID || got.Kind != mem.Kind {
		t.Errorf("GetTombstone = {%q,%q}, want {%q,%q}", got.ID, got.Kind, mem.ID, mem.Kind)
	}
	if !got.DeletedAt.Equal(deletedAt) {
		t.Errorf("GetTombstone DeletedAt = %v, want %v", got.DeletedAt, deletedAt)
	}

	// Get not-found returns store.ErrNotFound.
	if _, err := ts.GetTombstone(memory.TombstoneKindMemory, "does-not-exist"); err != store.ErrNotFound {
		t.Errorf("GetTombstone(missing) err = %v, want store.ErrNotFound", err)
	}

	// Delete one, then verify only the other remains.
	if err := ts.DeleteTombstone(mem.Kind, mem.ID); err != nil {
		t.Fatalf("DeleteTombstone: %v", err)
	}
	if _, err := ts.GetTombstone(mem.Kind, mem.ID); err != store.ErrNotFound {
		t.Errorf("after delete, GetTombstone err = %v, want store.ErrNotFound", err)
	}

	if err := ts.DeleteTombstone(dec.Kind, dec.ID); err != nil {
		t.Fatalf("DeleteTombstone(dec): %v", err)
	}
	list, err = ts.ListTombstones()
	if err != nil {
		t.Fatalf("ListTombstones(empty): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListTombstones after deletes len = %d, want 0", len(list))
	}
}

// TestTombstoneService_AddStampsDeletedAt verifies the server stamps DeletedAt
// when the caller supplies a zero value, and the adapter reflects it back.
func TestTombstoneService_AddStampsDeletedAt(t *testing.T) {
	ts := newTombstoneFixture(t)

	before := time.Now()
	tomb := &memory.Tombstone{
		ID:   "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		Kind: memory.TombstoneKindMemory,
		// DeletedAt deliberately zero.
	}
	if err := ts.AddTombstone(tomb); err != nil {
		t.Fatalf("AddTombstone: %v", err)
	}
	if tomb.DeletedAt.IsZero() {
		t.Fatal("AddTombstone did not stamp DeletedAt on the caller's tombstone")
	}
	if tomb.DeletedAt.Before(before) {
		t.Errorf("stamped DeletedAt %v is before call start %v", tomb.DeletedAt, before)
	}

	got, err := ts.GetTombstone(tomb.Kind, tomb.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if !got.DeletedAt.Equal(tomb.DeletedAt) {
		t.Errorf("persisted DeletedAt = %v, want %v", got.DeletedAt, tomb.DeletedAt)
	}
}
