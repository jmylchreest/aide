package grammar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// manifestStore — basic CRUD
// ---------------------------------------------------------------------------

func TestManifestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	// Set entries
	ms.set("ruby", &ManifestEntry{
		Version:     "v0.23.1",
		File:        "libtree-sitter-ruby.so",
		SHA256:      "abc123",
		CSymbol:     "tree_sitter_ruby",
		InstalledAt: time.Now().Truncate(time.Second),
	})
	ms.set("php", &ManifestEntry{
		Version:     "v0.24.0",
		File:        "libtree-sitter-php.so",
		SHA256:      "def456",
		CSymbol:     "tree_sitter_php",
		InstalledAt: time.Now().Truncate(time.Second),
	})

	// Save
	if err := ms.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not created: %v", err)
	}

	// Load into a fresh store
	ms2 := newManifestStore(dir)
	if err := ms2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	ruby := ms2.get("ruby")
	if ruby == nil {
		t.Fatal("ruby entry missing after round-trip")
	}
	if ruby.Version != "v0.23.1" {
		t.Errorf("ruby version: got %q, want %q", ruby.Version, "v0.23.1")
	}
	if ruby.SHA256 != "abc123" {
		t.Errorf("ruby sha256: got %q, want %q", ruby.SHA256, "abc123")
	}
	if ruby.CSymbol != "tree_sitter_ruby" {
		t.Errorf("ruby c_symbol: got %q, want %q", ruby.CSymbol, "tree_sitter_ruby")
	}

	php := ms2.get("php")
	if php == nil {
		t.Fatal("php entry missing after round-trip")
	}
	if php.Version != "v0.24.0" {
		t.Errorf("php version: got %q, want %q", php.Version, "v0.24.0")
	}
}

func TestManifestStoreLoadNotExist(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	if err := ms.load(); err != nil {
		t.Fatalf("load on non-existent: %v", err)
	}

	// Should initialise empty grammars map.
	entries := ms.entries()
	if len(entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(entries))
	}
}

func TestManifestStoreGetMissing(t *testing.T) {
	ms := newManifestStore(t.TempDir())
	if got := ms.get("nonexistent"); got != nil {
		t.Errorf("get(nonexistent) = %+v; want nil", got)
	}
}

func TestManifestStoreRemove(t *testing.T) {
	ms := newManifestStore(t.TempDir())
	ms.set("ruby", &ManifestEntry{Version: "v1"})
	ms.set("php", &ManifestEntry{Version: "v1"})

	ms.remove("ruby")

	if ms.get("ruby") != nil {
		t.Error("ruby should be nil after remove")
	}
	if ms.get("php") == nil {
		t.Error("php should still exist after removing ruby")
	}
}

func TestManifestStoreEntries(t *testing.T) {
	ms := newManifestStore(t.TempDir())
	ms.set("a", &ManifestEntry{Version: "v1"})
	ms.set("b", &ManifestEntry{Version: "v2"})

	entries := ms.entries()
	if len(entries) != 2 {
		t.Fatalf("entries count: got %d, want 2", len(entries))
	}

	// Modifying the returned map should not affect the store (it's a copy).
	delete(entries, "a")
	if ms.get("a") == nil {
		t.Error("deleting from entries() return should not affect the store")
	}
}

func TestManifestStoreOverwrite(t *testing.T) {
	ms := newManifestStore(t.TempDir())
	ms.set("ruby", &ManifestEntry{Version: "v1"})
	ms.set("ruby", &ManifestEntry{Version: "v2"})

	got := ms.get("ruby")
	if got == nil || got.Version != "v2" {
		t.Errorf("overwrite: got version %v; want v2", got)
	}
}

func TestManifestStoreSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")
	ms := newManifestStore(dir)
	ms.set("test", &ManifestEntry{Version: "v1"})

	if err := ms.save(); err != nil {
		t.Fatalf("save should create missing directories: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Errorf("manifest.json not found: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NeedsRescan field persistence
// ---------------------------------------------------------------------------

func TestManifestStoreNeedsRescanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	// Set an entry with NeedsRescan=true.
	ms.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})
	// Set another entry without NeedsRescan (false/omitted).
	ms.set("php", &ManifestEntry{
		Version: "v1.0.0",
		File:    "php/grammar.so",
	})

	if err := ms.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload from disk.
	ms2 := newManifestStore(dir)
	if err := ms2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	ruby := ms2.get("ruby")
	if ruby == nil {
		t.Fatal("expected ruby entry")
	}
	if !ruby.NeedsRescan {
		t.Error("ruby.NeedsRescan should be true after round-trip")
	}

	php := ms2.get("php")
	if php == nil {
		t.Fatal("expected php entry")
	}
	if php.NeedsRescan {
		t.Error("php.NeedsRescan should be false (default)")
	}
}

func TestManifestStoreNeedsRescanClearAndPersist(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	ms.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})
	if err := ms.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Clear NeedsRescan by modifying the entry directly (simulating MarkRescanComplete).
	ms.mu.Lock()
	if entry, ok := ms.data.Grammars["ruby"]; ok {
		entry.NeedsRescan = false
	}
	ms.mu.Unlock()
	if err := ms.save(); err != nil {
		t.Fatalf("save after clear: %v", err)
	}

	// Reload.
	ms2 := newManifestStore(dir)
	if err := ms2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	entry := ms2.get("ruby")
	if entry == nil {
		t.Fatal("expected ruby entry")
	}
	if entry.NeedsRescan {
		t.Error("NeedsRescan should be false after clearing and persisting")
	}
}

func TestManifestStoreNeedsRescanOmittedInJSON(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	// Entry with NeedsRescan=false should use omitempty → field absent from JSON.
	ms.set("ruby", &ManifestEntry{
		Version: "v1.0.0",
		File:    "ruby/grammar.so",
	})
	if err := ms.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Read the raw JSON and verify "needs_rescan" is not present.
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The JSON should NOT contain the "needs_rescan" key when false.
	if strings.Contains(string(data), "needs_rescan") {
		t.Error("expected needs_rescan to be omitted from JSON when false (omitempty)")
	}

	// Now set NeedsRescan=true and verify it IS present.
	ms.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})
	if err := ms.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err = os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "needs_rescan") {
		t.Error("expected needs_rescan to be present in JSON when true")
	}
}

func TestManifestStoreEntriesDefensiveCopyPreservesNeedsRescan(t *testing.T) {
	dir := t.TempDir()
	ms := newManifestStore(dir)

	ms.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		NeedsRescan: true,
	})
	ms.set("php", &ManifestEntry{
		Version:     "v1.0.0",
		NeedsRescan: false,
	})

	entries := ms.entries()

	// Verify the defensive copy preserves NeedsRescan.
	if !entries["ruby"].NeedsRescan {
		t.Error("ruby.NeedsRescan should be true in copied entries")
	}
	if entries["php"].NeedsRescan {
		t.Error("php.NeedsRescan should be false in copied entries")
	}

	// Mutating the copy should not affect the original.
	entries["ruby"].NeedsRescan = false
	original := ms.get("ruby")
	if !original.NeedsRescan {
		t.Error("mutating entries() copy should not affect original")
	}
}
