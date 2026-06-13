package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// newShareProject creates a project-shaped temp dir and returns the db path.
// Unlike setupShareTest it does not hold a backend open, so the cmd handlers
// (which open their own backend) can run without contending for the bolt lock.
func newShareProject(t *testing.T) (dbPath, tmpDir string) {
	t.Helper()
	tmpDir = t.TempDir()
	memDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(memDir, "memory.db"), tmpDir
}

// withBackend runs fn against a backend that is closed before returning.
func withBackend(t *testing.T, dbPath string, fn func(b *Backend)) {
	t.Helper()
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	defer b.Close()
	fn(b)
}

func TestHasPerRecordLayout(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  bool
	}{
		{
			name: "manifest marks per-record layout",
			setup: func(t *testing.T, dir string) {
				if err := contextshare.WriteManifest(dir, time.Now()); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "tombstones dir marks per-record layout",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "tombstones"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "decision topic subdirs mark per-record layout",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "decisions", "auth-strategy"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "legacy flat decisions dir is not per-record layout",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "decisions"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "decisions", "auth.md"), []byte("---\ntopic: auth\n---\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name:  "empty dir is not per-record layout",
			setup: func(t *testing.T, dir string) {},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			if got := hasPerRecordLayout(dir); got != tt.want {
				t.Errorf("hasPerRecordLayout = %v, want %v", got, tt.want)
			}
		})
	}
}

// withMemorySharingOn enables memory export+import for the duration of a test,
// restoring the previous config afterwards. The default policy keeps memories
// local, so any round-trip that asserts on memories must opt in explicitly.
func withMemorySharingOn(t *testing.T) {
	t.Helper()
	prev := config.Get()
	yes := true
	cfg := *prev
	cfg.Share.Memories.Export = &yes
	cfg.Share.Memories.Import = &yes
	config.Set(&cfg)
	t.Cleanup(func() { config.Set(prev) })
}

