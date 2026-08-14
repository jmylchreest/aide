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
// of that type are written. Tombstones still propagate across a closed gate:
// a deletion is materialised whenever it has published work to do — either its
// type is being exported, or a record file it shadows is still in the tree and
// must be unpublished. The one case that is skipped is a deletion that was
// never published and never can be: type off and nothing left to remove, where
// no subscriber can hold the record either. Expiry applies to all of them.
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
	// TombstonesSkipped counts deletions that were not materialised because
	// their type is not published and they had nothing left to unpublish.
	TombstonesSkipped int
}

// exportResult is what one per-type pass reports back. Newest is the newest
// record timestamp the pass published, which Export folds into the manifest
// watermark.
type exportResult struct {
	Count    int
	Excluded int
	Newest   time.Time
}

// Export projects the shareable subset of src into the context tree at root.
//
// Record files are write-once: an existing decision version file is never
// rewritten, and a memory file is only rewritten when its owner's record
// changed. Nothing is deleted except records shadowed by a tombstone and
// tombstones past their TTL. Re-exporting an unchanged store is a complete
// no-op, manifest included — opts.Now reaches the tree only through the TTL
// decisions it drives, never as a timestamp of its own.
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

	live, gcHorizon, err := collectLiveTombstones(tombs, root, now, ttl)
	if err != nil {
		return nil, err
	}

	var newest time.Time

	// Materialise live tombstones and remove the record files they shadow.
	for _, t := range live {
		path := TombstonePath(root, t)
		_, statErr := os.Stat(path)
		if errors.Is(statErr, fs.ErrNotExist) {
			// This deletion has never been published. If its type is not being
			// published either, and no record file is left to unpublish, the
			// tombstone has no work to do here or downstream — no subscriber
			// can hold a record that was never exported. Skip it rather than
			// commit an inert file, which is the common shape for a store whose
			// memories stay local while its decisions ship.
			if !typePublished(t.Kind, opts) && !shadowedRecordExists(root, t) {
				stats.TombstonesSkipped++
				continue
			}
			if err := os.WriteFile(path, MarshalTombstone(t), 0o644); err != nil {
				return nil, fmt.Errorf("failed to write tombstone %s: %w", t.ID, err)
			}
		}
		if err := removeShadowedRecord(root, t); err != nil {
			return nil, err
		}
		newest = later(newest, t.DeletedAt)
		stats.Tombstones++
	}

	if opts.Decisions {
		res, err := exportDecisions(src, root, live, opts.DecisionFilter)
		if err != nil {
			return nil, err
		}
		stats.Decisions = res.Count
		stats.DecisionsExcluded = res.Excluded
		newest = later(newest, res.Newest)
	}

	if opts.Memories {
		res, err := exportMemories(src, root, live, opts.MemoryFilter)
		if err != nil {
			return nil, err
		}
		stats.Memories = res.Count
		stats.MemoriesExcluded = res.Excluded
		newest = later(newest, res.Newest)
	}

	// The horizon only ever moves forward, and a re-export that GCs nothing
	// must leave it where the previous one put it. A tree whose manifest is
	// missing or unreadable restarts from whatever this run dropped: the
	// alternative is refusing to export at all over a file the export is
	// about to rewrite anyway.
	horizon := gcHorizon
	if prev, err := ReadManifest(root); err == nil {
		horizon = later(horizon, prev.Horizon)
	}

	if err := WriteManifest(root, Manifest{Watermark: newest, Horizon: horizon}); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}
	return stats, nil
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// collectLiveTombstones merges DB tombstones with tombstone files already in
// the tree (newest DeletedAt wins per id), garbage-collecting both expired DB
// rows and expired files along the way.
//
// The second return is the GC horizon: the newest DeletedAt this call dropped,
// zero when it dropped nothing. Every deletion at or before that point is now
// unrecoverable from this tree, which is exactly what an importer needs to
// know to tell whether it can trust the tree's record of deletions.
func collectLiveTombstones(tombs TombstoneAccess, root string, now time.Time, ttl time.Duration) (map[string]*memory.Tombstone, time.Time, error) {
	live := make(map[string]*memory.Tombstone)
	var gcHorizon time.Time
	keep := func(t *memory.Tombstone) {
		key := t.Kind + ":" + t.ID
		if existing, ok := live[key]; !ok || t.DeletedAt.After(existing.DeletedAt) {
			live[key] = t
		}
	}

	if tombs != nil {
		dbTombstones, err := tombs.ListTombstones()
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to list tombstones: %w", err)
		}
		for _, t := range dbTombstones {
			if now.Sub(t.DeletedAt) > ttl {
				if err := tombs.DeleteTombstone(t.Kind, t.ID); err != nil {
					return nil, time.Time{}, fmt.Errorf("failed to GC tombstone %s: %w", t.ID, err)
				}
				gcHorizon = later(gcHorizon, t.DeletedAt)
				continue
			}
			keep(t)
		}
	}

	dir := filepath.Join(root, tombstonesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, time.Time{}, err
		}
		t, err := ParseTombstone(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping tombstone %s: %v\n", entry.Name(), err)
			continue
		}
		if now.Sub(t.DeletedAt) > ttl {
			// Past TTL: GC the tombstone file and the record it shadowed.
			if err := os.Remove(path); err != nil {
				return nil, time.Time{}, fmt.Errorf("failed to GC tombstone file %s: %w", entry.Name(), err)
			}
			if err := removeShadowedRecord(root, t); err != nil {
				return nil, time.Time{}, err
			}
			gcHorizon = later(gcHorizon, t.DeletedAt)
			continue
		}
		keep(t)
	}

	return live, gcHorizon, nil
}

