package contextshare

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// ErrStaleExport is returned when a context tree cannot be merged safely:
// its manifest is missing, or the caller last imported before the tree's
// deletion horizon and would therefore keep records whose tombstones have
// since been garbage-collected.
var ErrStaleExport = errors.New("stale context export")

// Target is the write surface Import needs from the store.
type Target interface {
	GetDecisionHistory(topic string) ([]*memory.Decision, error)
	SetDecision(d *memory.Decision) error
	DeleteDecision(topic string) (int, error)
	GetMemory(id string) (*memory.Memory, error)
	AddMemory(m *memory.Memory) error
	UpdateMemory(m *memory.Memory) error
	DeleteMemory(id string) error
}

// ImportOptions configures Import.
//
// Decisions and Memories are the per-type consume gates: when false, that type
// is skipped entirely. DecisionFilter and MemoryFilter then select which
// incoming records of an enabled type are ingested, globbing over each record's
// token set. Tombstones are always processed regardless of the type gates so
// deletions still propagate and resurrection is prevented.
type ImportOptions struct {
	Now            time.Time     // TTL reference; zero = time.Now()
	LastImport     time.Time     // This target's last successful import of this tree; zero = never
	TombstoneTTL   time.Duration // Zero = DefaultTombstoneTTL
	Force          bool          // Bypass the stale-export guard
	DryRun         bool          // Report what would change without writing
	Decisions      bool          // Import decisions when true
	Memories       bool          // Import memories when true
	DecisionFilter Filter        // Applied to each incoming decision's token set
	MemoryFilter   Filter        // Applied to each incoming memory's token set
}

// ImportStats reports what an import changed. Tombstone effects are counted
// separately: RecordsDeleted is local records actually removed by incoming
// tombstones, TombstonesRecorded is tombstones newly stored (or refreshed to
// a newer DeletedAt) locally so the deletion propagates onward. A single
// tombstone can count towards both, either, or neither (then it is ignored).
type ImportStats struct {
	DecisionsImported  int
	DecisionsSkipped   int
	MemoriesImported   int
	MemoriesSkipped    int
	RecordsDeleted     int
	TombstonesRecorded int
	TombstonesIgnored  int
}

// Import merges the context tree at root into tgt.
//
// Decisions are an append-only set per topic: any version (topic +
// created_at) not present locally is inserted with its original timestamp,
// so "latest" is decided by when decisions were made, not when they were
// imported. Memories are last-write-wins by updated_at with a monotonic
// forget tag: once a clone has soft-deleted a memory, no import strips that
// tag. Tombstones delete matching local records and are recorded locally so
// the deletion propagates onward.
//
// Callers that keep a store across imports must pass opts.LastImport and
// persist the import time on success; without it the guard cannot tell a
// caller that is caught up from one that has been away long enough to have
// missed a garbage-collected deletion. Store-free readers, which cannot
// resurrect anything, pass Force instead.
func Import(tgt Target, tombs TombstoneAccess, root string, opts ImportOptions) (*ImportStats, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	ttl := opts.TombstoneTTL
	if ttl <= 0 {
		ttl = DefaultTombstoneTTL
	}

	if err := checkStaleGuard(root, now, opts.LastImport, ttl, opts.Force); err != nil {
		return nil, err
	}

	stats := &ImportStats{}

	if err := importTombstones(tgt, tombs, root, now, ttl, opts.DryRun, stats); err != nil {
		return nil, err
	}
	if opts.Decisions {
		if err := importDecisions(tgt, tombs, root, now, ttl, opts.DryRun, opts.DecisionFilter, stats); err != nil {
			return nil, err
		}
	}
	if opts.Memories {
		if err := importMemories(tgt, tombs, root, now, ttl, opts.DryRun, opts.MemoryFilter, stats); err != nil {
			return nil, err
		}
	}
	return stats, nil
}

