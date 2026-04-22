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

func TestAnnotateEstTokensExcludesGeneratedAndLockfiles(t *testing.T) {
	tmp := t.TempDir()

	// Write real files so we know any missing est_tokens is the exclusion,
	// not a failed stat.
	mustWrite := func(rel string) {
		full := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	excluded := []string{
		"bundle.min.js",
		"styles.min.css",
		"package-lock.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lock",
		"bun.lockb",
		"Cargo.lock",
		"go.sum",
		"Gemfile.lock",
		"composer.lock",
		"poetry.lock",
		"Pipfile.lock",
		"vendor/some/pkg.go",
		"node_modules/lib/index.js",
	}
	for _, rel := range excluded {
		mustWrite(rel)
	}
	// Generated protobuf sources are deliberately NOT excluded — agents may
	// need to read them when debugging wire format or field metadata.
	kept := []string{
		"pkg/thing.go",
		"src/index.ts",
		"aidememory.pb.go",
		"aidememory_grpc.pb.go",
		"api.pb.ts",
		"api.pb.js",
	}
	for _, rel := range kept {
		mustWrite(rel)
	}

	all := append(append([]string{}, excluded...), kept...)
	entries := make([]*Entry, len(all))
	for i, rel := range all {
		entries[i] = &Entry{FilePath: rel}
	}
	AnnotateEstTokens(tmp, entries)

	for i, rel := range excluded {
		if EstTokensFor(entries[i]) != 0 {
			t.Errorf("%s should be excluded from est_tokens, got %d", rel, EstTokensFor(entries[i]))
		}
	}
	for i, rel := range kept {
		idx := len(excluded) + i
		if EstTokensFor(entries[idx]) == 0 {
			t.Errorf("%s should have est_tokens set", rel)
		}
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
