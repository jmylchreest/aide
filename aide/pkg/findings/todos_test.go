package findings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeTodos_BasicCategories(t *testing.T) {
	tmp := t.TempDir()
	src := `package demo

// TODO: tidy this up later
// FIXME(jmylchreest): wrong sort order — see AIDE-123
// BUG: panics on empty slice
func demo() {} // NOTE: keep small
`
	path := filepath.Join(tmp, "demo.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ff, res, err := AnalyzeTodos(TodosConfig{Paths: []string{tmp}, ProjectRoot: tmp})
	if err != nil {
		t.Fatalf("AnalyzeTodos: %v", err)
	}
	if res.FilesAnalyzed != 1 {
		t.Errorf("expected 1 file analyzed, got %d", res.FilesAnalyzed)
	}
	if len(ff) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(ff))
	}

	bySev := map[string]int{}
	byKW := map[string]*Finding{}
	for _, f := range ff {
		bySev[f.Severity]++
		byKW[f.Metadata["keyword"]] = f
	}

	if bySev[SevInfo] != 2 || bySev[SevWarning] != 1 || bySev[SevCritical] != 1 {
		t.Errorf("severity distribution wrong: %v", bySev)
	}

	fixme := byKW["FIXME"]
	if fixme == nil {
		t.Fatal("missing FIXME finding")
	}
	if fixme.Metadata["owner"] != "jmylchreest" {
		t.Errorf("expected owner=jmylchreest, got %q", fixme.Metadata["owner"])
	}
	if fixme.Metadata["ticket"] != "AIDE-123" {
		t.Errorf("expected ticket=AIDE-123, got %q", fixme.Metadata["ticket"])
	}
}

func TestAnalyzeTodos_OnlyComments(t *testing.T) {
	tmp := t.TempDir()
	// Tier A accepts that string literals containing keywords may produce noise;
	// here we just check that ordinary code lines without comment delimiters are skipped.
	src := `package demo
func demo() string { return "TODO not flagged here" }
`
	path := filepath.Join(tmp, "demo.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ff, _, err := AnalyzeTodos(TodosConfig{Paths: []string{tmp}, ProjectRoot: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if len(ff) != 0 {
		t.Errorf("expected 0 findings (no comment), got %d", len(ff))
	}
}
