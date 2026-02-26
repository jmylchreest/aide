package grammar

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockFileRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &LockFile{
		Grammars: map[string]*LockEntry{
			"ruby": {
				Version: "v0.23.1",
				SHA256:  "abc123",
				CSymbol: "tree_sitter_ruby",
			},
			"kotlin": {
				Version: "v0.3.8",
				SHA256:  "def456",
				CSymbol: "tree_sitter_kotlin",
			},
		},
	}

	if err := WriteLockFile(dir, original); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	// Verify file exists
	lockPath := filepath.Join(dir, LockFileName)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	loaded, err := ReadLockFile(dir)
	if err != nil {
		t.Fatalf("ReadLockFile: %v", err)
	}
	if loaded == nil {
		t.Fatal("ReadLockFile returned nil")
	}

	// Check comment was set
	if loaded.Comment == "" {
		t.Error("Comment should be set by WriteLockFile")
	}

	// Check generated_at was set
	if loaded.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should be set by WriteLockFile")
	}

	// Check grammar entries
	if len(loaded.Grammars) != 2 {
		t.Fatalf("expected 2 grammars, got %d", len(loaded.Grammars))
	}

	for _, name := range []string{"ruby", "kotlin"} {
		orig := original.Grammars[name]
		got := loaded.Grammars[name]
		if got == nil {
			t.Errorf("grammar %q not found in loaded lock file", name)
			continue
		}
		if got.Version != orig.Version {
			t.Errorf("grammar %q version: got %q, want %q", name, got.Version, orig.Version)
		}
		if got.SHA256 != orig.SHA256 {
			t.Errorf("grammar %q sha256: got %q, want %q", name, got.SHA256, orig.SHA256)
		}
		if got.CSymbol != orig.CSymbol {
			t.Errorf("grammar %q c_symbol: got %q, want %q", name, got.CSymbol, orig.CSymbol)
		}
	}
}

func TestReadLockFileNotExist(t *testing.T) {
	dir := t.TempDir()

	lf, err := ReadLockFile(dir)
	if err != nil {
		t.Fatalf("ReadLockFile on missing file: %v", err)
	}
	if lf != nil {
		t.Error("expected nil for non-existent lock file")
	}
}

func TestLockFileNames(t *testing.T) {
	lf := &LockFile{
		Grammars: map[string]*LockEntry{
			"kotlin": {Version: "v1"},
			"bash":   {Version: "v2"},
			"ruby":   {Version: "v3"},
		},
	}

	names := lf.Names()
	expected := []string{"bash", "kotlin", "ruby"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d]: got %q, want %q", i, name, expected[i])
		}
	}
}

func TestLockFileFromManifest(t *testing.T) {
	ms := newManifestStore(t.TempDir())
	ms.set("ruby", &ManifestEntry{
		Version:     "v0.23.1",
		File:        "libtree-sitter-ruby.so",
		SHA256:      "abc123",
		CSymbol:     "tree_sitter_ruby",
		InstalledAt: time.Now(),
	})
	ms.set("php", &ManifestEntry{
		Version:     "v0.24.0",
		File:        "libtree-sitter-php.so",
		SHA256:      "def456",
		CSymbol:     "tree_sitter_php",
		InstalledAt: time.Now(),
	})

	lf := LockFileFromManifest(ms)
	if len(lf.Grammars) != 2 {
		t.Fatalf("expected 2 grammars, got %d", len(lf.Grammars))
	}

	ruby := lf.Grammars["ruby"]
	if ruby == nil {
		t.Fatal("ruby entry missing")
	}
	if ruby.Version != "v0.23.1" {
		t.Errorf("ruby version: got %q, want %q", ruby.Version, "v0.23.1")
	}
	if ruby.SHA256 != "abc123" {
		t.Errorf("ruby sha256: got %q, want %q", ruby.SHA256, "abc123")
	}

	php := lf.Grammars["php"]
	if php == nil {
		t.Fatal("php entry missing")
	}
	if php.Version != "v0.24.0" {
		t.Errorf("php version: got %q, want %q", php.Version, "v0.24.0")
	}
}