// The share command default path: export writes the per-record layout, and an
// import on a second clone reproduces decisions (with original timestamps)
// and memories.
func TestCmdShareContextExportImportRoundTrip(t *testing.T) {
	withMemorySharingOn(t)
	dbA, tmpA := newShareProject(t)
	dbB, tmpB := newShareProject(t)

	t1 := time.Date(2026, 1, 10, 8, 0, 0, 100, time.UTC)
	t2 := time.Date(2026, 2, 20, 9, 0, 0, 200, time.UTC)
	withBackend(t, dbA, func(b *Backend) {
		for _, d := range []*memory.Decision{
			{Topic: "auth", Decision: "JWT", Rationale: "stateless", CreatedAt: t1},
			{Topic: "auth", Decision: "JWT v2", Rationale: "rotation", CreatedAt: t2},
		} {
			if err := b.Store().SetDecision(d); err != nil {
				t.Fatalf("SetDecision: %v", err)
			}
		}
		if err := b.Store().AddMemory(&memory.Memory{
			ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			Category:  "pattern",
			Content:   "Use table-driven tests",
			Tags:      []string{"project:test"},
			CreatedAt: t1,
		}); err != nil {
			t.Fatalf("AddMemory: %v", err)
		}
	})

	if err := cmdShareExport(dbA, nil); err != nil {
		t.Fatalf("cmdShareExport: %v", err)
	}
	sharedDir := filepath.Join(tmpA, ".aide", "shared")
	if _, err := os.Stat(filepath.Join(sharedDir, contextshare.ManifestName)); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	versionFile := contextshare.DecisionPath(sharedDir, &memory.Decision{Topic: "auth", CreatedAt: t1})
	if _, err := os.Stat(versionFile); err != nil {
		t.Errorf("decision version file not written: %v", err)
	}

	// Publish A's tree into B's default import location.
	if err := os.MkdirAll(filepath.Join(tmpB, ".aide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(tmpB, ".aide", "shared"), os.DirFS(sharedDir)); err != nil {
		t.Fatalf("copy shared tree: %v", err)
	}

	if err := cmdShareImport(dbB, nil); err != nil {
		t.Fatalf("cmdShareImport: %v", err)
	}

	withBackend(t, dbB, func(b *Backend) {
		history, err := b.GetDecisionHistory("auth")
		if err != nil {
			t.Fatalf("GetDecisionHistory: %v", err)
		}
		if len(history) != 2 {
			t.Fatalf("history length: got %d, want 2", len(history))
		}
		latest, err := b.GetDecision("auth")
		if err != nil {
			t.Fatalf("GetDecision: %v", err)
		}
		if latest.Decision != "JWT v2" || !latest.CreatedAt.Equal(t2) {
			t.Errorf("latest: got %q@%s, want JWT v2@%s", latest.Decision, latest.CreatedAt, t2)
		}
		mem, err := b.GetMemory("01ARZ3NDEKTSV4RRFFQ69G5FAV")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if mem.Content != "Use table-driven tests" || !mem.CreatedAt.Equal(t1) {
			t.Errorf("memory round-trip: got %q@%s", mem.Content, mem.CreatedAt)
		}
	})
}

// Importing a context tree with a missing or stale manifest must fail with a
// clear error unless --force is given.
func TestCmdShareImportStaleGuard(t *testing.T) {
	dbB, tmpB := newShareProject(t)
	sharedDir := filepath.Join(tmpB, ".aide", "shared")
	if err := os.MkdirAll(filepath.Join(sharedDir, "tombstones"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contextshare.WriteManifest(sharedDir, time.Now().Add(-120*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	err := cmdShareImport(dbB, nil)
	if err == nil {
		t.Fatal("expected stale-export error, got nil")
	}
	if !strings.Contains(err.Error(), "stale context export") {
		t.Errorf("error should mention staleness, got: %v", err)
	}

	if err := cmdShareImport(dbB, []string{"--force"}); err != nil {
		t.Errorf("--force should bypass the stale guard, got: %v", err)
	}
}

// An existing-but-empty (or manifest-less) shared directory must hit the
// per-record importer's stale guard instead of silently falling through to a
// legacy import of nothing. Same for an explicit --input directory that
// holds no legacy records.
func TestCmdShareImportEmptyContextDirIsNotSilent(t *testing.T) {
	t.Run("default empty .aide/shared", func(t *testing.T) {
		dbB, tmpB := newShareProject(t)
		if err := os.MkdirAll(filepath.Join(tmpB, ".aide", "shared"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := cmdShareImport(dbB, nil)
		if err == nil || !strings.Contains(err.Error(), "stale context export") {
			t.Errorf("want stale-export error for empty shared dir, got: %v", err)
		}
	})

	t.Run("explicit --input empty dir", func(t *testing.T) {
		dbB, _ := newShareProject(t)
		err := cmdShareImport(dbB, []string{"--input=" + t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "stale context export") {
			t.Errorf("want stale-export error for empty --input dir, got: %v", err)
		}
	})
}

// An explicit --input pointing at a legacy aggregate tree must still route
// to the legacy importer.
func TestCmdShareImportExplicitLegacyInput(t *testing.T) {
	dbB, _ := newShareProject(t)
	legacyDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacyDir, "decisions"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := "---\ntopic: auth\ndecision: JWT\n---\n\n# auth\n\n**Decision:** JWT\n"
	if err := os.WriteFile(filepath.Join(legacyDir, "decisions", "auth.md"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdShareImport(dbB, []string{"--input=" + legacyDir}); err != nil {
		t.Fatalf("cmdShareImport legacy: %v", err)
	}
	withBackend(t, dbB, func(b *Backend) {
		d, err := b.GetDecision("auth")
		if err != nil {
			t.Fatalf("GetDecision: %v", err)
		}
		if d.Decision != "JWT" {
			t.Errorf("decision: got %q, want JWT", d.Decision)
		}
	})
}

// The first per-record export migrates legacy aggregate files: it imports them
// into the store, deletes the flat aggregates, and re-materialises them as
// per-record files. A re-import yields the same records (nothing lost).
func TestCmdShareExportMigratesLegacyAggregates(t *testing.T) {
	withMemorySharingOn(t)
	dbA, tmpA := newShareProject(t)

	sharedDir := filepath.Join(tmpA, ".aide", "shared")
	legacyDecision := filepath.Join(sharedDir, "decisions", "auth.md")
	legacyMemory := filepath.Join(sharedDir, "memories", "pattern.md")
	if err := os.MkdirAll(filepath.Dir(legacyDecision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyMemory), 0o755); err != nil {
		t.Fatal(err)
	}

	created := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := writeDecisionMarkdown(legacyDecision, &memory.Decision{
		Topic: "auth", Decision: "JWT", Rationale: "stateless", CreatedAt: created,
	}); err != nil {
		t.Fatalf("writeDecisionMarkdown: %v", err)
	}
	if err := writeMemoriesMarkdown(legacyMemory, "pattern", []*memory.Memory{{
		ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Category: "pattern",
		Content: "Use table-driven tests", Tags: []string{"project:test"}, CreatedAt: created,
	}}); err != nil {
		t.Fatalf("writeMemoriesMarkdown: %v", err)
	}

	if err := cmdShareExport(dbA, nil); err != nil {
		t.Fatalf("cmdShareExport: %v", err)
	}

	// Legacy aggregate files are gone.
	if _, err := os.Stat(legacyDecision); !os.IsNotExist(err) {
		t.Errorf("legacy decision file should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(legacyMemory); !os.IsNotExist(err) {
		t.Errorf("legacy memory file should have been removed, stat err = %v", err)
	}

	// Per-record files are present. The migrated decision's CreatedAt is
	// re-stamped by the legacy import path (a known M0 defect, not in scope
	// here), so assert on the topic directory rather than a fixed timestamp.
	if _, err := os.Stat(filepath.Join(sharedDir, contextshare.ManifestName)); err != nil {
		t.Errorf("manifest not written: %v", err)
	}
	topicDir := filepath.Join(sharedDir, "decisions", contextshare.TopicName("auth"))
	if entries, err := os.ReadDir(topicDir); err != nil || len(entries) == 0 {
		t.Errorf("per-record decision dir empty/missing: %v (%d entries)", err, len(entries))
	}
	if _, err := os.Stat(contextshare.MemoryPath(sharedDir, "01ARZ3NDEKTSV4RRFFQ69G5FAV")); err != nil {
		t.Errorf("per-record memory file not written: %v", err)
	}

	// Records survived the migration: a fresh clone importing the per-record
	// tree reconstructs the same decision and memory.
	dbB, tmpB := newShareProject(t)
	if err := os.MkdirAll(filepath.Join(tmpB, ".aide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(tmpB, ".aide", "shared"), os.DirFS(sharedDir)); err != nil {
		t.Fatalf("copy shared tree: %v", err)
	}
	if err := cmdShareImport(dbB, nil); err != nil {
		t.Fatalf("cmdShareImport: %v", err)
	}
	withBackend(t, dbB, func(b *Backend) {
		d, err := b.GetDecision("auth")
		if err != nil || d.Decision != "JWT" {
			t.Errorf("migrated decision: got %+v, err %v", d, err)
		}
		m, err := b.GetMemory("01ARZ3NDEKTSV4RRFFQ69G5FAV")
		if err != nil || m.Content != "Use table-driven tests" {
			t.Errorf("migrated memory: got %+v, err %v", m, err)
		}
	})

	// Idempotent: a second export with no legacy files left is a quiet no-op for
	// the migration step (no error, files stay per-record).
	if err := cmdShareExport(dbA, nil); err != nil {
		t.Fatalf("second cmdShareExport: %v", err)
	}
}

// With default policy, memory export/import is a no-op (nothing written or
// ingested) while decisions flow. Enabling memories.export with the default
// filter ships a project:* memory but excludes scope:global and session:*.
func TestCmdShareDefaultPolicyMemoriesNoOp(t *testing.T) {
	config.Set(&config.Config{}) // explicit defaults: memories off both ways
	t.Cleanup(func() { config.Set(&config.Config{}) })

	dbA, tmpA := newShareProject(t)
	created := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	withBackend(t, dbA, func(b *Backend) {
		if err := b.Store().SetDecision(&memory.Decision{
			Topic: "db", Decision: "postgres", CreatedAt: created,
		}); err != nil {
			t.Fatalf("SetDecision: %v", err)
		}
		if err := b.Store().AddMemory(&memory.Memory{
			ID: "01PROJECTAAAAAAAAAAAAAAAAA", Category: "pattern",
			Content: "project memory", Tags: []string{"project:foo"}, CreatedAt: created,
		}); err != nil {
			t.Fatalf("AddMemory: %v", err)
		}
	})

	if err := cmdShareExport(dbA, nil); err != nil {
		t.Fatalf("cmdShareExport: %v", err)
	}
	sharedDir := filepath.Join(tmpA, ".aide", "shared")

	// Decisions flowed.
	if _, err := os.Stat(contextshare.DecisionPath(sharedDir, &memory.Decision{Topic: "db", CreatedAt: created})); err != nil {
		t.Errorf("decision should have been exported under default policy: %v", err)
	}
	// Memories did not — no memory files written.
	if _, err := os.Stat(contextshare.MemoryPath(sharedDir, "01PROJECTAAAAAAAAAAAAAAAAA")); !os.IsNotExist(err) {
		t.Errorf("memory must NOT export under default policy, stat err = %v", err)
	}

	// And on the import side: a clone with default policy ingests the decision
	// but not the memory, even though a memory file is present in the tree.
	dbB, tmpB := newShareProject(t)
	if err := os.MkdirAll(filepath.Join(tmpB, ".aide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(tmpB, ".aide", "shared"), os.DirFS(sharedDir)); err != nil {
		t.Fatalf("copy shared tree: %v", err)
	}
	// Hand-place a memory file so the import-side no-op is exercised directly.
	memFile := contextshare.MemoryPath(filepath.Join(tmpB, ".aide", "shared"), "01IMPORTMEAAAAAAAAAAAAAAAA")
	if err := os.WriteFile(memFile, contextshare.MarshalMemory(&memory.Memory{
		ID: "01IMPORTMEAAAAAAAAAAAAAAAA", Category: "pattern",
		Content: "incoming", Tags: []string{"project:foo"}, CreatedAt: created,
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdShareImport(dbB, nil); err != nil {
		t.Fatalf("cmdShareImport: %v", err)
	}
	withBackend(t, dbB, func(b *Backend) {
		if _, err := b.GetDecision("db"); err != nil {
			t.Errorf("decision should import under default policy: %v", err)
		}
		if m, err := b.GetMemory("01IMPORTMEAAAAAAAAAAAAAAAA"); err == nil && m != nil {
			t.Errorf("memory must NOT import under default policy, got %+v", m)
		}
	})
}

// Enabling memories.export with the default filter ships a project:* memory but
// excludes a scope:global and a session:* memory.
func TestCmdShareMemoryExportDefaultFilter(t *testing.T) {
	yes := true
	config.Set(&config.Config{Share: config.ShareConfig{
		Memories: config.ShareTypePolicy{Export: &yes},
	}})
	t.Cleanup(func() { config.Set(&config.Config{}) })

	dbA, tmpA := newShareProject(t)
	created := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	withBackend(t, dbA, func(b *Backend) {
		for _, m := range []*memory.Memory{
			{ID: "01PROJECTAAAAAAAAAAAAAAAAA", Category: "pattern", Content: "ship me", Tags: []string{"project:foo"}, CreatedAt: created},
			{ID: "01GLOBALAAAAAAAAAAAAAAAAAA", Category: "pattern", Content: "global", Tags: []string{"scope:global"}, CreatedAt: created},
			{ID: "01SESSIONAAAAAAAAAAAAAAAAA", Category: "learning", Content: "ephemeral", Tags: []string{"session:x"}, CreatedAt: created},
		} {
			if err := b.Store().AddMemory(m); err != nil {
				t.Fatalf("AddMemory: %v", err)
			}
		}
	})

	if err := cmdShareExport(dbA, nil); err != nil {
		t.Fatalf("cmdShareExport: %v", err)
	}
	sharedDir := filepath.Join(tmpA, ".aide", "shared")

	if _, err := os.Stat(contextshare.MemoryPath(sharedDir, "01PROJECTAAAAAAAAAAAAAAAAA")); err != nil {
		t.Errorf("project:* memory should export: %v", err)
	}
	if _, err := os.Stat(contextshare.MemoryPath(sharedDir, "01GLOBALAAAAAAAAAAAAAAAAAA")); !os.IsNotExist(err) {
		t.Errorf("scope:global memory must be excluded by default filter, stat err = %v", err)
	}
	if _, err := os.Stat(contextshare.MemoryPath(sharedDir, "01SESSIONAAAAAAAAAAAAAAAAA")); !os.IsNotExist(err) {
		t.Errorf("session:* memory must be excluded by default filter, stat err = %v", err)
	}
}
