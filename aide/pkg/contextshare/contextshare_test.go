package contextshare

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// newTestStore opens a fresh BoltStore in a temp dir. BoltStore implements
// Source, Target and TombstoneAccess, so the tests exercise the real merge
// surface end to end.
func newTestStore(t *testing.T) *store.BoltStore {
	t.Helper()
	s, err := store.NewBoltStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// matchAll is the include-everything, exclude-nothing filter the merge-surface
// tests use unless they exercise filtering explicitly.
var matchAll = Filter{Include: []string{"*"}}

// mustExport exports both record types with match-all filters by default, so
// the merge-surface tests (which predate the configurable policy) keep their
// "export everything shareable" semantics. Tests that need a narrower policy
// set Decisions/Memories/filters on opts before calling Export directly.
func mustExport(t *testing.T, s *store.BoltStore, root string, opts ExportOptions) *ExportStats {
	t.Helper()
	if !opts.Decisions && !opts.Memories {
		opts.Decisions, opts.Memories = true, true
	}
	if len(opts.DecisionFilter.Include) == 0 {
		opts.DecisionFilter = matchAll
	}
	if len(opts.MemoryFilter.Include) == 0 {
		opts.MemoryFilter = matchAll
	}
	stats, err := Export(s, s, root, opts)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return stats
}

// mustImport mirrors mustExport: both types, match-all filters by default.
func mustImport(t *testing.T, s *store.BoltStore, root string, opts ImportOptions) *ImportStats {
	t.Helper()
	if !opts.Decisions && !opts.Memories {
		opts.Decisions, opts.Memories = true, true
	}
	if len(opts.DecisionFilter.Include) == 0 {
		opts.DecisionFilter = matchAll
	}
	if len(opts.MemoryFilter.Include) == 0 {
		opts.MemoryFilter = matchAll
	}
	stats, err := Import(s, s, root, opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return stats
}

// treeSnapshot maps relative paths to file contents, excluding the manifest
// (the only file allowed to differ across re-exports of unchanged content).
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ManifestName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snap[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("treeSnapshot: %v", err)
	}
	return snap
}

// storeSnapshot canonicalises a store's shareable state for convergence
// comparison: decision versions, memories (including forgotten ones), and
// tombstones.
func storeSnapshot(t *testing.T, s *store.BoltStore) map[string]string {
	t.Helper()
	snap := map[string]string{}

	decisions, err := s.ListDecisions()
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	for _, d := range decisions {
		snap[fmt.Sprintf("decision:%s@%d", d.Topic, d.CreatedAt.UnixNano())] = d.Decision
	}

	memories, err := s.ListMemories(memory.SearchOptions{IncludeAll: true})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	for _, m := range memories {
		tags := slices.Clone(m.Tags)
		slices.Sort(tags)
		snap["memory:"+m.ID] = fmt.Sprintf("%s|%s|%d", m.Content, strings.Join(tags, ","), recordTimeMemory(m).UnixNano())
	}

	tombstones, err := s.ListTombstones()
	if err != nil {
		t.Fatalf("ListTombstones: %v", err)
	}
	for _, ts := range tombstones {
		snap["tombstone:"+ts.Kind+":"+ts.ID] = fmt.Sprintf("%d", ts.DeletedAt.UnixNano())
	}

	return snap
}

// =============================================================================
// Record format round-trips
// =============================================================================

func TestDecisionMarshalParseRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		d    *memory.Decision
	}{
		{
			name: "full record",
			d: &memory.Decision{
				Topic:      "auth-strategy",
				Decision:   "JWT with refresh tokens",
				Rationale:  "Stateless, mobile-friendly",
				Details:    "Use RS256 signing\nwith rotation",
				DecidedBy:  "agent-1",
				References: []string{"https://jwt.io", "docs/auth.md"},
				CreatedAt:  time.Date(2026, 1, 15, 10, 30, 45, 123456789, time.UTC),
			},
		},
		{
			name: "minimal record",
			d: &memory.Decision{
				Topic:     "db",
				Decision:  "PostgreSQL",
				CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "yaml-special decision text",
			d: &memory.Decision{
				Topic:     "naming",
				Decision:  `use "kebab-case": always, everywhere`,
				Rationale: "consistency",
				CreatedAt: time.Date(2026, 3, 1, 8, 0, 0, 42, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseDecision(MarshalDecision(tt.d))
			if err != nil {
				t.Fatalf("ParseDecision: %v", err)
			}
			if parsed.Topic != tt.d.Topic {
				t.Errorf("topic: got %q, want %q", parsed.Topic, tt.d.Topic)
			}
			if parsed.Decision != tt.d.Decision {
				t.Errorf("decision: got %q, want %q", parsed.Decision, tt.d.Decision)
			}
			if parsed.Rationale != tt.d.Rationale {
				t.Errorf("rationale: got %q, want %q", parsed.Rationale, tt.d.Rationale)
			}
			if parsed.Details != tt.d.Details {
				t.Errorf("details: got %q, want %q", parsed.Details, tt.d.Details)
			}
			if parsed.DecidedBy != tt.d.DecidedBy {
				t.Errorf("decided_by: got %q, want %q", parsed.DecidedBy, tt.d.DecidedBy)
			}
			if !reflect.DeepEqual(parsed.References, tt.d.References) {
				t.Errorf("references: got %v, want %v", parsed.References, tt.d.References)
			}
			if !parsed.CreatedAt.Equal(tt.d.CreatedAt) {
				t.Errorf("created_at: got %s, want %s (nanosecond precision must survive)", parsed.CreatedAt, tt.d.CreatedAt)
			}
		})
	}
}

func TestMemoryMarshalParseRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		m    *memory.Memory
	}{
		{
			name: "full record",
			m: &memory.Memory{
				ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				Category:  memory.CategoryLearning,
				Content:   "Auth middleware lives at src/auth.ts",
				Tags:      []string{"project:myapp", "scope:global"},
				CreatedAt: time.Date(2026, 1, 10, 9, 0, 0, 987654321, time.UTC),
				UpdatedAt: time.Date(2026, 1, 12, 9, 30, 0, 1, time.UTC),
			},
		},
		{
			name: "multiline content with delimiter lines",
			m: &memory.Memory{
				ID:        "01BX5ZZKBKACTAV9WEVGEMMVRZ",
				Category:  "pattern",
				Content:   "First line\n\n---\n\ncode:\n  indented\nlast line",
				CreatedAt: time.Date(2026, 2, 2, 2, 2, 2, 0, time.UTC),
			},
		},
		{
			name: "never edited (zero UpdatedAt)",
			m: &memory.Memory{
				ID:        "01BX5ZZKBKACTAV9WEVGEMMVS0",
				Category:  "gotcha",
				Content:   "single line",
				CreatedAt: time.Date(2026, 3, 3, 3, 3, 3, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseMemory(MarshalMemory(tt.m))
			if err != nil {
				t.Fatalf("ParseMemory: %v", err)
			}
			if parsed.ID != tt.m.ID {
				t.Errorf("id: got %q, want %q", parsed.ID, tt.m.ID)
			}
			if parsed.Category != tt.m.Category {
				t.Errorf("category: got %q, want %q", parsed.Category, tt.m.Category)
			}
			if parsed.Content != tt.m.Content {
				t.Errorf("content: got %q, want %q (body must round-trip verbatim)", parsed.Content, tt.m.Content)
			}
			if !reflect.DeepEqual(parsed.Tags, tt.m.Tags) {
				t.Errorf("tags: got %v, want %v", parsed.Tags, tt.m.Tags)
			}
			if !parsed.CreatedAt.Equal(tt.m.CreatedAt) {
				t.Errorf("created_at: got %s, want %s", parsed.CreatedAt, tt.m.CreatedAt)
			}
			if !parsed.UpdatedAt.Equal(tt.m.UpdatedAt) {
				t.Errorf("updated_at: got %s, want %s", parsed.UpdatedAt, tt.m.UpdatedAt)
			}
		})
	}
}

func TestTombstoneMarshalParseRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ts   *memory.Tombstone
	}{
		{
			name: "memory tombstone",
			ts: &memory.Tombstone{
				ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				Kind:      memory.TombstoneKindMemory,
				DeletedAt: time.Date(2026, 4, 1, 12, 0, 0, 5, time.UTC),
			},
		},
		{
			name: "decision topic tombstone",
			ts: &memory.Tombstone{
				ID:        "auth strategy/v2",
				Kind:      memory.TombstoneKindDecisionTopic,
				DeletedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseTombstone(MarshalTombstone(tt.ts))
			if err != nil {
				t.Fatalf("ParseTombstone: %v", err)
			}
			if !reflect.DeepEqual(parsed, &memory.Tombstone{
				ID:        tt.ts.ID,
				Kind:      tt.ts.Kind,
				DeletedAt: parsed.DeletedAt,
			}) || !parsed.DeletedAt.Equal(tt.ts.DeletedAt) {
				t.Errorf("round-trip mismatch: got %+v, want %+v", parsed, tt.ts)
			}
		})
	}
}

func TestSanitizeTopic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"auth-strategy", "auth-strategy"},
		{"has spaces", "has-spaces"},
		{"has/slashes", "has-slashes"},
		{"", "unnamed"},
		{"---", "unnamed"},
		{strings.Repeat("a", 150), strings.Repeat("a", 100)},
	}
	for _, tt := range tests {
		if got := SanitizeTopic(tt.input); got != tt.want {
			t.Errorf("SanitizeTopic(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTopicName(t *testing.T) {
	a, b := TopicName("auth strategy"), TopicName("auth-strategy")
	if a == b {
		t.Errorf("distinct raw topics map to the same name: %q", a)
	}
	if !strings.HasPrefix(a, "auth-strategy-") || !strings.HasPrefix(b, "auth-strategy-") {
		t.Errorf("names should keep the sanitized prefix: %q, %q", a, b)
	}
	if TopicName("auth strategy") != a {
		t.Error("TopicName is not deterministic")
	}
}

// =============================================================================
// Export determinism
// =============================================================================

// Test 1: exporting the same store twice into fresh dirs must produce
// identical bytes everywhere except the manifest watermark, and re-exporting
// over an existing tree must not change any record file.
func TestExportDeterminism(t *testing.T) {
	s := newTestStore(t)

	t1 := time.Date(2026, 1, 10, 8, 0, 0, 111, time.UTC)
	t2 := time.Date(2026, 1, 11, 8, 0, 0, 222, time.UTC)
	seedDecision(t, s, "testing", "vitest", t1)
	seedDecision(t, s, "testing", "vitest + playwright", t2)
	seedMemory(t, s, "01ARZ3NDEKTSV4RRFFQ69G5FAV", "Auth middleware lives at src/auth.ts", []string{"project:myapp"}, t1, t2)
	if err := s.AddTombstone(&memory.Tombstone{
		ID:        "01OLDMEMORYAAAAAAAAAAAAAAA",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("AddTombstone: %v", err)
	}

	dir1 := filepath.Join(t.TempDir(), "context")
	dir2 := filepath.Join(t.TempDir(), "context")
	mustExport(t, s, dir1, ExportOptions{})
	mustExport(t, s, dir2, ExportOptions{})

	snap1 := treeSnapshot(t, dir1)
	snap2 := treeSnapshot(t, dir2)
	if !reflect.DeepEqual(snap1, snap2) {
		t.Errorf("fresh exports differ:\n%v\nvs\n%v", snap1, snap2)
	}
	if len(snap1) == 0 {
		t.Fatal("export produced no record files")
	}

	// Re-export over an existing tree: still byte-identical.
	mustExport(t, s, dir1, ExportOptions{})
	if again := treeSnapshot(t, dir1); !reflect.DeepEqual(again, snap2) {
		t.Errorf("re-export changed record files:\n%v\nvs\n%v", again, snap2)
	}

	// The manifest is present and is the only intended difference.
	if _, _, err := ReadManifest(dir1); err != nil {
		t.Errorf("ReadManifest: %v", err)
	}
}

func TestExportTombstoneGC(t *testing.T) {
	s := newTestStore(t)
	root := filepath.Join(t.TempDir(), "context")

	expired := &memory.Tombstone{
		ID:        "01EXPIREDAAAAAAAAAAAAAAAAA",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: time.Now().Add(-91 * 24 * time.Hour),
	}
	liveTS := &memory.Tombstone{
		ID:        "01LIVEAAAAAAAAAAAAAAAAAAAA",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: time.Now().Add(-time.Hour),
	}
	for _, ts := range []*memory.Tombstone{expired, liveTS} {
		if err := s.AddTombstone(ts); err != nil {
			t.Fatalf("AddTombstone: %v", err)
		}
	}

	// A stale tombstone file in the tree must be GC'd too.
	if err := os.MkdirAll(filepath.Join(root, "tombstones"), 0o755); err != nil {
		t.Fatal(err)
	}
	fileExpired := &memory.Tombstone{
		ID:        "01FILEEXPIREDAAAAAAAAAAAAA",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: time.Now().Add(-200 * 24 * time.Hour),
	}
	if err := os.WriteFile(TombstonePath(root, fileExpired), MarshalTombstone(fileExpired), 0o644); err != nil {
		t.Fatal(err)
	}

	stats := mustExport(t, s, root, ExportOptions{})
	if stats.Tombstones != 1 {
		t.Errorf("live tombstones: got %d, want 1", stats.Tombstones)
	}
	if _, err := os.Stat(TombstonePath(root, liveTS)); err != nil {
		t.Errorf("live tombstone file missing: %v", err)
	}
	for _, ts := range []*memory.Tombstone{expired, fileExpired} {
		if _, err := os.Stat(TombstonePath(root, ts)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("expired tombstone %s should be GC'd, stat err=%v", ts.ID, err)
		}
	}
	if _, err := s.GetTombstone(memory.TombstoneKindMemory, expired.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expired DB tombstone should be pruned, got err=%v", err)
	}
}

// =============================================================================
// Decision lineage
// =============================================================================

// Test 2: lineage survives the wire with original timestamps, and importing
// a stale publisher's tree (containing only v1) never un-supersedes v2.
func TestDecisionLineagePreservation(t *testing.T) {
	a := newTestStore(t)
	b := newTestStore(t)

	t1 := time.Date(2026, 1, 10, 8, 0, 0, 100, time.UTC)
	t2 := time.Date(2026, 2, 20, 9, 0, 0, 200, time.UTC)
	seedDecision(t, a, "testing", "v1: vitest", t1)
	seedDecision(t, a, "testing", "v2: vitest + playwright", t2)

	rootA := filepath.Join(t.TempDir(), "context")
	mustExport(t, a, rootA, ExportOptions{})

	stats := mustImport(t, b, rootA, ImportOptions{})
	if stats.DecisionsImported != 2 {
		t.Fatalf("imported %d decisions, want 2", stats.DecisionsImported)
	}

	history, err := b.GetDecisionHistory("testing")
	if err != nil {
		t.Fatalf("GetDecisionHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history length: got %d, want 2 (lineage must survive the wire)", len(history))
	}
	for _, d := range history {
		if !d.CreatedAt.Equal(t1) && !d.CreatedAt.Equal(t2) {
			t.Errorf("imported decision re-stamped: CreatedAt=%s, want %s or %s", d.CreatedAt, t1, t2)
		}
	}
	latest, err := b.GetDecision("testing")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if latest.Decision != "v2: vitest + playwright" {
		t.Errorf("latest: got %q, want v2", latest.Decision)
	}

	// Stale publisher: a tree containing only v1, imported after v2 exists.
	stale := newTestStore(t)
	seedDecision(t, stale, "testing", "v1: vitest", t1)
	rootStale := filepath.Join(t.TempDir(), "context")
	mustExport(t, stale, rootStale, ExportOptions{})

	stats = mustImport(t, b, rootStale, ImportOptions{})
	if stats.DecisionsImported != 0 || stats.DecisionsSkipped != 1 {
		t.Errorf("stale import counts: imported=%d skipped=%d, want 0/1", stats.DecisionsImported, stats.DecisionsSkipped)
	}
	latest, err = b.GetDecision("testing")
	if err != nil {
		t.Fatalf("GetDecision after stale import: %v", err)
	}
	if latest.Decision != "v2: vitest + playwright" {
		t.Errorf("stale import un-superseded v2: latest is %q", latest.Decision)
	}
}

// Distinct topics that sanitise to the same name ("auth strategy" vs
// "auth-strategy") must not collide on disk: both topics' records coexist in
// one export (even with identical CreatedAt, which would collide on the same
// <unixnano>.md in a shared directory), and both deletions propagate via
// distinct tombstone files instead of the write-once guard dropping one.
func TestDecisionTopicSanitizationCollision(t *testing.T) {
	const topicSpaced = "auth strategy"
	const topicDashed = "auth-strategy"

	a := newTestStore(t)
	b := newTestStore(t)

	t1 := time.Now().UTC().Truncate(time.Second).Add(-48 * time.Hour)
	seedDecision(t, a, topicSpaced, "decision for spaced topic", t1)
	seedDecision(t, a, topicDashed, "decision for dashed topic", t1)

	root := filepath.Join(t.TempDir(), "context")
	stats := mustExport(t, a, root, ExportOptions{})
	if stats.Decisions != 2 {
		t.Fatalf("exported %d decisions, want 2", stats.Decisions)
	}
	pathSpaced := DecisionPath(root, &memory.Decision{Topic: topicSpaced, CreatedAt: t1})
	pathDashed := DecisionPath(root, &memory.Decision{Topic: topicDashed, CreatedAt: t1})
	if pathSpaced == pathDashed {
		t.Fatalf("colliding decision record paths: %s", pathSpaced)
	}
	for _, p := range []string{pathSpaced, pathDashed} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing decision record %s: %v", p, err)
		}
	}

	// Both topics survive the wire with raw identity from frontmatter.
	mustImport(t, b, root, ImportOptions{})
	for topic, want := range map[string]string{
		topicSpaced: "decision for spaced topic",
		topicDashed: "decision for dashed topic",
	} {
		d, err := b.GetDecision(topic)
		if err != nil {
			t.Fatalf("GetDecision(%q): %v", topic, err)
		}
		if d.Decision != want {
			t.Errorf("GetDecision(%q) = %q, want %q", topic, d.Decision, want)
		}
	}

	// Delete both topics on A: both tombstones must materialise (distinct
	// files) and both deletions must propagate to B.
	for _, topic := range []string{topicSpaced, topicDashed} {
		if _, err := a.DeleteDecision(topic); err != nil {
			t.Fatalf("DeleteDecision(%q): %v", topic, err)
		}
	}
	mustExport(t, a, root, ExportOptions{})
	tsSpaced := TombstonePath(root, &memory.Tombstone{ID: topicSpaced, Kind: memory.TombstoneKindDecisionTopic})
	tsDashed := TombstonePath(root, &memory.Tombstone{ID: topicDashed, Kind: memory.TombstoneKindDecisionTopic})
	if tsSpaced == tsDashed {
		t.Fatalf("colliding tombstone paths: %s", tsSpaced)
	}
	for _, p := range []string{tsSpaced, tsDashed} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing tombstone file %s: %v", p, err)
		}
	}

	mustImport(t, b, root, ImportOptions{})
	for _, topic := range []string{topicSpaced, topicDashed} {
		if _, err := b.GetDecision(topic); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("topic %q should be deleted on B, err=%v", topic, err)
		}
	}
}

