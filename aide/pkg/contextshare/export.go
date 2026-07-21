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
// are unavailable (no store at all); export then skips materialising DB
// tombstones and import skips local tombstone bookkeeping, degrading to the
// pre-tombstone behaviour rather than failing. In daemon mode the access is
// served over gRPC (see grpcapi adapter.TombstoneAdapter), so this is non-nil.
type TombstoneAccess interface {
	AddTombstone(t *memory.Tombstone) error
	GetTombstone(kind, id string) (*memory.Tombstone, error)
	ListTombstones() ([]*memory.Tombstone, error)
	DeleteTombstone(kind, id string) error
}

// ExportOptions configures Export.
//
// Decisions and Memories are the per-type publish gates: when false, no records
// of that type are written. Tombstones are always materialised and expired
// regardless of these gates — deletions propagate independently of the type
// policy, so a disabled type still has its pending deletions recorded.
// DecisionFilter and MemoryFilter then select which records of an enabled type
// ship, globbing over each record's token set (see DecisionTokens/MemoryTokens).
// The cmd layer maps user config into these; contextshare just applies them.
type ExportOptions struct {
	Now            time.Time     // Watermark and TTL reference; zero = time.Now()
	TombstoneTTL   time.Duration // Zero = DefaultTombstoneTTL
	Decisions      bool          // Export decisions when true
	Memories       bool          // Export memories when true
	DecisionFilter Filter        // Applied to each decision's token set
	MemoryFilter   Filter        // Applied to each memory's token set
}

// ExportStats reports what an export wrote — and what the configured
// filters held back, so callers can surface "N excluded by policy"
// instead of records disappearing silently.
type ExportStats struct {
	Decisions         int // Decision version files present after export
	Memories          int // Memory files present after export
	Tombstones        int // Live tombstone files present after export
	DecisionsExcluded int // Decision versions rejected by the export filter
	MemoriesExcluded  int // Memories rejected by the export filter
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

	if opts.Decisions {
		n, excluded, err := exportDecisions(src, root, live, opts.DecisionFilter)
		if err != nil {
			return nil, err
		}
		stats.Decisions = n
		stats.DecisionsExcluded = excluded
	}

	if opts.Memories {
		n, excluded, err := exportMemories(src, root, live, opts.MemoryFilter)
		if err != nil {
			return nil, err
		}
		stats.Memories = n
		stats.MemoriesExcluded = excluded
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

// exportDecisions writes one write-once file per decision version that passes
// the filter.
func exportDecisions(src Source, root string, live map[string]*memory.Tombstone, filter Filter) (int, int, error) {
	decisions, err := src.ListDecisions()
	if err != nil {
		return 0, 0, err
	}

	count, excluded := 0, 0
	for _, d := range decisions {
		if !filter.Match(DecisionTokens(d)) {
			excluded++
			continue
		}
		if shadowed(live, memory.TombstoneKindDecisionTopic, d.Topic, recordTimeDecision(d)) {
			continue
		}
		path := DecisionPath(root, d)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return count, excluded, err
		}
		// Write-once: version files are immutable by identity.
		if _, err := os.Stat(path); err == nil {
			count++
			continue
		}
		if err := os.WriteFile(path, MarshalDecision(d), 0o644); err != nil {
			return count, excluded, fmt.Errorf("failed to write %s: %w", path, err)
		}
		count++
	}
	return count, excluded, nil
}

// exportMemories writes one file per memory that passes the filter.
// Soft-deleted memories (forget tag) that still pass the filter are included
// as data: the updated record carries its forget tag and newer UpdatedAt, so
// peers that imported an earlier version — or never had the memory at all —
// converge to the forgotten state instead of keeping (or resurrecting) an
// unforgotten copy from a stale tree file. Filtering by tokens, not by the old
// IsShareableMemory gate, is what makes the export policy user-configurable.
func exportMemories(src Source, root string, live map[string]*memory.Tombstone, filter Filter) (int, int, error) {
	memories, err := src.ListMemories(memory.SearchOptions{IncludeAll: true})
	if err != nil {
		return 0, 0, err
	}

	count, excluded := 0, 0
	for _, m := range memories {
		if !filter.Match(MemoryTokens(m)) {
			excluded++
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
			return count, excluded, fmt.Errorf("failed to write %s: %w", path, err)
		}
		count++
	}
	return count, excluded, nil
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
