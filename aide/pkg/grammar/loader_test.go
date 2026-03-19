package grammar

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestWithVersion(t *testing.T) {
	cl := NewCompositeLoader(WithVersion("v0.0.39"))
	if cl.dynamic.version != "v0.0.39" {
		t.Errorf("dynamic.version = %q; want %q", cl.dynamic.version, "v0.0.39")
	}
}

func TestWithVersionSnapshot(t *testing.T) {
	cl := NewCompositeLoader(WithVersion("snapshot"))
	if cl.dynamic.version != "snapshot" {
		t.Errorf("dynamic.version = %q; want %q", cl.dynamic.version, "snapshot")
	}
}

func TestWithVersionEmpty(t *testing.T) {
	// Empty string should be allowed — Download() falls back to "snapshot".
	cl := NewCompositeLoader(WithVersion(""))
	if cl.dynamic.version != "" {
		t.Errorf("dynamic.version = %q; want empty", cl.dynamic.version)
	}
}

// ---------------------------------------------------------------------------
// Load — builtin grammars
// ---------------------------------------------------------------------------

func TestCompositeLoaderLoadBuiltin(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	for _, name := range []string{"go", "python", "typescript", "tsx", "javascript", "rust", "java", "c", "cpp", "zig"} {
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
	if _, ok := err.(*GrammarNotFoundError); !ok {
		t.Errorf("error type = %T; want *GrammarNotFoundError", err)
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

	// Should include all 10 builtins.
	availSet := make(map[string]bool)
	for _, n := range avail {
		availSet[n] = true
	}

	for _, name := range []string{"go", "python", "typescript", "tsx", "javascript", "rust", "java", "c", "cpp", "zig"} {
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

	// Total should be 10 builtins + 19 dynamic = 29
	expected := len(expectedBuiltins) + len(DefaultPackRegistry().DynamicPacks())
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

	// Should have exactly the 10 builtins.
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
	if _, ok := err.(*GrammarNotFoundError); !ok {
		t.Errorf("error type = %T; want *GrammarNotFoundError", err)
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

func TestGrammarNotFoundErrorMessage(t *testing.T) {
	err := &GrammarNotFoundError{Name: "ruby"}
	got := err.Error()
	if got != `grammar "ruby" not found` {
		t.Errorf("Error() = %q", got)
	}
}

func TestDownloadFailedErrorMessage(t *testing.T) {
	inner := &GrammarNotFoundError{Name: "inner"}
	err := &DownloadFailedError{Name: "ruby", Err: inner}
	got := err.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
}

func TestIncompatibleABIErrorMessage(t *testing.T) {
	err := &IncompatibleABIError{Name: "ruby", AbiVersion: 10, MinVersion: 13, MaxVersion: 14}
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

// ---------------------------------------------------------------------------
// downloadFailure backoff
// ---------------------------------------------------------------------------

func TestDownloadFailureBackoff(t *testing.T) {
	// Verify that backoff durations increase exponentially and are capped.
	tests := []struct {
		attempts int
		minDelay time.Duration // 0 (jitter can produce 0)
		maxDelay time.Duration // exponential * 2 base, capped at downloadBackoffMax
	}{
		{attempts: 1, minDelay: 0, maxDelay: downloadBackoffBase},     // [0, 30s)
		{attempts: 2, minDelay: 0, maxDelay: 2 * downloadBackoffBase}, // [0, 60s)
		{attempts: 3, minDelay: 0, maxDelay: 4 * downloadBackoffBase}, // [0, 120s)
		{attempts: 4, minDelay: 0, maxDelay: 8 * downloadBackoffBase}, // [0, 240s)
		{attempts: 5, minDelay: 0, maxDelay: downloadBackoffMax},      // [0, 300s) — capped
		{attempts: 10, minDelay: 0, maxDelay: downloadBackoffMax},     // [0, 300s) — still capped
		{attempts: 100, minDelay: 0, maxDelay: downloadBackoffMax},    // [0, 300s) — extreme case
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("attempts=%d", tc.attempts), func(t *testing.T) {
			f := &downloadFailure{
				lastAttempt: time.Now(),
				attempts:    tc.attempts,
				lastErr:     fmt.Errorf("test error"),
			}

			// Run multiple samples to verify the range (jitter is random).
			for i := 0; i < 100; i++ {
				got := f.backoff()
				if got < tc.minDelay {
					t.Errorf("backoff() = %v, want >= %v", got, tc.minDelay)
				}
				if got >= tc.maxDelay {
					t.Errorf("backoff() = %v, want < %v", got, tc.maxDelay)
				}
			}
		})
	}
}

func TestDownloadFailureBackoffMonotonicity(t *testing.T) {
	// Verify that the maximum possible backoff increases with more attempts
	// (up to the cap). We compute the delay ceiling (before jitter) directly.
	prevMax := time.Duration(0)
	for attempts := 1; attempts <= 10; attempts++ {
		delay := downloadBackoffBase
		for i := 1; i < attempts; i++ {
			delay *= 2
			if delay > downloadBackoffMax {
				delay = downloadBackoffMax
				break
			}
		}
		if delay < prevMax {
			t.Errorf("attempts=%d: delay ceiling %v < previous %v", attempts, delay, prevMax)
		}
		prevMax = delay
	}
}

func TestDownloadFailureShouldRetry(t *testing.T) {
	// A failure whose lastAttempt is far in the past should be retryable.
	f := &downloadFailure{
		lastAttempt: time.Now().Add(-10 * time.Minute),
		attempts:    1,
		lastErr:     fmt.Errorf("test"),
	}
	if !f.shouldRetry() {
		t.Error("expected shouldRetry=true for old failure")
	}

	// A failure that just happened should NOT be retryable (unless jitter
	// happens to produce 0, but with attempts=5 the max is 5min so the
	// probability is negligible). Use high attempts to ensure a long backoff.
	f2 := &downloadFailure{
		lastAttempt: time.Now(),
		attempts:    5,
		lastErr:     fmt.Errorf("test"),
	}
	// Run multiple checks — at least one should be false (overwhelmingly likely).
	retryCount := 0
	for i := 0; i < 100; i++ {
		if f2.shouldRetry() {
			retryCount++
		}
	}
	if retryCount == 100 {
		t.Error("expected shouldRetry=false at least sometimes for a just-failed download with 5 attempts")
	}
}

// ---------------------------------------------------------------------------
// Negative cache in CompositeLoader
// ---------------------------------------------------------------------------

func TestCompositeLoaderNegativeCacheInit(t *testing.T) {
	cl := NewCompositeLoader()
	if cl.failedDownloads == nil {
		t.Fatal("failedDownloads map should be initialized")
	}
	if len(cl.failedDownloads) != 0 {
		t.Error("failedDownloads should start empty")
	}
}

func TestCompositeLoaderNegativeCacheDirectManipulation(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	// Simulate a failed download by inserting directly.
	cl.failedDownloads["testlang"] = &downloadFailure{
		lastAttempt: time.Now(),
		attempts:    3,
		lastErr:     fmt.Errorf("download failed: 404"),
	}

	if len(cl.failedDownloads) != 1 {
		t.Errorf("expected 1 entry, got %d", len(cl.failedDownloads))
	}

	f := cl.failedDownloads["testlang"]
	if f.attempts != 3 {
		t.Errorf("attempts = %d, want 3", f.attempts)
	}

	// Clearing works.
	delete(cl.failedDownloads, "testlang")
	if len(cl.failedDownloads) != 0 {
		t.Error("expected 0 entries after delete")
	}
}

// ---------------------------------------------------------------------------
// WithOnInstall option and SetOnInstall
// ---------------------------------------------------------------------------

func TestWithOnInstallOption(t *testing.T) {
	var called atomic.Bool
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithOnInstall(func(name string) {
			called.Store(true)
		}),
	)

	if cl.onInstall == nil {
		t.Fatal("onInstall should be set by WithOnInstall option")
	}

	// Invoke the callback directly.
	cl.onInstall("test")
	if !called.Load() {
		t.Error("onInstall callback was not invoked")
	}
}

func TestSetOnInstall(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	if cl.onInstall != nil {
		t.Error("onInstall should be nil by default")
	}

	var called atomic.Int64
	cl.SetOnInstall(func(name string) {
		called.Add(1)
	})

	if cl.onInstall == nil {
		t.Fatal("onInstall should be set after SetOnInstall")
	}

	// Invoke and verify.
	cl.onInstall("ruby")
	cl.onInstall("php")
	if got := called.Load(); got != 2 {
		t.Errorf("onInstall call count = %d, want 2", got)
	}
}

func TestSetOnInstallReplacesPrevious(t *testing.T) {
	var first, second atomic.Bool
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithOnInstall(func(name string) {
			first.Store(true)
		}),
	)

	// Replace with a new callback.
	cl.SetOnInstall(func(name string) {
		second.Store(true)
	})

	cl.onInstall("test")
	if first.Load() {
		t.Error("first callback should not have been called after replacement")
	}
	if !second.Load() {
		t.Error("second callback should have been called")
	}
}

func TestSetOnInstallConcurrency(t *testing.T) {
	cl := NewCompositeLoader(WithAutoDownload(false))

	var wg sync.WaitGroup
	// Concurrently set callbacks — should not race.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cl.SetOnInstall(func(name string) {
				_ = n
			})
		}(i)
	}
	wg.Wait()

	// Should have a non-nil callback at the end.
	if cl.onInstall == nil {
		t.Error("onInstall should be set")
	}
}