// =============================================================================
// Tombstones
// =============================================================================

// Test 3: A deletes memory M; the deletion propagates to B via A's export,
// and B's subsequent export must not bring M back to A.
func TestNoResurrection(t *testing.T) {
	a := newTestStore(t)
	b := newTestStore(t)

	created := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	seedMemory(t, a, id, "doomed memory", []string{"project:test"}, created, created)
	seedMemory(t, b, id, "doomed memory", []string{"project:test"}, created, created)

	if err := a.DeleteMemory(id); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	rootA := filepath.Join(t.TempDir(), "context")
	mustExport(t, a, rootA, ExportOptions{})
	if _, err := os.Stat(MemoryPath(rootA, id)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleted memory must not be exported, stat err=%v", err)
	}

	stats := mustImport(t, b, rootA, ImportOptions{})
	if stats.RecordsDeleted != 1 {
		t.Errorf("records deleted: got %d, want 1", stats.RecordsDeleted)
	}
	if stats.TombstonesRecorded != 1 {
		t.Errorf("tombstones recorded: got %d, want 1", stats.TombstonesRecorded)
	}
	if _, err := b.GetMemory(id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("memory should be deleted in B, got err=%v", err)
	}

	rootB := filepath.Join(t.TempDir(), "context")
	mustExport(t, b, rootB, ExportOptions{})
	if _, err := os.Stat(MemoryPath(rootB, id)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("B's export must not contain the deleted memory, stat err=%v", err)
	}

	mustImport(t, a, rootB, ImportOptions{})
	if _, err := a.GetMemory(id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("memory resurrected in A, got err=%v", err)
	}
}

