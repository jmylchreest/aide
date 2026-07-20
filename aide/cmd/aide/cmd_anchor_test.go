package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// anchorFixture builds a filesystem estate covering the gitdir shapes the
// resolver must classify:
//
//	outer/                     plain repo (.git dir)
//	  super/                   plain repo (.git dir) + .aide/  — nested inside outer
//	    vendor/lib/            submodule of super (.git file -> super/.git/modules/lib)
//	    nested/                independent plain repo (git init inside super)
//	  stray/                   .aide/ only, no VCS  — must never join a chain
//	wt/                        linked worktree of super (.git file -> super/.git/worktrees/wt)
//	bare/                      no markers at all
func anchorFixture(t *testing.T) (tmp, outer, super, submodule, nested, worktree, bare string) {
	t.Helper()
	var err error
	tmp, err = filepath.EvalSymlinks(t.TempDir())
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

	outer = filepath.Join(tmp, "outer")
	mustMkdir(filepath.Join(outer, ".git"))

	super = filepath.Join(outer, "super")
	mustMkdir(filepath.Join(super, ".git", "modules", "lib"))
	mustMkdir(filepath.Join(super, ".git", "worktrees", "wt"))
	mustMkdir(filepath.Join(super, ".aide"))

	submodule = filepath.Join(super, "vendor", "lib")
	mustMkdir(filepath.Join(submodule, "src"))
	mustWrite(filepath.Join(submodule, ".git"),
		"gitdir: "+filepath.Join(super, ".git", "modules", "lib")+"\n")

	nested = filepath.Join(super, "nested")
	mustMkdir(filepath.Join(nested, ".git"))
	mustMkdir(filepath.Join(nested, "pkg"))

	mustMkdir(filepath.Join(outer, "stray", ".aide"))

	worktree = filepath.Join(tmp, "wt")
	mustMkdir(filepath.Join(worktree, "sub"))
	mustWrite(filepath.Join(worktree, ".git"),
		"gitdir: "+filepath.Join(super, ".git", "worktrees", "wt")+"\n")

	bare = filepath.Join(tmp, "bare")
	mustMkdir(bare)

	t.Setenv("AIDE_PROJECT_ROOT", "")
	os.Unsetenv("AIDE_PROJECT_ROOT")
	return
}

func chainRoots(a anchorInfo) []string {
	roots := make([]string, len(a.Chain))
	for i, s := range a.Chain {
		roots[i] = s.Root
	}
	return roots
}

