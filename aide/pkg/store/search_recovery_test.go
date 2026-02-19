package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchIndexRecoveryFromCorruptedIndex(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-search-recovery-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	searchPath := filepath.Join(tmpDir, "search.bleve")

	// Create a valid index first, then close it
	store, err := NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		t.Fatalf("failed to create initial search store: %v", err)
	}
	store.Close()

	// Corrupt the index by truncating the meta file
	metaPath := filepath.Join(searchPath, "index_meta.json")
	if err := os.WriteFile(metaPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to corrupt index: %v", err)
	}

	// Opening should auto-recover (not error)
	store, err = NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		t.Fatalf("expected auto-recovery, got error: %v", err)
	}
	defer store.Close()

	// Verify the recovered index works
	count, err := store.index.DocCount()
	if err != nil {
		t.Fatalf("DocCount on recovered index: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 docs in recovered index, got %d", count)
	}
}

func TestSearchIndexRecoveryFromMissingSegment(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-search-recovery-seg-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	searchPath := filepath.Join(tmpDir, "search.bleve")

	// Create a valid index
	store, err := NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		t.Fatalf("failed to create initial search store: %v", err)
	}
	store.Close()

	// Corrupt by removing the store directory contents
	storePath := filepath.Join(searchPath, "store")
	os.RemoveAll(storePath)
	os.MkdirAll(storePath, 0755)

	// Opening should auto-recover
	store, err = NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		t.Fatalf("expected auto-recovery, got error: %v", err)
	}
	defer store.Close()
}

func TestSearchIndexCreatesNewWhenNoneExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-search-new-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	searchPath := filepath.Join(tmpDir, "search.bleve")

	// No index exists — should create fresh
	store, err := NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		t.Fatalf("failed to create new search store: %v", err)
	}
	defer store.Close()

	// Directory should now exist
	if _, err := os.Stat(searchPath); os.IsNotExist(err) {
		t.Error("expected search index directory to be created")
	}
}

func TestSearchIndexInMemory(t *testing.T) {
	// Empty path = in-memory index
	store, err := NewSearchStore(SearchConfig{Path: ""})
	if err != nil {
		t.Fatalf("failed to create in-memory search store: %v", err)
	}
	defer store.Close()

	count, err := store.index.DocCount()
	if err != nil {
		t.Fatalf("DocCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}
