package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"time"
)

// The import stale guard compares a tree's deletion horizon against how far
// behind the reader is — a property of the store, not of the tree, so it has
// to be recorded here.
//
// One stamp per tree: "caught up" is not transferable, and importing a
// teammate's export says nothing about deletions the project tree GC'd. The
// path is hashed so separators and unicode cannot collide in the flat state
// keyspace.
//
// The key is deliberately global rather than agent-scoped, because
// CleanupStaleState only prunes agent-scoped entries. A stamp lost to a
// session sweep would read as "never imported" and silently reopen the
// resurrection window this exists to close.
func lastImportKey(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return "share:lastImport:" + hex.EncodeToString(sum[:8])
}

// lastImportAt reports when this store last imported the tree at root, or the
// zero time if it never has. A store that cannot answer is treated as never
// having imported: the guard then allows the merge, which is the right call
// for a fresh clone and no worse than the behaviour before it existed.
func lastImportAt(backend *Backend, root string) time.Time {
	st, err := backend.GetState(lastImportKey(root), "")
	if err != nil || st == nil {
		return time.Time{}
	}
	at, err := time.Parse(time.RFC3339Nano, st.Value)
	if err != nil {
		return time.Time{}
	}
	return at
}

func recordImportAt(backend *Backend, root string, at time.Time) error {
	return backend.SetState(lastImportKey(root), at.UTC().Format(time.RFC3339Nano), "")
}