// An incoming record older than a local tombstone must not be re-added even
// when the publisher never saw the deletion (their tree still has the file).
func TestImportBlockedByLocalTombstone(t *testing.T) {
	a := newTestStore(t)
	b := newTestStore(t)

	created := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	seedMemory(t, a, id, "deleted locally", []string{"project:test"}, created, created)
	seedMemory(t, b, id, "deleted locally", []string{"project:test"}, created, created)

	// B publishes while still holding the record; A deletes it locally.
	rootB := filepath.Join(t.TempDir(), "context")
	mustExport(t, b, rootB, ExportOptions{})
	if err := a.DeleteMemory(id); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	stats := mustImport(t, a, rootB, ImportOptions{})
	if stats.MemoriesImported != 0 {
		t.Errorf("imported %d memories, want 0", stats.MemoriesImported)
	}
	if _, err := a.GetMemory(id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("locally deleted memory resurrected by peer import, err=%v", err)
	}
}

// Tombstones older than the TTL are ignored for deletion but are not errors.
func TestImportIgnoresExpiredTombstone(t *testing.T) {
	b := newTestStore(t)
	created := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	seedMemory(t, b, id, "survivor", []string{"project:test"}, created, created)

	root := filepath.Join(t.TempDir(), "context")
	if err := os.MkdirAll(filepath.Join(root, "tombstones"), 0o755); err != nil {
		t.Fatal(err)
	}
	expired := &memory.Tombstone{ID: id, Kind: memory.TombstoneKindMemory, DeletedAt: time.Now().Add(-91 * 24 * time.Hour)}
	if err := os.WriteFile(TombstonePath(root, expired), MarshalTombstone(expired), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(root, time.Now()); err != nil {
		t.Fatal(err)
	}

	stats := mustImport(t, b, root, ImportOptions{})
	if stats.RecordsDeleted != 0 || stats.TombstonesRecorded != 0 || stats.TombstonesIgnored != 1 {
		t.Errorf("counts: deleted=%d recorded=%d ignored=%d, want 0/0/1",
			stats.RecordsDeleted, stats.TombstonesRecorded, stats.TombstonesIgnored)
	}
	if _, err := b.GetMemory(id); err != nil {
		t.Errorf("memory should survive an expired tombstone: %v", err)
	}
}

// =============================================================================
// Monotonic forget
// =============================================================================

func TestMergeForgetTag(t *testing.T) {
	tests := []struct {
		name     string
		local    []string
		incoming []string
		want     []string
	}{
		{
			name:     "local forget survives newer incoming without it",
			local:    []string{"project:test", "forget"},
			incoming: []string{"project:test"},
			want:     []string{"project:test", "forget"},
		},
		{
			name:     "both have forget",
			local:    []string{"forget"},
			incoming: []string{"project:test", "forget"},
			want:     []string{"project:test", "forget"},
		},
		{
			name:     "neither has forget",
			local:    []string{"project:test"},
			incoming: []string{"project:test", "extra"},
			want:     []string{"project:test", "extra"},
		},
		{
			name:     "incoming-only forget is kept",
			local:    []string{"project:test"},
			incoming: []string{"forget"},
			want:     []string{"forget"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeForgetTag(tt.local, tt.incoming)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeForgetTag(%v, %v) = %v, want %v", tt.local, tt.incoming, got, tt.want)
			}
		})
	}
}