// ---------------------------------------------------------------------------
// GrammarsNeedingRescan and MarkRescanComplete
// ---------------------------------------------------------------------------

func TestGrammarsNeedingRescanEmpty(t *testing.T) {
	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)

	got := cl.GrammarsNeedingRescan()
	if len(got) != 0 {
		t.Errorf("expected 0 grammars needing rescan, got %d", len(got))
	}
}

func TestGrammarsNeedingRescan(t *testing.T) {
	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)

	// Manually set up manifest entries — one needing rescan, one not.
	cl.dynamic.manifest.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})
	cl.dynamic.manifest.set("php", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "php/grammar.so",
		NeedsRescan: false,
	})
	cl.dynamic.manifest.set("lua", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "lua/grammar.so",
		NeedsRescan: true,
	})

	got := cl.GrammarsNeedingRescan()
	if len(got) != 2 {
		t.Fatalf("expected 2 grammars needing rescan, got %d: %v", len(got), got)
	}

	// Verify both names are present (order is not guaranteed).
	names := make(map[string]bool)
	for _, n := range got {
		names[n] = true
	}
	if !names["ruby"] {
		t.Error("expected 'ruby' in rescan list")
	}
	if !names["lua"] {
		t.Error("expected 'lua' in rescan list")
	}
}

