package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// startDaemonForTest stands up a real gRPC server on the socket NewBackend
// derives from dbPath, so a Backend constructed afterwards routes through gRPC
// (daemon mode). Returns the dbPath the test should hand to NewBackend.
func startDaemonForTest(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	memDir := filepath.Join(root, ".aide", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(memDir, "memory.db")
	socketPath := grpcapi.SocketPathFromDB(dbPath)

	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := grpcapi.NewServer(st, dbPath, socketPath, grammar.NewCompositeLoader())
	go func() { _ = srv.Start() }()
	t.Cleanup(srv.Stop)

	// Wait for the socket to appear so SocketExistsForDB sees it.
	deadline := time.Now().Add(2 * time.Second)
	for !grpcapi.SocketExistsForDB(dbPath) {
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not appear")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return dbPath
}

// TestBackendTombstoneStore_DaemonMode asserts that in daemon mode (gRPC) the
// Backend exposes a non-nil TombstoneStore, and that it round-trips through
// gRPC. This is the regression guard for the bug where TombstoneStore() was nil
// mid-session, silently dropping tombstone propagation during share export.
func TestBackendTombstoneStore_DaemonMode(t *testing.T) {
	dbPath := startDaemonForTest(t)

	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	t.Cleanup(func() { b.Close() })

	if !b.UsingGRPC() {
		t.Fatal("expected Backend to use gRPC (daemon mode)")
	}

	ts := b.TombstoneStore()
	if ts == nil {
		t.Fatal("TombstoneStore() = nil in daemon mode, want non-nil gRPC adapter")
	}

	deletedAt := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	tomb := &memory.Tombstone{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: deletedAt,
	}
	if err := ts.AddTombstone(tomb); err != nil {
		t.Fatalf("AddTombstone over gRPC: %v", err)
	}

	got, err := ts.GetTombstone(tomb.Kind, tomb.ID)
	if err != nil {
		t.Fatalf("GetTombstone over gRPC: %v", err)
	}
	if !got.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt = %v, want %v", got.DeletedAt, deletedAt)
	}

	list, err := ts.ListTombstones()
	if err != nil {
		t.Fatalf("ListTombstones over gRPC: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListTombstones len = %d, want 1", len(list))
	}
}

// TestBackendExportMaterialisesTombstone_DaemonMode is the end-to-end proof for
// the original bug: with the daemon holding the DB, a deletion recorded as a
// tombstone must still materialise a tombstones/*.md file when share export
// routes through gRPC. Before the TombstoneService existed, TombstoneStore() was
// nil here and the file was silently dropped.
func TestBackendExportMaterialisesTombstone_DaemonMode(t *testing.T) {
	dbPath := startDaemonForTest(t)

	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	t.Cleanup(func() { b.Close() })

	if !b.UsingGRPC() {
		t.Fatal("expected Backend to use gRPC (daemon mode)")
	}

	tombs := b.TombstoneStore()
	if tombs == nil {
		t.Fatal("TombstoneStore() = nil in daemon mode")
	}

	tomb := &memory.Tombstone{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: time.Now(),
	}
	if err := tombs.AddTombstone(tomb); err != nil {
		t.Fatalf("AddTombstone over gRPC: %v", err)
	}

	outputDir := t.TempDir()
	stats, err := contextshare.Export(b.Store(), tombs, outputDir, contextshare.ExportOptions{
		Decisions: true,
		Memories:  true,
	})
	if err != nil {
		t.Fatalf("Export through daemon-routed store: %v", err)
	}
	if stats.Tombstones != 1 {
		t.Errorf("stats.Tombstones = %d, want 1", stats.Tombstones)
	}

	entries, err := os.ReadDir(filepath.Join(outputDir, "tombstones"))
	if err != nil {
		t.Fatalf("read tombstones dir: %v", err)
	}
	var mdCount int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			mdCount++
		}
	}
	if mdCount != 1 {
		t.Errorf("tombstone .md files = %d, want 1", mdCount)
	}
}
