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

// TestIndexerReconcile_BootstrapsEmptyIndex verifies that Reconcile populates
// a store with no entries at all by walking the working tree. Without this,
// a fresh store surfaces as "code index is empty" to code-index-dependent
// analyzers (deadcode, modules) even though the reconciler ran.
func TestIndexerReconcile_BootstrapsEmptyIndex(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	idx := NewIndexerFromStore(cs, newGrammarLoader(dbPath, nil), tmpDir)

	res, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Refreshed == 0 {
		t.Errorf("expected bootstrap to index files, got %+v", res)
	}

	stats, err := cs.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Symbols == 0 {
		t.Errorf("expected symbols after bootstrap, got %+v", stats)
	}
	if _, err := cs.GetFileInfo("main.go"); err != nil {
		t.Errorf("main.go should be indexed: %v", err)
	}
}

// TestIndexerReconcile_DiscoversUnindexedFiles verifies that Reconcile picks
// up a file that is on disk but has never been indexed, and reports it in
// Touched. The watcher can miss a file entirely — one created inside a
// directory before that directory had an inotify watch — and the per-entry
// reconcile loop cannot see it, because it only inspects paths that are
// already index entries. Without this walk such a file stays invisible until
// someone runs `aide code index` by hand.
func TestIndexerReconcile_DiscoversUnindexedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	known := filepath.Join(tmpDir, "known.go")
	if err := os.WriteFile(known, []byte("package main\n\nfunc Known() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	knownStat, err := os.Stat(known)
	if err != nil {
		t.Fatal(err)
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Already indexed and unchanged: must not be re-indexed or reported.
	if err := cs.SetFileInfo(&code.FileInfo{Path: "known.go", ModTime: knownStat.ModTime()}); err != nil {
		t.Fatal(err)
	}

	// Never indexed, in a subdirectory — the shape a missed watch produces.
	missedDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(missedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(missedDir, "missed.go"), []byte("package pkg\n\nfunc Missed() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewIndexerFromStore(cs, newGrammarLoader(dbPath, nil), tmpDir)

	res, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, err := cs.GetFileInfo(filepath.Join("pkg", "missed.go")); err != nil {
		t.Errorf("pkg/missed.go should have been discovered: %v", err)
	}
	if res.Refreshed != 1 {
		t.Errorf("expected 1 refreshed (the discovered file), got %d (%+v)", res.Refreshed, res)
	}
	if len(res.Touched) != 1 {
		t.Fatalf("expected 1 touched path, got %v", res.Touched)
	}
	if got := filepath.Base(res.Touched[0]); got != "missed.go" {
		t.Errorf("Touched = %v, want the discovered file", res.Touched)
	}
}

// TestIndexerReconcile_ReportsRefreshedInTouched verifies that a stale entry
// re-indexed by Reconcile is reported in Touched, so the caller can re-run the
// findings analysers over it. Healing the index without re-analysing leaves
// findings stale for exactly the files that changed.
func TestIndexerReconcile_ReportsRefreshedInTouched(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	stale := filepath.Join(tmpDir, "stale.go")
	if err := os.WriteFile(stale, []byte("package main\n\nfunc Stale() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Indexed mtime predates the file on disk, so the entry is stale.
	if err := cs.SetFileInfo(&code.FileInfo{Path: "stale.go", ModTime: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}

	idx := NewIndexerFromStore(cs, newGrammarLoader(dbPath, nil), tmpDir)

	res, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Refreshed != 1 {
		t.Errorf("expected 1 refreshed, got %d (%+v)", res.Refreshed, res)
	}
	if len(res.Touched) != 1 || filepath.Base(res.Touched[0]) != "stale.go" {
		t.Errorf("Touched = %v, want the refreshed file", res.Touched)
	}
}