// Test 4: a newer incoming version without the forget tag must not strip a
// local forget — soft-deletion is monotonic.
func TestImportKeepsLocalForgetOnNewerIncoming(t *testing.T) {
	a := newTestStore(t)
	b := newTestStore(t)

	created := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	edited := created.Add(48 * time.Hour)
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	// A edits the memory after B soft-deleted its copy.
	seedMemory(t, a, id, "newer content from teammate", []string{"project:test"}, created, edited)
	seedMemory(t, b, id, "original content", []string{"project:test", "forget"}, created, created)

	rootA := filepath.Join(t.TempDir(), "context")
	mustExport(t, a, rootA, ExportOptions{})

	stats := mustImport(t, b, rootA, ImportOptions{})
	if stats.MemoriesImported != 1 {
		t.Fatalf("imported %d memories, want 1 (newer incoming should update)", stats.MemoriesImported)
	}

	got, err := b.GetMemory(id)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "newer content from teammate" {
		t.Errorf("content: got %q, want the newer incoming content", got.Content)
	}
	if !slices.Contains(got.Tags, "forget") {
		t.Errorf("forget tag stripped by import: tags=%v", got.Tags)
	}
}

// A forget-tagged memory is context-export data, not an exclusion: the record
// is written with its forget tag and newer UpdatedAt, and a peer that never
// had the memory imports it directly in the forgotten state.
func TestForgetTaggedMemoryPropagates(t *testing.T) {
	a := newTestStore(t)
	b := newTestStore(t)

	created := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	forgotten := created.Add(24 * time.Hour)
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	seedMemory(t, a, id, "rule of thumb that aged badly", []string{"project:test", "forget"}, created, forgotten)

	rootA := filepath.Join(t.TempDir(), "context")
	mustExport(t, a, rootA, ExportOptions{})

	data, err := os.ReadFile(MemoryPath(rootA, id))
	if err != nil {
		t.Fatalf("forget-tagged memory must still be exported: %v", err)
	}
	rec, err := ParseMemory(data)
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	if !slices.Contains(rec.Tags, "forget") {
		t.Fatalf("exported record lost the forget tag: tags=%v", rec.Tags)
	}
	if !rec.UpdatedAt.Equal(forgotten) {
		t.Errorf("exported UpdatedAt: got %s, want %s", rec.UpdatedAt, forgotten)
	}

	stats := mustImport(t, b, rootA, ImportOptions{})
	if stats.MemoriesImported != 1 {
		t.Fatalf("imported %d memories, want 1", stats.MemoriesImported)
	}
	got, err := b.GetMemory(id)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if !slices.Contains(got.Tags, "forget") {
		t.Errorf("peer imported the memory without its forget tag: tags=%v", got.Tags)
	}

	visible, err := b.ListMemories(memory.SearchOptions{})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	for _, m := range visible {
		if m.ID == id {
			t.Error("forgotten memory must stay hidden from default listings on the peer")
		}
	}
}