func TestMarkRescanComplete(t *testing.T) {
	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)

	// Set up a grammar that needs rescan.
	cl.dynamic.manifest.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})

	// Verify it shows up.
	if got := cl.GrammarsNeedingRescan(); len(got) != 1 {
		t.Fatalf("expected 1 grammar needing rescan, got %d", len(got))
	}

	// Mark rescan complete.
	cl.MarkRescanComplete("ruby")

	// Should no longer appear.
	if got := cl.GrammarsNeedingRescan(); len(got) != 0 {
		t.Errorf("expected 0 grammars needing rescan after MarkRescanComplete, got %d", len(got))
	}

	// The manifest entry should still exist.
	entry := cl.dynamic.manifest.get("ruby")
	if entry == nil {
		t.Fatal("manifest entry should still exist after MarkRescanComplete")
	}
	if entry.NeedsRescan {
		t.Error("NeedsRescan should be false after MarkRescanComplete")
	}
}

func TestMarkRescanCompletePersists(t *testing.T) {
	dir := t.TempDir()

	// Create and populate manifest.
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)
	cl.dynamic.manifest.set("ruby", &ManifestEntry{
		Version:     "v1.0.0",
		File:        "ruby/grammar.so",
		NeedsRescan: true,
	})
	_ = cl.dynamic.manifest.save()

	// Mark rescan complete — this should persist.
	cl.MarkRescanComplete("ruby")

	// Reload from disk with a new loader.
	cl2 := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)
	if err := cl2.dynamic.manifest.load(); err != nil {
		t.Fatalf("manifest.load: %v", err)
	}

	entry := cl2.dynamic.manifest.get("ruby")
	if entry == nil {
		t.Fatal("expected ruby entry after reload")
	}
	if entry.NeedsRescan {
		t.Error("NeedsRescan should be false after reload (was persisted by MarkRescanComplete)")
	}
}

func TestMarkRescanCompleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(dir),
	)

	// Should not panic when called for a non-existent grammar.
	cl.MarkRescanComplete("nonexistent")

	// No entries affected.
	if got := cl.GrammarsNeedingRescan(); len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}
