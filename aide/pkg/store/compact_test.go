package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// TestCompactClosedDB_ReclaimsFreePages writes a large bucket, deletes it
// (leaving the file full of free pages bbolt won't return to the OS), and
// verifies compaction shrinks the file while preserving surviving data.
func TestCompactClosedDB_ReclaimsFreePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	blob := make([]byte, 4096)
	if err := db.Update(func(tx *bolt.Tx) error {
		junk, _ := tx.CreateBucketIfNotExists([]byte("junk"))
		for i := 0; i < 4000; i++ {
			if err := junk.Put(itob(uint64(i)), blob); err != nil {
				return err
			}
		}
		keep, _ := tx.CreateBucketIfNotExists([]byte("keep"))
		return keep.Put([]byte("survivor"), []byte("value"))
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Delete the junk bucket: frees pages but leaves the file size unchanged.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte("junk"))
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	before := fileSize(t, path)

	res, err := CompactClosedDB(path)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if !res.Compacted {
		t.Fatalf("expected compaction to run; free pages should exceed threshold (before=%d)", before)
	}
	after := fileSize(t, path)
	if after >= before {
		t.Fatalf("expected file to shrink: before=%d after=%d", before, after)
	}
	if res.Before != before || res.After != after {
		t.Fatalf("result sizes mismatch: res=%+v actual before=%d after=%d", res, before, after)
	}
	if res.Reclaimed != before-after {
		t.Fatalf("reclaimed mismatch: got %d want %d", res.Reclaimed, before-after)
	}

	// Surviving data must still be readable after the swap.
	db2, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	if err := db2.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keep"))
		if b == nil {
			t.Fatalf("keep bucket missing after compaction")
		}
		if got := string(b.Get([]byte("survivor"))); got != "value" {
			t.Fatalf("survivor value = %q, want %q", got, "value")
		}
		return nil
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestCompactClosedDB_SkipsTightFile verifies a small/tight database is left
// untouched (no rewrite, Compacted=false, no temp file left behind).
func TestCompactClosedDB_SkipsTightFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tight.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte("b"))
		return b.Put([]byte("k"), []byte("v"))
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	res, err := CompactClosedDB(path)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Compacted {
		t.Fatalf("expected tight file to be skipped, got %+v", res)
	}
	if _, err := os.Stat(path + ".compact"); !os.IsNotExist(err) {
		t.Fatalf("temp compaction file should not remain")
	}
}

// TestCompactClosedDB_MissingFile is a no-op, not an error.
func TestCompactClosedDB_MissingFile(t *testing.T) {
	res, err := CompactClosedDB(filepath.Join(t.TempDir(), "nope.db"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if res.Compacted {
		t.Fatalf("missing file should not report Compacted")
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}