// =============================================================================
// Convergence
// =============================================================================

// Test 5: two clones with overlapping and disjoint records reach the same
// state whichever direction syncs first.
func TestTwoCloneConvergence(t *testing.T) {
	// Recent (within the tombstone TTL) but fixed for both exchange orders.
	tBase := time.Now().UTC().Truncate(time.Second).Add(-72 * time.Hour)

	seedA := func(t *testing.T, s *store.BoltStore) {
		seedDecision(t, s, "shared-topic", "v1 from A", tBase)
		seedMemory(t, s, "01AONLYAAAAAAAAAAAAAAAAAAA", "memory only in A", []string{"project:test"}, tBase, tBase)
		seedMemory(t, s, "01SHAREDAAAAAAAAAAAAAAAAAA", "shared, A's older edit", []string{"project:test"}, tBase, tBase.Add(time.Hour))
		// A soft-deleted (forget-tagged) a memory both clones share; the
		// forget edit is newer than B's copy and must propagate as data.
		seedMemory(t, s, "01FSHAREDAAAAAAAAAAAAAAAAA", "soft-deleted on A", []string{"project:test", "forget"}, tBase, tBase.Add(4*time.Hour))
		// A soft-deleted a memory B never had; B must still converge to the
		// forgotten state rather than never learning about the record.
		seedMemory(t, s, "01FONLYAAAAAAAAAAAAAAAAAAA", "forgotten, only ever on A", []string{"project:test", "forget"}, tBase, tBase.Add(time.Hour))
		// A deleted a record both clones used to have.
		seedMemory(t, s, "01DELETEDAAAAAAAAAAAAAAAAA", "deleted on A", []string{"project:test"}, tBase, tBase)
		if err := s.DeleteMemory("01DELETEDAAAAAAAAAAAAAAAAA"); err != nil {
			t.Fatal(err)
		}
		// Pin the tombstone time so both convergence runs are comparable.
		if err := s.AddTombstone(&memory.Tombstone{
			ID:        "01DELETEDAAAAAAAAAAAAAAAAA",
			Kind:      memory.TombstoneKindMemory,
			DeletedAt: tBase.Add(2 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedB := func(t *testing.T, s *store.BoltStore) {
		seedDecision(t, s, "shared-topic", "v2 from B", tBase.Add(24*time.Hour))
		seedDecision(t, s, "b-topic", "only B decided this", tBase)
		seedMemory(t, s, "01BONLYAAAAAAAAAAAAAAAAAAA", "memory only in B", []string{"project:test"}, tBase, tBase)
		seedMemory(t, s, "01SHAREDAAAAAAAAAAAAAAAAAA", "shared, B's newer edit", []string{"project:test"}, tBase, tBase.Add(2*time.Hour))
		seedMemory(t, s, "01FSHAREDAAAAAAAAAAAAAAAAA", "soft-deleted on A", []string{"project:test"}, tBase, tBase)
		seedMemory(t, s, "01DELETEDAAAAAAAAAAAAAAAAA", "deleted on A", []string{"project:test"}, tBase, tBase)
	}

	sync := func(t *testing.T, from, to *store.BoltStore) {
		root := filepath.Join(t.TempDir(), "context")
		mustExport(t, from, root, ExportOptions{})
		mustImport(t, to, root, ImportOptions{})
	}

	// Order 1: A→B, then B→A.
	a1, b1 := newTestStore(t), newTestStore(t)
	seedA(t, a1)
	seedB(t, b1)
	sync(t, a1, b1)
	sync(t, b1, a1)

	// Order 2 from fresh copies: B→A, then A→B.
	a2, b2 := newTestStore(t), newTestStore(t)
	seedA(t, a2)
	seedB(t, b2)
	sync(t, b2, a2)
	sync(t, a2, b2)

	snapA1, snapB1 := storeSnapshot(t, a1), storeSnapshot(t, b1)
	snapA2, snapB2 := storeSnapshot(t, a2), storeSnapshot(t, b2)

	if !reflect.DeepEqual(snapA1, snapB1) {
		t.Errorf("order 1 did not converge:\nA=%v\nB=%v", snapA1, snapB1)
	}
	if !reflect.DeepEqual(snapA2, snapB2) {
		t.Errorf("order 2 did not converge:\nA=%v\nB=%v", snapA2, snapB2)
	}
	if !reflect.DeepEqual(snapA1, snapA2) {
		t.Errorf("exchange order changed the final state:\norder1=%v\norder2=%v", snapA1, snapA2)
	}

	// Spot-check the merged content.
	got, err := a1.GetMemory("01SHAREDAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "shared, B's newer edit" {
		t.Errorf("LWW winner: got %q, want B's newer edit", got.Content)
	}
	latest, err := b1.GetDecision("shared-topic")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if latest.Decision != "v2 from B" {
		t.Errorf("latest decision: got %q, want v2 from B", latest.Decision)
	}
	if _, err := b1.GetMemory("01DELETEDAAAAAAAAAAAAAAAAA"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deletion did not propagate to B, err=%v", err)
	}
	for _, id := range []string{"01FSHAREDAAAAAAAAAAAAAAAAA", "01FONLYAAAAAAAAAAAAAAAAAAA"} {
		got, err := b1.GetMemory(id)
		if err != nil {
			t.Fatalf("GetMemory(%s): %v (forgotten records must still converge)", id, err)
		}
		if !slices.Contains(got.Tags, "forget") {
			t.Errorf("memory %s lost its forget tag on B: tags=%v", id, got.Tags)
		}
	}
}

// =============================================================================
// Stale-export guard
// =============================================================================

// Test 7: imports refuse missing or stale manifests unless forced.
func TestStaleExportGuard(t *testing.T) {
	tests := []struct {
		name      string
		manifest  func(root string) error
		force     bool
		wantStale bool
	}{
		{
			name:      "missing manifest refused",
			manifest:  func(string) error { return nil },
			wantStale: true,
		},
		{
			name:      "missing manifest forced",
			manifest:  func(string) error { return nil },
			force:     true,
			wantStale: false,
		},
		{
			name:      "stale watermark refused",
			manifest:  func(root string) error { return WriteManifest(root, time.Now().Add(-91*24*time.Hour)) },
			wantStale: true,
		},
		{
			name:      "stale watermark forced",
			manifest:  func(root string) error { return WriteManifest(root, time.Now().Add(-91*24*time.Hour)) },
			force:     true,
			wantStale: false,
		},
		{
			name:      "fresh watermark accepted",
			manifest:  func(root string) error { return WriteManifest(root, time.Now()) },
			wantStale: false,
		},
		{
			name: "malformed manifest refused",
			manifest: func(root string) error {
				return os.WriteFile(filepath.Join(root, ManifestName), []byte("not json"), 0o644)
			},
			wantStale: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			root := t.TempDir()
			if err := tt.manifest(root); err != nil {
				t.Fatalf("manifest setup: %v", err)
			}

			_, err := Import(s, s, root, ImportOptions{Force: tt.force})
			if tt.wantStale {
				if !errors.Is(err, ErrStaleExport) {
					t.Errorf("expected ErrStaleExport, got %v", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// Seed helpers
// =============================================================================

func seedDecision(t *testing.T, s *store.BoltStore, topic, decision string, createdAt time.Time) {
	t.Helper()
	if err := s.SetDecision(&memory.Decision{
		Topic:     topic,
		Decision:  decision,
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("SetDecision: %v", err)
	}
}

func seedMemory(t *testing.T, s *store.BoltStore, id, content string, tags []string, createdAt, updatedAt time.Time) {
	t.Helper()
	if err := s.AddMemory(&memory.Memory{
		ID:        id,
		Category:  memory.CategoryLearning,
		Content:   content,
		Tags:      tags,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
}
