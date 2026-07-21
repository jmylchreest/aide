package survey

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDiscoverSubprojects pins the downward estate map: nested VCS roots
// and submodule checkouts surface as KindSubproject entries with identity
// and evidence; children's internals are not descended into; a stray
// .aide-only directory is NOT a subproject (no VCS evidence).
func TestDiscoverSubprojects(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite := func(p, content string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	root := filepath.Join(tmp, "super")
	mustMkdir(filepath.Join(root, ".git", "modules", "lib"))

	// Submodule checkout.
	sub := filepath.Join(root, "components", "lib")
	mustMkdir(filepath.Join(sub, ".aide"))
	mustWrite(filepath.Join(sub, ".git"),
		"gitdir: "+filepath.Join(root, ".git", "modules", "lib")+"\n")

	// Nested plain repo, with a deeper repo inside it that must NOT appear
	// (children's internals belong to their own surveys).
	nested := filepath.Join(root, "services", "api")
	mustMkdir(filepath.Join(nested, ".git"))
	mustMkdir(filepath.Join(nested, "inner", ".git"))

	// Stray .aide-only dir: no VCS evidence, not a subproject.
	mustMkdir(filepath.Join(root, "stray", ".aide"))

	entries := discoverSubprojects(root)

	byPath := map[string]*Entry{}
	for _, e := range entries {
		if e.Kind != KindSubproject {
			t.Errorf("unexpected kind %q", e.Kind)
		}
		byPath[e.FilePath] = e
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 subprojects, got %d: %v", len(entries), byPath)
	}

	subEntry := byPath[filepath.Join("components", "lib")]
	if subEntry == nil {
		t.Fatal("submodule checkout missing from subprojects")
	}
	if subEntry.Metadata["evidence"] != "submodule-gitdir" {
		t.Errorf("submodule evidence = %q", subEntry.Metadata["evidence"])
	}
	if subEntry.Metadata["has_aide_store"] != "true" {
		t.Errorf("submodule has_aide_store = %q, want true", subEntry.Metadata["has_aide_store"])
	}

	nestedEntry := byPath[filepath.Join("services", "api")]
	if nestedEntry == nil {
		t.Fatal("nested repo missing from subprojects")
	}
	if nestedEntry.Metadata["evidence"] != "nested-vcs-root" {
		t.Errorf("nested evidence = %q", nestedEntry.Metadata["evidence"])
	}
	if nestedEntry.Metadata["has_aide_store"] != "false" {
		t.Errorf("nested has_aide_store = %q, want false", nestedEntry.Metadata["has_aide_store"])
	}

	if _, ok := byPath[filepath.Join("services", "api", "inner")]; ok {
		t.Error("descended into a child scope's internals")
	}
	if _, ok := byPath["stray"]; ok {
		t.Error("stray .aide-only dir surfaced as a subproject")
	}
}