// checkStaleGuard refuses trees this target cannot merge without risking
// resurrection.
//
// The question is never "how old is this tree" — a tree whose context has
// been settled for a year is perfectly current. It is "has this tree thrown
// away a deletion I never saw", and that is a comparison between the tree's
// horizon and how far behind the caller is. A caller that has never imported
// (zero LastImport) cannot be holding a stale copy of anything, so it is
// always safe.
//
// v1 trees carry no horizon, only an export-time watermark, so they fall back
// to the guard that shipped with them: refuse once the export itself is older
// than the TTL. That is the conservative reading — it can refuse a tree that
// was merely stable — but a v1 publisher bumped its watermark on every export,
// so in practice an old watermark did mean an abandoned tree.
func checkStaleGuard(root string, now, lastImport time.Time, ttl time.Duration, force bool) error {
	m, err := ReadManifest(root)
	if err != nil {
		if force {
			return nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s has no %s — ask the publisher for a fresh `aide share export`, or use --force to import anyway",
				ErrStaleExport, root, ManifestName)
		}
		return fmt.Errorf("%w: %v — ask the publisher for a fresh `aide share export`, or use --force to import anyway",
			ErrStaleExport, err)
	}

	if m.Version < 2 {
		if !force && now.Sub(m.Watermark) > ttl {
			return fmt.Errorf("%w: v1 tree exported %s, more than %s ago, so deletions may have been garbage-collected — ask the publisher for a fresh `aide share export`, or use --force to import anyway",
				ErrStaleExport, m.Watermark.Format(time.RFC3339), ttl)
		}
		return nil
	}

	if !force && !lastImport.IsZero() && !m.Horizon.IsZero() && lastImport.Before(m.Horizon) {
		return fmt.Errorf("%w: deletions up to %s have been garbage-collected from this tree and you last imported it %s, so records deleted in that gap would survive locally — use --force to import anyway and reconcile by hand",
			ErrStaleExport, m.Horizon.Format(time.RFC3339), lastImport.Format(time.RFC3339))
	}
	return nil
}

// importTombstones applies incoming tombstones: delete the matching local
// record when it predates the deletion, and record the tombstone locally so
// it propagates onward. Tombstones past the TTL are ignored, not errors.
func importTombstones(tgt Target, tombs TombstoneAccess, root string, now time.Time, ttl time.Duration, dryRun bool, stats *ImportStats) error {
	records, err := readRecordDir[memory.Tombstone](filepath.Join(root, tombstonesDir), ParseTombstone)
	if err != nil {
		return err
	}

	for _, t := range records {
		if now.Sub(t.DeletedAt) > ttl {
			stats.TombstonesIgnored++
			continue
		}

		// Capture the pre-existing local tombstone before any delete call:
		// store-level deletes record their own now()-stamped tombstone, which
		// we overwrite below to preserve the original deletion time.
		var pre *memory.Tombstone
		if tombs != nil {
			pre, _ = tombs.GetTombstone(t.Kind, t.ID)
		}

		deleted := false
		switch t.Kind {
		case memory.TombstoneKindMemory:
			if local, err := tgt.GetMemory(t.ID); err == nil && local != nil {
				if recordTimeMemory(local).Before(t.DeletedAt) {
					if !dryRun {
						if err := tgt.DeleteMemory(t.ID); err != nil {
							return fmt.Errorf("failed to apply tombstone for memory %s: %w", t.ID, err)
						}
					}
					deleted = true
				}
			}
		case memory.TombstoneKindDecisionTopic:
			history, err := tgt.GetDecisionHistory(t.ID)
			if err == nil && len(history) > 0 && allDecisionsBefore(history, t.DeletedAt) {
				if !dryRun {
					if _, err := tgt.DeleteDecision(t.ID); err != nil {
						return fmt.Errorf("failed to apply tombstone for topic %s: %w", t.ID, err)
					}
				}
				deleted = true
			}
		}

		recorded := false
		if tombs != nil && (pre == nil || pre.DeletedAt.Before(t.DeletedAt)) {
			if !dryRun {
				if err := tombs.AddTombstone(t); err != nil {
					return fmt.Errorf("failed to record tombstone %s: %w", t.ID, err)
				}
			}
			recorded = true
		}

		if deleted {
			stats.RecordsDeleted++
		}
		if recorded {
			stats.TombstonesRecorded++
		}
		if !deleted && !recorded {
			stats.TombstonesIgnored++
		}
	}
	return nil
}

// allDecisionsBefore reports whether every version of a topic predates the
// cutoff. A topic re-decided after its deletion must not be deleted again.
func allDecisionsBefore(history []*memory.Decision, cutoff time.Time) bool {
	for _, d := range history {
		if !d.CreatedAt.Before(cutoff) {
			return false
		}
	}
	return true
}