// removeShadowedRecord deletes the record file(s) a tombstone shadows.
// This is the only path that ever deletes record files from a context tree.
// typePublished reports whether the record type a tombstone shadows is being
// exported by this run.
func typePublished(kind string, opts ExportOptions) bool {
	switch kind {
	case memory.TombstoneKindMemory:
		return opts.Memories
	case memory.TombstoneKindDecisionTopic:
		return opts.Decisions
	}
	// Unknown kinds are treated as published so an unrecognised tombstone is
	// still materialised — never silently dropped.
	return true
}

// shadowedRecordExists reports whether the record a tombstone shadows is still
// present in the tree, i.e. whether there is anything left to unpublish.
func shadowedRecordExists(root string, t *memory.Tombstone) bool {
	switch t.Kind {
	case memory.TombstoneKindMemory:
		_, err := os.Stat(MemoryPath(root, t.ID))
		return err == nil
	case memory.TombstoneKindDecisionTopic:
		_, err := os.Stat(filepath.Join(root, decisionsDir, TopicName(t.ID)))
		return err == nil
	}
	return false
}

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
func exportDecisions(src Source, root string, live map[string]*memory.Tombstone, filter Filter) (exportResult, error) {
	decisions, err := src.ListDecisions()
	if err != nil {
		return exportResult{}, err
	}

	var res exportResult
	for _, d := range decisions {
		if !filter.Match(DecisionTokens(d)) {
			res.Excluded++
			continue
		}
		if shadowed(live, memory.TombstoneKindDecisionTopic, d.Topic, recordTimeDecision(d)) {
			continue
		}
		path := DecisionPath(root, d)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return res, err
		}
		// Write-once: version files are immutable by identity.
		if _, err := os.Stat(path); err != nil {
			if err := os.WriteFile(path, MarshalDecision(d), 0o644); err != nil {
				return res, fmt.Errorf("failed to write %s: %w", path, err)
			}
		}
		res.Count++
		res.Newest = later(res.Newest, recordTimeDecision(d))
	}
	return res, nil
}

// exportMemories writes one file per memory that passes the filter.
// Soft-deleted memories (forget tag) that still pass the filter are included
// as data: the updated record carries its forget tag and newer UpdatedAt, so
// peers that imported an earlier version — or never had the memory at all —
// converge to the forgotten state instead of keeping (or resurrecting) an
// unforgotten copy from a stale tree file. Filtering by tokens, not by the old
// IsShareableMemory gate, is what makes the export policy user-configurable.
func exportMemories(src Source, root string, live map[string]*memory.Tombstone, filter Filter) (exportResult, error) {
	memories, err := src.ListMemories(memory.SearchOptions{IncludeAll: true})
	if err != nil {
		return exportResult{}, err
	}

	var res exportResult
	for _, m := range memories {
		if !filter.Match(MemoryTokens(m)) {
			res.Excluded++
			continue
		}
		if shadowed(live, memory.TombstoneKindMemory, m.ID, recordTimeMemory(m)) {
			continue
		}
		path := MemoryPath(root, m.ID)
		data := MarshalMemory(m)
		// Rewrite only when the owner's record changed, so unchanged content
		// stays byte-identical (and mtime-stable) across re-exports.
		if existing, err := os.ReadFile(path); err != nil || !bytes.Equal(existing, data) {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return res, fmt.Errorf("failed to write %s: %w", path, err)
			}
		}
		res.Count++
		res.Newest = later(res.Newest, recordTimeMemory(m))
	}
	return res, nil
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
