package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// TestIndexerReconcile_RemovesOrphans verifies that Reconcile drops file-index
// entries whose underlying file no longer exists on disk. This is the bulk of
// the staleness problem the reconciler is designed to fix.
func TestIndexerReconcile_RemovesOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	realFile := filepath.Join(tmpDir, "real.go")
	if err := os.WriteFile(realFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	realStat, err := os.Stat(realFile)
	if err != nil {
		t.Fatal(err)
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Real file: stored with current mtime — should be left alone.
	if err := cs.SetFileInfo(&code.FileInfo{Path: "real.go", ModTime: realStat.ModTime()}); err != nil {
		t.Fatal(err)
	}
	// Orphan: file does not exist on disk — should be removed.
	if err := cs.SetFileInfo(&code.FileInfo{Path: "ghost.go", ModTime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	idx := NewIndexerFromStore(cs, newGrammarLoader(dbPath, nil), tmpDir)

	res, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Removed != 1 {
		t.Errorf("expected 1 removed, got %d (%+v)", res.Removed, res)
	}
	if res.Refreshed != 0 {
		t.Errorf("expected 0 refreshed, got %d (%+v)", res.Refreshed, res)
	}

	if _, err := cs.GetFileInfo("ghost.go"); err == nil {
		t.Error("ghost.go should have been removed from the index")
	}
	if _, err := cs.GetFileInfo("real.go"); err != nil {
		t.Errorf("real.go should still be indexed: %v", err)
	}
}