// importDecisions inserts every decision version not already present
// locally, preserving its original CreatedAt so lineage and "latest"
// survive the wire regardless of import order.
func importDecisions(tgt Target, tombs TombstoneAccess, root string, now time.Time, ttl time.Duration, dryRun bool, filter Filter, stats *ImportStats) error {
	dir := filepath.Join(root, decisionsDir)
	topics, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, topicDir := range topics {
		if !topicDir.IsDir() {
			continue
		}
		records, err := readRecordDir[memory.Decision](filepath.Join(dir, topicDir.Name()), ParseDecision)
		if err != nil {
			return err
		}

		for _, d := range records {
			if !filter.Match(DecisionTokens(d)) {
				stats.DecisionsSkipped++
				continue
			}
			if blockedByLocalTombstone(tombs, memory.TombstoneKindDecisionTopic, d.Topic, d.CreatedAt, now, ttl) {
				stats.DecisionsSkipped++
				continue
			}
			history, err := tgt.GetDecisionHistory(d.Topic)
			if err != nil {
				return fmt.Errorf("failed to read history for %s: %w", d.Topic, err)
			}
			if slices.ContainsFunc(history, func(h *memory.Decision) bool {
				return h.CreatedAt.UnixNano() == d.CreatedAt.UnixNano()
			}) {
				stats.DecisionsSkipped++
				continue
			}
			if !dryRun {
				if err := tgt.SetDecision(d); err != nil {
					return fmt.Errorf("failed to import decision %s: %w", d.Topic, err)
				}
			}
			stats.DecisionsImported++
		}
	}
	return nil
}

// importMemories merges incoming memories: unknown ULIDs are added with
// their original timestamps, known ULIDs are last-write-wins by UpdatedAt,
// and the forget tag is monotonic — a newer incoming version never strips a
// local forget.
func importMemories(tgt Target, tombs TombstoneAccess, root string, now time.Time, ttl time.Duration, dryRun bool, filter Filter, stats *ImportStats) error {
	records, err := readRecordDir[memory.Memory](filepath.Join(root, memoriesDir), ParseMemory)
	if err != nil {
		return err
	}

	for _, m := range records {
		if !filter.Match(MemoryTokens(m)) {
			stats.MemoriesSkipped++
			continue
		}
		if blockedByLocalTombstone(tombs, memory.TombstoneKindMemory, m.ID, recordTimeMemory(m), now, ttl) {
			stats.MemoriesSkipped++
			continue
		}

		existing, err := tgt.GetMemory(m.ID)
		if err != nil || existing == nil {
			if !dryRun {
				if err := tgt.AddMemory(m); err != nil {
					return fmt.Errorf("failed to import memory %s: %w", m.ID, err)
				}
			}
			stats.MemoriesImported++
			continue
		}

		if m.UpdatedAt.IsZero() || !m.UpdatedAt.After(existing.UpdatedAt) {
			stats.MemoriesSkipped++
			continue
		}

		merged := *existing
		merged.Content = m.Content
		merged.Category = m.Category
		merged.Tags = mergeForgetTag(existing.Tags, m.Tags)
		merged.UpdatedAt = m.UpdatedAt
		if !dryRun {
			if err := tgt.UpdateMemory(&merged); err != nil {
				return fmt.Errorf("failed to update memory %s: %w", m.ID, err)
			}
		}
		stats.MemoriesImported++
	}
	return nil
}

// mergeForgetTag returns the incoming tags, unioning in the forget tag when
// the local record carried it. Soft-deletion is monotonic: a teammate's
// newer edit must not undo a local forget.
func mergeForgetTag(localTags, incomingTags []string) []string {
	if !slices.Contains(localTags, "forget") || slices.Contains(incomingTags, "forget") {
		return incomingTags
	}
	return append(slices.Clone(incomingTags), "forget")
}

// blockedByLocalTombstone reports whether a live local tombstone shadows an
// incoming record. Records newer than the tombstone (re-creations) pass.
func blockedByLocalTombstone(tombs TombstoneAccess, kind, id string, recordTime time.Time, now time.Time, ttl time.Duration) bool {
	if tombs == nil {
		return false
	}
	t, err := tombs.GetTombstone(kind, id)
	if err != nil || t == nil {
		return false
	}
	if now.Sub(t.DeletedAt) > ttl {
		return false
	}
	return !recordTime.After(t.DeletedAt)
}

// readRecordDir parses every .md record in dir, warning on (and skipping)
// malformed files rather than failing the whole import. Results are sorted
// by file name for deterministic application order.
func readRecordDir[T any](dir string, parse func([]byte) (*T, error)) ([]*T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var records []*T
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		rec, err := parse(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}
