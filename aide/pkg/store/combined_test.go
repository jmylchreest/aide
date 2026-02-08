package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func setupTestCombinedStore(t *testing.T) (*CombinedStore, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-combined-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	cs, err := NewCombinedStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create combined store: %v", err)
	}

	cleanup := func() {
		cs.Close()
		os.RemoveAll(tmpDir)
	}

	return cs, tmpDir, cleanup
}

func TestCombinedStoreCreation(t *testing.T) {
	cs, _, cleanup := setupTestCombinedStore(t)
	defer cleanup()

	// Schema version should be stamped.
	v, err := GetSchemaVersion(cs.bolt.db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("expected schema version %d, got %d", SchemaVersion, v)
	}

	// Mapping hash should be stored.
	hash, err := cs.bolt.GetMeta("search_mapping_hash")
	if err != nil {
		t.Fatalf("GetMeta(search_mapping_hash): %v", err)
	}
	if hash == "" {
		t.Error("expected search_mapping_hash to be set")
	}
}

func TestCombinedStoreSearchRebuild(t *testing.T) {
	cs, tmpDir, cleanup := setupTestCombinedStore(t)

	// Add a memory so we can verify it survives a rebuild.
	m := &memory.Memory{
		ID:        "rebuild-test",
		Category:  memory.CategoryLearning,
		Content:   "This should survive search rebuild",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := cs.AddMemory(m); err != nil {
		cleanup()
		t.Fatalf("AddMemory: %v", err)
	}

	// Verify it's searchable.
	results, err := cs.SearchMemories("survive", 10)
	if err != nil {
		cleanup()
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) == 0 {
		cleanup()
		t.Fatal("expected search results before rebuild")
	}

	// Close the store.
	cs.Close()

	// Corrupt the stored hash to force a rebuild on next open.
	dbPath := filepath.Join(tmpDir, "test.db")
	bolt2, err := NewBoltStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to reopen bolt: %v", err)
	}
	if err := bolt2.SetMeta("search_mapping_hash", "stale-hash-value"); err != nil {
		bolt2.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("SetMeta: %v", err)
	}
	bolt2.Close()

	// Reopen CombinedStore — ensureSearchMapping should detect mismatch and rebuild.
	cs2, err := NewCombinedStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to reopen combined store: %v", err)
	}
	defer func() {
		cs2.Close()
		os.RemoveAll(tmpDir)
	}()

	// Verify the hash was updated.
	newHash, err := cs2.bolt.GetMeta("search_mapping_hash")
	if err != nil {
		t.Fatalf("GetMeta after rebuild: %v", err)
	}
	if newHash == "stale-hash-value" {
		t.Error("expected mapping hash to be updated after rebuild")
	}

	// Memory should still be in bolt (source of truth).
	got, err := cs2.GetMemory("rebuild-test")
	if err != nil {
		t.Fatalf("GetMemory after rebuild: %v", err)
	}
	if got.Content != "This should survive search rebuild" {
		t.Errorf("content mismatch: %q", got.Content)
	}

	// Search should work after rebuild (index was re-synced from bolt).
	results, err = cs2.SearchMemories("survive", 10)
	if err != nil {
		t.Fatalf("SearchMemories after rebuild: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results after rebuild")
	}
}

func TestCombinedStoreSearchNoRebuildWhenCurrent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-combined-norebuild-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create and close first store.
	cs1, err := NewCombinedStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	cs1.Close()

	// Reopen — should not rebuild since hash matches.
	cs2, err := NewCombinedStore(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen: %v", err)
	}
	defer cs2.Close()

	// If we got here without error, ensureSearchMapping was a no-op (hash matched).
	hash, err := cs2.bolt.GetMeta("search_mapping_hash")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}

	m, mErr := buildIndexMapping()
	if mErr != nil {
		t.Fatalf("buildIndexMapping: %v", mErr)
	}
	expected := MappingHash(m)
	if hash != expected {
		t.Errorf("hash mismatch: got %q, want %q", hash, expected)
	}
}