func TestResolveAnchorSubmodule(t *testing.T) {
	_, outer, super, submodule, _, _, _ := anchorFixture(t)

	a := resolveAnchor(filepath.Join(submodule, "src"))

	if a.Root != submodule || !a.HasMarker || a.Source != "walk" {
		t.Fatalf("root=%q hasMarker=%v source=%q, want submodule/true/walk", a.Root, a.HasMarker, a.Source)
	}
	if a.Provenance.Marker != ".git" || a.Provenance.GitdirShape != "submodule" {
		t.Errorf("provenance = %+v, want .git/submodule", a.Provenance)
	}

	// Chain: self, superproject (submodule-gitdir), outer (ancestor-vcs-root).
	want := []string{submodule, super, outer}
	got := chainRoots(a)
	if len(got) != len(want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if a.Chain[0].Relation != "self" {
		t.Errorf("chain[0].relation = %q, want self", a.Chain[0].Relation)
	}
	if a.Chain[1].Evidence != "submodule-gitdir" {
		t.Errorf("chain[1].evidence = %q, want submodule-gitdir", a.Chain[1].Evidence)
	}
	if a.Chain[2].Evidence != "ancestor-vcs-root" {
		t.Errorf("chain[2].evidence = %q, want ancestor-vcs-root", a.Chain[2].Evidence)
	}
}

func TestResolveAnchorNestedPlainRepo(t *testing.T) {
	_, outer, super, _, nested, _, _ := anchorFixture(t)

	a := resolveAnchor(filepath.Join(nested, "pkg"))

	if a.Root != nested {
		t.Fatalf("root = %q, want nested %q", a.Root, nested)
	}
	if a.Provenance.GitdirShape != "directory" {
		t.Errorf("gitdirShape = %q, want directory", a.Provenance.GitdirShape)
	}
	want := []string{nested, super, outer}
	got := chainRoots(a)
	if len(got) != len(want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}
	for i, s := range a.Chain[1:] {
		if s.Evidence != "ancestor-vcs-root" {
			t.Errorf("chain[%d].evidence = %q, want ancestor-vcs-root", i+1, s.Evidence)
		}
	}
}

func TestResolveAnchorWorktree(t *testing.T) {
	_, _, super, _, _, worktree, _ := anchorFixture(t)

	a := resolveAnchor(filepath.Join(worktree, "sub"))

	if a.Root != super {
		t.Fatalf("root = %q, want main repo %q (worktrees share the main store)", a.Root, super)
	}
	if a.Provenance.GitdirShape != "worktree" {
		t.Errorf("gitdirShape = %q, want worktree", a.Provenance.GitdirShape)
	}
}

func TestResolveAnchorSuperRoot(t *testing.T) {
	_, outer, super, _, _, _, _ := anchorFixture(t)

	a := resolveAnchor(super)

	if a.Root != super {
		t.Fatalf("root = %q, want %q", a.Root, super)
	}
	want := []string{super, outer}
	got := chainRoots(a)
	if len(got) != len(want) || got[1] != outer {
		t.Fatalf("chain = %v, want %v", got, want)
	}
}

// TestResolveAnchorStrayAideNeverJoinsChain pins the leakage guard: an
// .aide/-only directory (no VCS) must never appear as a parent scope.
func TestResolveAnchorStrayAideNeverJoinsChain(t *testing.T) {
	_, outer, _, _, _, _, _ := anchorFixture(t)

	strayChild := filepath.Join(outer, "stray", "child")
	if err := os.MkdirAll(strayChild, 0o755); err != nil {
		t.Fatal(err)
	}

	a := resolveAnchor(strayChild)

	// Closest VCS root wins: outer. stray/.aide sits between cwd and outer
	// but carries no VCS marker.
	if a.Root != outer {
		t.Fatalf("root = %q, want outer %q", a.Root, outer)
	}
	for _, s := range a.Chain {
		if filepath.Base(s.Root) == "stray" {
			t.Errorf("stray .aide-only dir joined the chain: %v", chainRoots(a))
		}
	}
}

func TestResolveAnchorNoMarker(t *testing.T) {
	tmp, _, _, _, _, _, bare := anchorFixture(t)
	_ = tmp

	a := resolveAnchor(bare)

	if a.HasMarker || a.Source != "none" {
		t.Fatalf("hasMarker=%v source=%q, want false/none", a.HasMarker, a.Source)
	}
	if a.Root != bare {
		t.Errorf("root = %q, want cwd %q", a.Root, bare)
	}
	if len(a.Chain) != 1 || a.Chain[0].Relation != "self" {
		t.Errorf("chain = %+v, want single self entry", a.Chain)
	}

	// The probe must be read-only: no .aide/ may appear anywhere.
	if _, err := os.Stat(filepath.Join(bare, ".aide")); err == nil {
		t.Error("anchor created .aide/ in an unmarked directory")
	}
}

func TestResolveAnchorEnvOverride(t *testing.T) {
	_, _, super, submodule, _, _, _ := anchorFixture(t)

	t.Setenv("AIDE_PROJECT_ROOT", super)
	a := resolveAnchor(filepath.Join(submodule, "src"))

	if a.Root != super || a.Source != "env" || !a.HasMarker {
		t.Fatalf("root=%q source=%q hasMarker=%v, want super/env/true", a.Root, a.Source, a.HasMarker)
	}
}

func TestResolveAnchorPayload(t *testing.T) {
	_, _, super, _, _, _, _ := anchorFixture(t)

	a := resolveAnchor(super)

	if a.SchemaVersion != anchorSchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", a.SchemaVersion, anchorSchemaVersion)
	}
	if a.ResolverVersion == "" {
		t.Error("resolverVersion empty")
	}
	if a.Identity.ProjectName != "super" || a.Identity.Source != "basename" {
		t.Errorf("identity = %+v, want super/basename (fixture has no git remote)", a.Identity)
	}
	if a.DBPath != filepath.Join(super, defaultDBName) {
		t.Errorf("dbPath = %q, want default under root", a.DBPath)
	}
	if a.SocketPath == "" {
		t.Error("socketPath empty")
	}

	// The payload must round-trip as JSON — it is the cross-layer contract.
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back anchorInfo
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Root != a.Root || len(back.Chain) != len(a.Chain) {
		t.Errorf("round-trip mismatch: %+v vs %+v", back, a)
	}
}

func TestLastURLSegment(t *testing.T) {
	cases := map[string]string{
		"git@github.com:org/repo.git":     "repo",
		"https://github.com/org/repo.git": "repo",
		"https://github.com/org/repo":     "repo",
		"https://github.com/org/repo/":    "repo",
		"ssh://git@host:2222/org/repo":    "repo",
		"host:repo.git":                   "repo",
	}
	for url, want := range cases {
		if got := lastURLSegment(url); got != want {
			t.Errorf("lastURLSegment(%q) = %q, want %q", url, got, want)
		}
	}
}
