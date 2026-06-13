package store

import (
	"fmt"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CompactResult reports the outcome of compacting a single bolt file.
type CompactResult struct {
	Path      string // the file acted on
	Before    int64  // file size before, bytes
	After     int64  // file size after, bytes (== Before when skipped)
	Reclaimed int64  // Before - After (0 when skipped)
	Compacted bool   // false when the file was absent or below threshold
}

// Compaction thresholds. bbolt never returns freed pages to the OS, so a file
// only shrinks when rewritten. We only bother when the free pages are both a
// meaningful fraction of the file AND above an absolute floor — so tiny files
// and already-tight files are left untouched.
const (
	compactMinFreeBytes    = 1 << 20 // 1 MiB of free pages
	compactMinFreeFraction = 0.20    // ...and at least 20% of the file
	// compactTxBatchBytes bounds the size of each copy transaction so a large
	// store doesn't buffer the whole dataset in one write txn.
	compactTxBatchBytes = 64 << 20
)

// CompactClosedDB rewrites the bolt database at path into a defragmented copy
// and atomically replaces the original, but only when its free space crosses
// the thresholds above. The file MUST NOT be open elsewhere — call it after the
// owning store's Close. A missing file is a no-op (returns Compacted=false).
//
// On any failure the original file is left untouched and the temp copy removed.
func CompactClosedDB(path string) (CompactResult, error) {
	res := CompactResult{Path: path}
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.Before, res.After = fi.Size(), fi.Size()

	// Open read-write with a short timeout: if anyone still holds the lock we
	// bail rather than block the process's exit. PreLoadFreelist makes
	// FreePageN accurate at open without needing a transaction.
	src, err := bolt.Open(path, 0o600, &bolt.Options{
		Timeout:         3 * time.Second,
		FreelistType:    bolt.FreelistMapType,
		PreLoadFreelist: true,
	})
	if err != nil {
		return res, fmt.Errorf("open %s: %w", path, err)
	}

	// Decide from the freelist whether it's worth the rewrite. FreeAlloc is
	// only populated during a commit, so derive free bytes from the page count
	// (set at open) and the page size instead.
	free := int64(src.Stats().FreePageN) * int64(src.Info().PageSize)
	if free < compactMinFreeBytes || float64(free) < compactMinFreeFraction*float64(res.Before) {
		_ = src.Close()
		return res, nil
	}

	tmp := path + ".compact"
	_ = os.Remove(tmp)
	dst, err := bolt.Open(tmp, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		_ = src.Close()
		return res, fmt.Errorf("open temp %s: %w", tmp, err)
	}

	if err := bolt.Compact(dst, src, compactTxBatchBytes); err != nil {
		_ = dst.Close()
		_ = src.Close()
		_ = os.Remove(tmp)
		return res, fmt.Errorf("compact %s: %w", path, err)
	}

	// Close both handles before the swap so the rename is safe on every OS.
	if err := dst.Close(); err != nil {
		_ = src.Close()
		_ = os.Remove(tmp)
		return res, fmt.Errorf("close temp %s: %w", tmp, err)
	}
	if err := src.Close(); err != nil {
		_ = os.Remove(tmp)
		return res, fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return res, fmt.Errorf("replace %s: %w", path, err)
	}

	if nfi, err := os.Stat(path); err == nil {
		res.After = nfi.Size()
	}
	res.Reclaimed = res.Before - res.After
	res.Compacted = true
	return res, nil
}
