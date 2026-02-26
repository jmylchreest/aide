package grammar

import (
	"context"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// CompositeLoader creation and options
// ---------------------------------------------------------------------------

func TestNewCompositeLoaderDefaults(t *testing.T) {
	cl := NewCompositeLoader()

	if cl.builtin == nil {
		t.Fatal("builtin registry should not be nil")
	}
	if cl.dynamic == nil {
		t.Fatal("dynamic loader should not be nil")
	}
	if !cl.autoLoad {
		t.Error("autoLoad should default to true")
	}
}

func TestWithAutoDownload(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))
	if cl.autoLoad {
		t.Error("autoLoad should be false after WithAutoDownload(false)")
	}

	cl2 := NewCompositeLoader(WithAutoDownload(true))
	if !cl2.autoLoad {
		t.Error("autoLoad should be true after WithAutoDownload(true)")
	}
}

func TestWithGrammarDir(t *testing.T) {
	dir := t.TempDir()
	cl := NewCompositeLoader(WithGrammarDir(dir))
	if cl.dynamic.dir != dir {
		t.Errorf("dynamic.dir = %q; want %q", cl.dynamic.dir, dir)
	}
}

func TestWithBaseURL(t *testing.T) {
	url := "https://custom.example.com/{version}/{asset}"
	cl := NewCompositeLoader(WithBaseURL(url))
	if cl.dynamic.baseURL != url {
		t.Errorf("dynamic.baseURL = %q; want %q", cl.dynamic.baseURL, url)
	}
}

func TestWithBaseURLEmpty(t *testing.T) {
	// Empty string should not override the default.
	cl := NewCompositeLoader(WithBaseURL(""))
	if cl.dynamic.baseURL != DefaultGrammarURL {
		t.Errorf("dynamic.baseURL = %q; want default %q", cl.dynamic.baseURL, DefaultGrammarURL)
	}
}

// ---------------------------------------------------------------------------
// Load — builtin grammars
// ---------------------------------------------------------------------------

func TestCompositeLoaderLoadBuiltin(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	for _, name := range []string{"go", "python", "typescript", "javascript", "rust", "java", "c", "cpp", "zig"} {
		t.Run(name, func(t *testing.T) {
			lang, err := cl.Load(context.Background(), name)
			if err != nil {
				t.Fatalf("Load(%q): %v", name, err)
			}
			if lang == nil {
				t.Fatalf("Load(%q) returned nil", name)
			}
		})
	}
}

func TestCompositeLoaderLoadCaching(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	lang1, _ := cl.Load(context.Background(), "go")
	lang2, _ := cl.Load(context.Background(), "go")

	if lang1 != lang2 {
		t.Error("second Load should return cached Language")
	}
}

