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

// ErrStaleExport is returned when a context tree's manifest is missing or
// its watermark predates the tombstone TTL. Merging such a tree could
// resurrect records whose tombstones have already been garbage-collected.
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

// ImportOptions configures Import. The zero value imports everything with
// the default TTL at the current time.
type ImportOptions struct {
	Now           time.Time     // TTL reference; zero = time.Now()
	TombstoneTTL  time.Duration // Zero = DefaultTombstoneTTL
	Force         bool          // Bypass the stale-export guard
	DryRun        bool          // Report what would change without writing
	SkipDecisions bool
	SkipMemories  bool
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
func Import(tgt Target, tombs TombstoneAccess, root string, opts ImportOptions) (*ImportStats, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	ttl := opts.TombstoneTTL
	if ttl <= 0 {
		ttl = DefaultTombstoneTTL
	}

	if err := checkStaleGuard(root, now, ttl, opts.Force); err != nil {
		return nil, err
	}

	stats := &ImportStats{}

	if err := importTombstones(tgt, tombs, root, now, ttl, opts.DryRun, stats); err != nil {
		return nil, err
	}
	if !opts.SkipDecisions {
		if err := importDecisions(tgt, tombs, root, now, ttl, opts.DryRun, stats); err != nil {
			return nil, err
		}
	}
	if !opts.SkipMemories {
		if err := importMemories(tgt, tombs, root, now, ttl, opts.DryRun, stats); err != nil {
			return nil, err
		}
	}
	return stats, nil
}

// checkStaleGuard refuses trees whose manifest is missing or older than the
// tombstone TTL, since tombstones the publisher GC'd may already be gone.
func checkStaleGuard(root string, now time.Time, ttl time.Duration, force bool) error {
	_, watermark, err := ReadManifest(root)
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
	if now.Sub(watermark) > ttl {
		if force {
			return nil
		}
		return fmt.Errorf("%w: watermark %s is older than %s, so deletions may have been garbage-collected — ask the publisher for a fresh `aide share export`, or use --force to import anyway",
			ErrStaleExport, watermark.Format(time.RFC3339), ttl)
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
func importDecisions(tgt Target, tombs TombstoneAccess, root string, now time.Time, ttl time.Duration, dryRun bool, stats *ImportStats) error {
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
func importMemories(tgt Target, tombs TombstoneAccess, root string, now time.Time, ttl time.Duration, dryRun bool, stats *ImportStats) error {
	records, err := readRecordDir[memory.Memory](filepath.Join(root, memoriesDir), ParseMemory)
	if err != nil {
		return err
	}

	for _, m := range records {
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
