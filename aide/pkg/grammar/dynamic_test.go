package grammar

import (
	"testing"
)

// ---------------------------------------------------------------------------
// DynamicLoader basics — without actually loading shared libraries
// ---------------------------------------------------------------------------

func TestNewDynamicLoaderDefaults(t *testing.T) {
	dl := NewDynamicLoader("")
	if dl.dir == "" {
		t.Error("dir should have a default value")
	}
	if dl.baseURL != DefaultGrammarURL {
		t.Errorf("baseURL = %q; want %q", dl.baseURL, DefaultGrammarURL)
	}
}

func TestNewDynamicLoaderCustomDir(t *testing.T) {
	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	if dl.dir != dir {
		t.Errorf("dir = %q; want %q", dl.dir, dir)
	}
}

func TestDynamicLoaderInstalledEmpty(t *testing.T) {
	dl := NewDynamicLoader(t.TempDir())
	infos := dl.Installed()
	if len(infos) != 0 {
		t.Errorf("Installed on empty loader: got %d, want 0", len(infos))
	}
}

func TestDynamicLoaderLoadNotFound(t *testing.T) {
	dl := NewDynamicLoader(t.TempDir())
	_, err := dl.Load("ruby")
	if err == nil {
		t.Fatal("expected error loading non-installed grammar")
	}
	if _, ok := err.(*GrammarNotFoundError); !ok {
		t.Errorf("error type = %T; want *GrammarNotFoundError", err)
	}
}

func TestDynamicLoaderRemoveNonexistent(t *testing.T) {
	dl := NewDynamicLoader(t.TempDir())
	// Removing a grammar that was never installed should not error.
	if err := dl.Remove("nonexistent"); err != nil {
		t.Errorf("Remove(nonexistent): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dynamic packs — sanity checks via PackRegistry
// ---------------------------------------------------------------------------

func TestDynamicPacksMap(t *testing.T) {
	expected := []string{
		"bash", "csharp", "css", "elixir", "elm", "groovy", "hcl",
		"html", "kotlin", "lua", "ocaml", "php", "protobuf", "ruby",
		"scala", "sql", "swift", "toml", "yaml",
	}

	dynPacks := DefaultPackRegistry().DynamicPacks()
	if len(dynPacks) != len(expected) {
		t.Errorf("DynamicPacks has %d entries, want %d", len(dynPacks), len(expected))
	}

	for _, name := range expected {
		pack, ok := dynPacks[name]
		if !ok {
			t.Errorf("DynamicPacks[%q] missing", name)
			continue
		}
		if pack.SourceRepo == "" {
			t.Errorf("DynamicPacks[%q].SourceRepo is empty", name)
		}
		if pack.CSymbol == "" {
			t.Errorf("DynamicPacks[%q].CSymbol is empty", name)
		}
	}
}

func TestDynamicPacksNoOverlapWithBuiltins(t *testing.T) {
	r := NewBuiltinRegistry()
	for name := range DefaultPackRegistry().DynamicPacks() {
		if r.Has(name) {
			t.Errorf("DynamicPacks[%q] overlaps with builtin — should be one or the other", name)
		}
	}
}
