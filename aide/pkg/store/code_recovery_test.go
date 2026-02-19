package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeSearchIndexRecoveryFromCorruption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-code-recovery-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")

	// Create a valid code store, then close it
	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("failed to create initial code store: %v", err)
	}
	cs.Close()

	// Corrupt the search index
	metaPath := filepath.Join(searchPath, "index_meta.json")
	if err := os.WriteFile(metaPath, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to corrupt index: %v", err)
	}

	// Opening should auto-recover
	cs, err = NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("expected auto-recovery, got error: %v", err)
	}
	defer cs.Close()
}

func TestCodeSearchIndexCreatesNewWhenNoneExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-code-new-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")

	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("failed to create new code store: %v", err)
	}
	defer cs.Close()

	if _, err := os.Stat(searchPath); os.IsNotExist(err) {
		t.Error("expected search index directory to be created")
	}
}