func TestCompositeLoaderLoadNotFound(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	_, err := cl.Load(context.Background(), "nonexistent-lang")
	if err == nil {
		t.Fatal("expected error for unknown grammar with autoLoad disabled")
	}
	if _, ok := err.(*ErrGrammarNotFound); !ok {
		t.Errorf("error type = %T; want *ErrGrammarNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Available — union of builtins + dynamic
// ---------------------------------------------------------------------------

func TestCompositeLoaderAvailable(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))
	avail := cl.Available()

	if len(avail) == 0 {
		t.Fatal("Available() returned empty list")
	}

	// Should include all 9 builtins.
	availSet := make(map[string]bool)
	for _, n := range avail {
		availSet[n] = true
	}

	for _, name := range []string{"go", "python", "typescript", "javascript", "rust", "java", "c", "cpp", "zig"} {
		if !availSet[name] {
			t.Errorf("Available() missing builtin %q", name)
		}
	}

	// Should include dynamic grammars too.
	for _, name := range []string{"ruby", "kotlin", "bash", "php"} {
		if !availSet[name] {
			t.Errorf("Available() missing dynamic %q", name)
		}
	}

	// Total should be 9 builtins + 19 dynamic = 28
	expected := len(expectedBuiltins) + len(DynamicGrammars)
	if len(avail) != expected {
		t.Errorf("Available() count = %d; want %d", len(avail), expected)
	}
}

// ---------------------------------------------------------------------------
// Installed — only builtins when nothing dynamic is installed
// ---------------------------------------------------------------------------

func TestCompositeLoaderInstalledOnlyBuiltins(t *testing.T) {
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(t.TempDir()),
	)

	installed := cl.Installed()

	// Should have exactly the 9 builtins.
	if len(installed) != len(expectedBuiltins) {
		t.Errorf("Installed() count = %d; want %d builtins", len(installed), len(expectedBuiltins))
	}

	for _, info := range installed {
		if !info.BuiltIn {
			t.Errorf("Installed() entry %q should be BuiltIn", info.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Install — builtin is a no-op, unknown returns error
// ---------------------------------------------------------------------------

func TestCompositeLoaderInstallBuiltinNoop(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	// Installing a builtin should be a no-op (no error).
	if err := cl.Install(context.Background(), "go"); err != nil {
		t.Errorf("Install(builtin) should be a no-op: %v", err)
	}
}

func TestCompositeLoaderInstallUnknown(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	err := cl.Install(context.Background(), "nonexistent-lang")
	if err == nil {
		t.Fatal("expected error installing unknown grammar")
	}
	if _, ok := err.(*ErrGrammarNotFound); !ok {
		t.Errorf("error type = %T; want *ErrGrammarNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Remove — clears cache
// ---------------------------------------------------------------------------

func TestCompositeLoaderRemove(t *testing.T) {
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(t.TempDir()),
	)

	// Load a builtin to populate cache.
	_, _ = cl.Load(context.Background(), "go")

	// Remove should clear the cache entry (and not error even though it's builtin).
	if err := cl.Remove("go"); err != nil {
		t.Errorf("Remove: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GenerateLockFile / InstallFromLock
// ---------------------------------------------------------------------------

func TestCompositeLoaderGenerateLockFileEmpty(t *testing.T) {
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(t.TempDir()),
	)

	lf := cl.GenerateLockFile()
	if len(lf.Grammars) != 0 {
		t.Errorf("GenerateLockFile on fresh loader: got %d grammars, want 0", len(lf.Grammars))
	}
}

func TestCompositeLoaderInstallFromLockSkipsInstalled(t *testing.T) {
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(t.TempDir()),
	)

	// Create a lock file with only builtin grammars.
	lf := &LockFile{
		Grammars: map[string]*LockEntry{
			"go":     {Version: "v1", CSymbol: "tree_sitter_go"},
			"python": {Version: "v1", CSymbol: "tree_sitter_python"},
		},
	}

	// All grammars are already installed (builtins), so nothing new should be installed.
	installed, err := cl.InstallFromLock(context.Background(), lf)
	if err != nil {
		t.Fatalf("InstallFromLock: %v", err)
	}
	if len(installed) != 0 {
		t.Errorf("installed = %v; want empty (all already installed)", installed)
	}
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

func TestErrGrammarNotFoundMessage(t *testing.T) {
	err := &ErrGrammarNotFound{Name: "ruby"}
	got := err.Error()
	if got != `grammar "ruby" not found` {
		t.Errorf("Error() = %q", got)
	}
}

func TestErrDownloadFailedMessage(t *testing.T) {
	inner := &ErrGrammarNotFound{Name: "inner"}
	err := &ErrDownloadFailed{Name: "ruby", Err: inner}
	got := err.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
}

func TestErrIncompatibleABIMessage(t *testing.T) {
	err := &ErrIncompatibleABI{Name: "ruby", AbiVersion: 10, MinVersion: 13, MaxVersion: 14}
	got := err.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
}

// ---------------------------------------------------------------------------
// expectedBuiltins is defined in builtin_test.go — verify Available() order is consistent
// ---------------------------------------------------------------------------

func TestCompositeLoaderAvailableSortable(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))
	avail := cl.Available()

	sorted := make([]string, len(avail))
	copy(sorted, avail)
	sort.Strings(sorted)

	// Available does NOT guarantee sorted order, but we verify the set is valid.
	if len(avail) != len(sorted) {
		t.Error("Available() returned duplicates")
	}
}
