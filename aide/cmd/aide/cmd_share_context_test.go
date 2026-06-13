package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestIsContextLayout(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  bool
	}{
		{
			name: "manifest marks context layout",
			setup: func(t *testing.T, dir string) {
				if err := contextshare.WriteManifest(dir, time.Now()); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "tombstones dir marks context layout",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "tombstones"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "decision topic subdirs mark context layout",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "decisions", "auth-strategy"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "legacy flat decisions dir is not context layout",
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
			name:  "empty dir is not context layout",
			setup: func(t *testing.T, dir string) {},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			if got := isContextLayout(dir); got != tt.want {
				t.Errorf("isContextLayout = %v, want %v", got, tt.want)
			}
		})
	}
}

// The share command default path: export writes the context layout, and an
// import on a second clone reproduces decisions (with original timestamps)
// and memories.
func TestCmdShareContextExportImportRoundTrip(t *testing.T) {
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
	contextDir := filepath.Join(tmpA, ".aide", "context")
	if _, err := os.Stat(filepath.Join(contextDir, contextshare.ManifestName)); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	versionFile := contextshare.DecisionPath(contextDir, &memory.Decision{Topic: "auth", CreatedAt: t1})
	if _, err := os.Stat(versionFile); err != nil {
		t.Errorf("decision version file not written: %v", err)
	}

	// Publish A's tree into B's default import location.
	if err := os.MkdirAll(filepath.Join(tmpB, ".aide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(tmpB, ".aide", "context"), os.DirFS(contextDir)); err != nil {
		t.Fatalf("copy context tree: %v", err)
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
	contextDir := filepath.Join(tmpB, ".aide", "context")
	if err := os.MkdirAll(filepath.Join(contextDir, "tombstones"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contextshare.WriteManifest(contextDir, time.Now().Add(-120*24*time.Hour)); err != nil {
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

// An existing-but-empty (or manifest-less) context directory must hit the
// context importer's stale guard instead of silently falling through to a
// legacy import of nothing. Same for an explicit --input directory that
// holds no legacy records.
func TestCmdShareImportEmptyContextDirIsNotSilent(t *testing.T) {
	t.Run("default empty .aide/context", func(t *testing.T) {
		dbB, tmpB := newShareProject(t)
		if err := os.MkdirAll(filepath.Join(tmpB, ".aide", "context"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := cmdShareImport(dbB, nil)
		if err == nil || !strings.Contains(err.Error(), "stale context export") {
			t.Errorf("want stale-export error for empty context dir, got: %v", err)
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
