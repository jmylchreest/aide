package grammar

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// NormaliseLang — alias table
// ---------------------------------------------------------------------------

func TestNormaliseLang(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Direct aliases
		{"ts", "typescript"},
		{"tsx", "typescript"},
		{"js", "javascript"},
		{"jsx", "javascript"},
		{"py", "python"},
		{"rs", "rust"},
		{"c++", "cpp"},
		{"c#", "csharp"},
		{"cs", "csharp"},
		{"rb", "ruby"},
		{"sh", "bash"},
		{"shell", "bash"},
		{"kt", "kotlin"},
		{"ex", "elixir"},
		{"exs", "elixir"},
		{"ml", "ocaml"},
		{"tf", "hcl"},
		{"terraform", "hcl"},
		{"proto", "protobuf"},
		{"yml", "yaml"},

		// Identity (no alias needed)
		{"go", "go"},
		{"python", "python"},
		{"rust", "rust"},
		{"java", "java"},
		{"ruby", "ruby"},
		{"typescript", "typescript"},

		// Case insensitivity
		{"TS", "typescript"},
		{"Py", "python"},
		{"RUST", "rust"},
		{"C++", "cpp"},

		// Whitespace trimming
		{"  py  ", "python"},
		{" ts\t", "typescript"},

		// Unknown passes through
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormaliseLang(tt.input)
			if got != tt.want {
				t.Errorf("NormaliseLang(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ScanProject — with temp dir fixtures
// ---------------------------------------------------------------------------

func TestScanProject(t *testing.T) {
	// Create a temp project with known files.
	root := t.TempDir()

	files := map[string]string{
		"main.go":            "package main",
		"util.go":            "package main",
		"lib.py":             "print('hello')",
		"app.ts":             "console.log('hi')",
		"index.js":           "module.exports = {}",
		"README.md":          "# readme", // no grammar — should be skipped
		"data.json":          "{}",       // LangJSON — detected but no grammar
		"nested/deep/foo.go": "package deep",
	}

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Simple detector based on extension.
	detect := func(filePath string, _ []byte) string {
		ext := filepath.Ext(filePath)
		switch ext {
		case ".go":
			return "go"
		case ".py":
			return "python"
		case ".ts":
			return "typescript"
		case ".js":
			return "javascript"
		case ".json":
			return "json"
		}
		return ""
	}

	loader := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(filepath.Join(root, ".aide", "grammars")),
	)

	result, err := ScanProject(root, loader, detect, nil)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	// Check language counts.
	if result.Languages["go"] != 3 {
		t.Errorf("go files: got %d, want 3", result.Languages["go"])
	}
	if result.Languages["python"] != 1 {
		t.Errorf("python files: got %d, want 1", result.Languages["python"])
	}
	if result.Languages["typescript"] != 1 {
		t.Errorf("typescript files: got %d, want 1", result.Languages["typescript"])
	}
	if result.Languages["javascript"] != 1 {
		t.Errorf("javascript files: got %d, want 1", result.Languages["javascript"])
	}
	if result.Languages["json"] != 1 {
		t.Errorf("json files: got %d, want 1", result.Languages["json"])
	}

	// Total recognised files: go(3) + py(1) + ts(1) + js(1) + json(1) = 7
	if result.TotalFiles != 7 {
		t.Errorf("TotalFiles: got %d, want 7", result.TotalFiles)
	}

	// "json" has no grammar (not builtin, not in DynamicPacks) — should be unavailable.
	found := false
	for _, u := range result.Unavailable {
		if u == "json" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'json' in Unavailable; got %v", result.Unavailable)
	}
}

func TestScanProjectEmpty(t *testing.T) {
	root := t.TempDir()

	detect := func(filePath string, _ []byte) string { return "" }
	loader := NewCompositeLoader(WithAutoDownload(false))

	result, err := ScanProject(root, loader, detect, nil)
	if err != nil {
		t.Fatalf("ScanProject on empty dir: %v", err)
	}
	if result.TotalFiles != 0 {
		t.Errorf("TotalFiles: got %d, want 0", result.TotalFiles)
	}
	if len(result.Languages) != 0 {
		t.Errorf("Languages should be empty, got %v", result.Languages)
	}
}

// ---------------------------------------------------------------------------
// ScanDetail — higher-level view
// ---------------------------------------------------------------------------

func TestScanDetail(t *testing.T) {
	root := t.TempDir()

	// Create files for a builtin (go), a dynamic available (ruby), and an unknown.
	files := map[string]string{
		"main.go":    "package main",
		"app.rb":     "puts 'hello'",
		"config.xyz": "unknown",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	detect := func(filePath string, _ []byte) string {
		switch filepath.Ext(filePath) {
		case ".go":
			return "go"
		case ".rb":
			return "ruby"
		case ".xyz":
			return "xyzlang"
		}
		return ""
	}

	loader := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(filepath.Join(root, ".aide", "grammars")),
	)

	statuses, err := ScanDetail(root, loader, detect, nil)
	if err != nil {
		t.Fatalf("ScanDetail: %v", err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d: %+v", len(statuses), statuses)
	}

	statusMap := make(map[string]LanguageStatus)
	for _, s := range statuses {
		statusMap[s.Name] = s
	}

	// Go should be builtin.
	if g, ok := statusMap["go"]; !ok {
		t.Error("missing 'go' in statuses")
	} else if g.Status != "builtin" {
		t.Errorf("go status = %q; want %q", g.Status, "builtin")
	}

	// Ruby should be available (in DynamicPacks but not installed).
	if r, ok := statusMap["ruby"]; !ok {
		t.Error("missing 'ruby' in statuses")
	} else {
		if r.Status != "available" {
			t.Errorf("ruby status = %q; want %q", r.Status, "available")
		}
		if !r.CanInstall {
			t.Error("ruby CanInstall should be true")
		}
	}

	// xyzlang should be unavailable.
	if x, ok := statusMap["xyzlang"]; !ok {
		t.Error("missing 'xyzlang' in statuses")
	} else if x.Status != "unavailable" {
		t.Errorf("xyzlang status = %q; want %q", x.Status, "unavailable")
	}
}

// ---------------------------------------------------------------------------
// sortedKeys — helper
// ---------------------------------------------------------------------------

func TestSortedKeys(t *testing.T) {
	got := sortedKeys(map[string]bool{"c": true, "a": true, "b": true})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedKeys[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestSortedKeysNil(t *testing.T) {
	got := sortedKeys(nil)
	if got != nil {
		t.Errorf("sortedKeys(nil) = %v; want nil", got)
	}
}

func TestSortedKeysEmpty(t *testing.T) {
	got := sortedKeys(map[string]bool{})
	if got != nil {
		t.Errorf("sortedKeys(empty) = %v; want nil", got)
	}
}

// ---------------------------------------------------------------------------
// dynamicAvailable
// ---------------------------------------------------------------------------

func TestDynamicAvailable(t *testing.T) {
	names, set := dynamicAvailable()

	dynPacks := DefaultPackRegistry().DynamicPacks()
	if len(names) != len(dynPacks) {
		t.Errorf("names length = %d; want %d", len(names), len(dynPacks))
	}

	// Names should be sorted.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %q > %q at index %d", names[i-1], names[i], i)
		}
	}

	// Every name should be in the set.
	for _, n := range names {
		if !set[n] {
			t.Errorf("name %q not in set", n)
		}
	}
}
