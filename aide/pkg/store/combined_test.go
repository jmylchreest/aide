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
	memories, err := cs.SearchMemories("survive", 10)
	if err != nil {
		cleanup()
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(memories) == 0 {
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
	memories, err = cs2.SearchMemories("survive", 10)
	if err != nil {
		t.Fatalf("SearchMemories after rebuild: %v", err)
	}
	if len(memories) == 0 {
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

func TestCombinedStoreMemoryOperations(t *testing.T) {
	cs, _, cleanup := setupTestCombinedStore(t)
	defer cleanup()

	m := &memory.Memory{
		ID:        "combined-1",
		Category:  memory.CategoryLearning,
		Content:   "Combined store test memory",
		Tags:      []string{"test"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	t.Run("AddAndGet", func(t *testing.T) {
		if err := cs.AddMemory(m); err != nil {
			t.Fatalf("AddMemory: %v", err)
		}
		got, err := cs.GetMemory("combined-1")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if got.Content != "Combined store test memory" {
			t.Errorf("content = %q", got.Content)
		}
	})

	t.Run("ListMemories", func(t *testing.T) {
		memories, err := cs.ListMemories(memory.SearchOptions{})
		if err != nil {
			t.Fatalf("ListMemories: %v", err)
		}
		if len(memories) < 1 {
			t.Error("expected at least 1 memory")
		}
	})

	t.Run("SearchMemories", func(t *testing.T) {
		memories, err := cs.SearchMemories("combined store", 10)
		if err != nil {
			t.Fatalf("SearchMemories: %v", err)
		}
		if len(memories) < 1 {
			t.Error("expected at least 1 search result")
		}
	})

	t.Run("SearchMultiWordOR", func(t *testing.T) {
		// Multi-word queries should use OR: "combined nonexistent" should still
		// match memories containing "combined" even though "nonexistent" is absent.
		memories, err := cs.SearchMemories("combined nonexistentword", 10)
		if err != nil {
			t.Fatalf("SearchMemories multi-word: %v", err)
		}
		if len(memories) < 1 {
			t.Error("expected at least 1 result for OR multi-word query, got 0")
		}
	})

	t.Run("SearchMemoriesWithScore", func(t *testing.T) {
		results, err := cs.SearchMemoriesWithScore("combined store", 10, nil)
		if err != nil {
			t.Fatalf("SearchMemoriesWithScore: %v", err)
		}
		if len(results) < 1 {
			t.Error("expected at least 1 scored result")
		}
		if results[0].Score <= 0 {
			t.Error("expected positive score from bleve search")
		}
		if results[0].Memory == nil {
			t.Error("expected Memory to be enriched")
		}
	})

	t.Run("SearchCount", func(t *testing.T) {
		count, err := cs.SearchCount()
		if err != nil {
			t.Fatalf("SearchCount: %v", err)
		}
		if count < 1 {
			t.Errorf("expected count >= 1, got %d", count)
		}
	})

	t.Run("DeleteMemory", func(t *testing.T) {
		if err := cs.DeleteMemory("combined-1"); err != nil {
			t.Fatalf("DeleteMemory: %v", err)
		}
		_, err := cs.GetMemory("combined-1")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("ClearMemories", func(t *testing.T) {
		cs.AddMemory(&memory.Memory{
			ID:        "clear-1",
			Category:  memory.CategoryLearning,
			Content:   "To clear",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		count, err := cs.ClearMemories()
		if err != nil {
			t.Fatalf("ClearMemories: %v", err)
		}
		if count < 1 {
			t.Errorf("expected at least 1 cleared, got %d", count)
		}
	})
}

func TestCombinedStoreTouchMemory(t *testing.T) {
	cs, _, cleanup := setupTestCombinedStore(t)
	defer cleanup()

	// Seed a memory.
	m := &memory.Memory{
		ID:        "ctouch-1",
		Category:  memory.CategoryLearning,
		Content:   "Combined store touch test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := cs.AddMemory(m); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	t.Run("TouchThroughCombinedStore", func(t *testing.T) {
		n, err := cs.TouchMemory([]string{"ctouch-1"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 1 {
			t.Errorf("expected 1 touched, got %d", n)
		}

		got, err := cs.GetMemory("ctouch-1")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if got.AccessCount != 1 {
			t.Errorf("expected AccessCount=1, got %d", got.AccessCount)
		}
		if got.LastAccessed.IsZero() {
			t.Error("expected LastAccessed to be set")
		}
	})

	t.Run("NonexistentSkipped", func(t *testing.T) {
		n, err := cs.TouchMemory([]string{"nonexistent"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 touched, got %d", n)
		}
	})
}
