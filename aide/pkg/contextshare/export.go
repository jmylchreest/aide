package contextshare

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// Source is the read surface Export needs from the store.
type Source interface {
	ListDecisions() ([]*memory.Decision, error)
	ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error)
}

// TombstoneAccess mirrors store.TombstoneStore. A nil value means tombstones
// are unavailable (gRPC mode has no tombstone RPCs); export then skips
// materialising DB tombstones and import skips local tombstone bookkeeping,
// degrading to the pre-tombstone behaviour rather than failing.
type TombstoneAccess interface {
	AddTombstone(t *memory.Tombstone) error
	GetTombstone(kind, id string) (*memory.Tombstone, error)
	ListTombstones() ([]*memory.Tombstone, error)
	DeleteTombstone(kind, id string) error
}

// ExportOptions configures Export. The zero value exports everything with
// the default TTL at the current time.
type ExportOptions struct {
	Now           time.Time     // Watermark and TTL reference; zero = time.Now()
	TombstoneTTL  time.Duration // Zero = DefaultTombstoneTTL
	SkipDecisions bool
	SkipMemories  bool
}

// ExportStats reports what an export wrote.
type ExportStats struct {
	Decisions  int // Decision version files present after export
	Memories   int // Memory files present after export
	Tombstones int // Live tombstone files present after export
}

// Export projects the shareable subset of src into the context tree at root.
//
// Record files are write-once: an existing decision version file is never
// rewritten, and a memory file is only rewritten when its owner's record
// changed. Nothing is deleted except records shadowed by a tombstone and
// tombstones past their TTL. The manifest watermark is the only byte that
// changes across re-exports of unchanged content.
func Export(src Source, tombs TombstoneAccess, root string, opts ExportOptions) (*ExportStats, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	ttl := opts.TombstoneTTL
	if ttl <= 0 {
		ttl = DefaultTombstoneTTL
	}

	for _, dir := range []string{decisionsDir, memoriesDir, tombstonesDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	stats := &ExportStats{}

	live, err := collectLiveTombstones(tombs, root, now, ttl)
	if err != nil {
		return nil, err
	}

	// Materialise live tombstones and remove the record files they shadow.
	for _, t := range live {
		path := TombstonePath(root, t)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(path, MarshalTombstone(t), 0o644); err != nil {
				return nil, fmt.Errorf("failed to write tombstone %s: %w", t.ID, err)
			}
		}
		if err := removeShadowedRecord(root, t); err != nil {
			return nil, err
		}
		stats.Tombstones++
	}

	if !opts.SkipDecisions {
		n, err := exportDecisions(src, root, live)
		if err != nil {
			return nil, err
		}
		stats.Decisions = n
	}

	if !opts.SkipMemories {
		n, err := exportMemories(src, root, live)
		if err != nil {
			return nil, err
		}
		stats.Memories = n
	}

	if err := WriteManifest(root, now); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}
	return stats, nil
}

// collectLiveTombstones merges DB tombstones with tombstone files already in
// the tree (newest DeletedAt wins per id), garbage-collecting both expired DB
// rows and expired files along the way.
func collectLiveTombstones(tombs TombstoneAccess, root string, now time.Time, ttl time.Duration) (map[string]*memory.Tombstone, error) {
	live := make(map[string]*memory.Tombstone)
	keep := func(t *memory.Tombstone) {
		key := t.Kind + ":" + t.ID
		if existing, ok := live[key]; !ok || t.DeletedAt.After(existing.DeletedAt) {
			live[key] = t
		}
	}

	if tombs != nil {
		dbTombstones, err := tombs.ListTombstones()
		if err != nil {
			return nil, fmt.Errorf("failed to list tombstones: %w", err)
		}
		for _, t := range dbTombstones {
			if now.Sub(t.DeletedAt) > ttl {
				if err := tombs.DeleteTombstone(t.Kind, t.ID); err != nil {
					return nil, fmt.Errorf("failed to GC tombstone %s: %w", t.ID, err)
				}
				continue
			}
			keep(t)
		}
	}

	dir := filepath.Join(root, tombstonesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		t, err := ParseTombstone(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping tombstone %s: %v\n", entry.Name(), err)
			continue
		}
		if now.Sub(t.DeletedAt) > ttl {
			// Past TTL: GC the tombstone file and the record it shadowed.
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("failed to GC tombstone file %s: %w", entry.Name(), err)
			}
			if err := removeShadowedRecord(root, t); err != nil {
				return nil, err
			}
			continue
		}
		keep(t)
	}

	return live, nil
}

// removeShadowedRecord deletes the record file(s) a tombstone shadows.
// This is the only path that ever deletes record files from a context tree.
func removeShadowedRecord(root string, t *memory.Tombstone) error {
	switch t.Kind {
	case memory.TombstoneKindMemory:
		err := os.Remove(MemoryPath(root, t.ID))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to remove tombstoned memory %s: %w", t.ID, err)
		}
	case memory.TombstoneKindDecisionTopic:
		if err := os.RemoveAll(filepath.Join(root, decisionsDir, TopicName(t.ID))); err != nil {
			return fmt.Errorf("failed to remove tombstoned topic %s: %w", t.ID, err)
		}
	}
	return nil
}

// exportDecisions writes one write-once file per decision version.
func exportDecisions(src Source, root string, live map[string]*memory.Tombstone) (int, error) {
	decisions, err := src.ListDecisions()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, d := range decisions {
		if shadowed(live, memory.TombstoneKindDecisionTopic, d.Topic, recordTimeDecision(d)) {
			continue
		}
		path := DecisionPath(root, d)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return count, err
		}
		// Write-once: version files are immutable by identity.
		if _, err := os.Stat(path); err == nil {
			count++
			continue
		}
		if err := os.WriteFile(path, MarshalDecision(d), 0o644); err != nil {
			return count, fmt.Errorf("failed to write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// exportMemories writes one file per shareable memory. Soft-deleted memories
// (forget tag) are included as data: the updated record carries its forget
// tag and newer UpdatedAt, so peers that imported an earlier version — or
// never had the memory at all — converge to the forgotten state instead of
// keeping (or resurrecting) an unforgotten copy from a stale tree file.
func exportMemories(src Source, root string, live map[string]*memory.Tombstone) (int, error) {
	memories, err := src.ListMemories(memory.SearchOptions{IncludeAll: true})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, m := range memories {
		if !IsShareableMemory(m) {
			continue
		}
		if shadowed(live, memory.TombstoneKindMemory, m.ID, recordTimeMemory(m)) {
			continue
		}
		path := MemoryPath(root, m.ID)
		data := MarshalMemory(m)
		// Rewrite only when the owner's record changed, so unchanged content
		// stays byte-identical (and mtime-stable) across re-exports.
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
			count++
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return count, fmt.Errorf("failed to write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// shadowed reports whether a live tombstone covers a record. A record
// re-created after its tombstone (newer timestamp) is not shadowed.
func shadowed(live map[string]*memory.Tombstone, kind, id string, recordTime time.Time) bool {
	t, ok := live[kind+":"+id]
	return ok && !recordTime.After(t.DeletedAt)
}

// recordTimeMemory returns the LWW timestamp of a memory: UpdatedAt when the
// record has been edited, otherwise CreatedAt.
func recordTimeMemory(m *memory.Memory) time.Time {
	if !m.UpdatedAt.IsZero() {
		return m.UpdatedAt
	}
	return m.CreatedAt
}

// recordTimeDecision returns the timestamp of a decision version.
func recordTimeDecision(d *memory.Decision) time.Time {
	return d.CreatedAt
}
