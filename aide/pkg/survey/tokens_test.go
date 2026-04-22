package survey

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAnnotateEstTokensRegularFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "foo.go")
	// ~300 bytes of Go source → tokens ≈ 300 / 2.79 ≈ 108.
	body := []byte("package foo\n\nfunc Hello() string { return \"hi\" }\n")
	for len(body) < 300 {
		body = append(body, body...)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []*Entry{
		{FilePath: "foo.go"},
	}
	AnnotateEstTokens(tmp, entries)

	got := EstTokensFor(entries[0])
	if got <= 0 {
		t.Fatalf("est_tokens not set on a real file, metadata=%v", entries[0].Metadata)
	}
	// Sanity: within a reasonable range for the written body.
	if got > len(body) {
		t.Errorf("est_tokens %d exceeds byte count %d — ratio likely wrong", got, len(body))
	}

	// Round-trip through strconv matches the helper.
	if raw, ok := entries[0].Metadata[MetaEstTokens]; !ok {
		t.Error("metadata missing MetaEstTokens key")
	} else if _, err := strconv.Atoi(raw); err != nil {
		t.Errorf("metadata value not integer: %q", raw)
	}
}

func TestAnnotateEstTokensDirectoryAndMissingSkipped(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []*Entry{
		{FilePath: "subdir"},          // directory — skip
		{FilePath: "nope.go"},         // missing — skip
		{FilePath: ""},                // empty — skip
		{FilePath: "also-missing.py"}, // missing — skip
	}
	AnnotateEstTokens(tmp, entries)

	for i, e := range entries {
		if EstTokensFor(e) != 0 {
			t.Errorf("entries[%d]: expected 0 tokens, got %d (metadata=%v)", i, EstTokensFor(e), e.Metadata)
		}
	}
}

func TestAnnotateEstTokensPreservesExistingMetadata(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "x.ts")
	if err := os.WriteFile(path, []byte("export const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []*Entry{
		{
			FilePath: "x.ts",
			Metadata: map[string]string{"language": "typescript"},
		},
	}
	AnnotateEstTokens(tmp, entries)

	if entries[0].Metadata["language"] != "typescript" {
		t.Error("existing metadata key was overwritten")
	}
	if EstTokensFor(entries[0]) <= 0 {
		t.Error("est_tokens not written alongside existing metadata")
	}
}

// TestAnnotateEstTokensWalksUpFromSubdir covers the monorepo case where the
// caller hands us a sub-module rootDir but the entry's FilePath is reported
// relative to the repo root (e.g., git-based churn paths).
func TestAnnotateEstTokensWalksUpFromSubdir(t *testing.T) {
	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "module")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "pkg", "thing.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Caller passes the sub-module root; FilePath is relative to the repo
	// root (one level up). The helper should walk up and find it.
	entries := []*Entry{{FilePath: "pkg/thing.go"}}
	AnnotateEstTokens(subDir, entries)

	if EstTokensFor(entries[0]) <= 0 {
		t.Fatalf("walk-up resolution failed, metadata=%v", entries[0].Metadata)
	}
}

func TestAnnotateEstTokensIdempotent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "x.go")
	if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []*Entry{{
		FilePath: "x.go",
		Metadata: map[string]string{MetaEstTokens: "9999"},
	}}
	AnnotateEstTokens(tmp, entries)

	// Pre-existing value must survive. Survey-time snapshot wins even when
	// it diverges from current disk state; repeat calls become cheap no-ops.
	if got := EstTokensFor(entries[0]); got != 9999 {
		t.Errorf("pre-existing est_tokens overwritten: got %d, want 9999", got)
	}
}

func TestCounterfactualTokensForEntriesFallsBackToDisk(t *testing.T) {
	tmp := t.TempDir()
	cached := filepath.Join(tmp, "cached.go")
	live := filepath.Join(tmp, "live.go")
	for _, p := range []string{cached, live} {
		if err := os.WriteFile(p, []byte("package x; var a = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries := []*Entry{
		{FilePath: "cached.go", Metadata: map[string]string{MetaEstTokens: "250"}},
		{FilePath: "live.go"}, // no metadata — must resolve from disk
		{FilePath: "missing.go"},
		{FilePath: ""},
		nil,
	}
	total := CounterfactualTokensForEntries(tmp, entries)
	if total <= 250 {
		t.Errorf("total %d does not include a live-stat contribution for live.go", total)
	}

	// Non-mutating: entry[1] must not have gained metadata.
	if _, ok := entries[1].Metadata[MetaEstTokens]; ok {
		t.Error("CounterfactualTokensForEntries mutated input entries")
	}
}

func TestEstTokensForMalformedReturnsZero(t *testing.T) {
	cases := []*Entry{
		nil,
		{},
		{Metadata: map[string]string{}},
		{Metadata: map[string]string{MetaEstTokens: "not-a-number"}},
		{Metadata: map[string]string{MetaEstTokens: "-5"}},
	}
	for i, c := range cases {
		if got := EstTokensFor(c); got != 0 {
			t.Errorf("case %d: got %d, want 0", i, got)
		}
	}
}
