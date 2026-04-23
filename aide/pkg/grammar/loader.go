// Package grammar provides a hybrid grammar loading system for tree-sitter languages.
//
// It supports two modes:
//   - Compiled-in (built-in): 10 core grammars linked via CGO at build time
//   - Dynamic: grammars downloaded as shared libraries and loaded via purego at runtime
//
// The CompositeLoader tries built-in first, then dynamic, then auto-downloads.
package grammar

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"golang.org/x/sync/singleflight"
)

const (
	// downloadBackoffBase is the initial backoff delay after a grammar
	// download failure before the next attempt is allowed.
	downloadBackoffBase = 30 * time.Second

	// downloadBackoffMax caps the exponential backoff for failed downloads.
	downloadBackoffMax = 5 * time.Minute
)

// Loader provides access to tree-sitter language grammars.
type Loader interface {
	// Load returns the Language for the given name.
	// For compiled-in grammars, returns immediately.
	// For dynamic grammars, checks local cache then optionally downloads.
	Load(ctx context.Context, name string) (*tree_sitter.Language, error)

	// Available returns all grammar names that can be loaded (compiled-in + downloadable).
	Available() []string

	// Installed returns grammars currently available locally (compiled-in + cached).
	Installed() []GrammarInfo

	// Install downloads a grammar to the local cache.
	Install(ctx context.Context, name string) error

	// Remove deletes a grammar from the local cache.
	Remove(name string) error
}

// GrammarInfo describes an installed or available grammar.
type GrammarInfo struct {
	Name        string    `json:"name"`
	Version     string    `json:"version,omitempty"`
	AbiVersion  uint32    `json:"abi_version,omitempty"`
	BuiltIn     bool      `json:"built_in"`
	Path        string    `json:"path,omitempty"` // Empty for built-in
	InstalledAt time.Time `json:"installed_at,omitempty"`
}

// BuiltinProvider is a function that returns an unsafe.Pointer to a TSLanguage.
// This is the signature exposed by tree-sitter grammar Go bindings.
type BuiltinProvider func() unsafe.Pointer

// GrammarNotFoundError is returned when a grammar is not available.
type GrammarNotFoundError struct {
	Name string
}

func (e *GrammarNotFoundError) Error() string {
	return fmt.Sprintf("grammar %q not found", e.Name)
}

// DownloadFailedError is returned when a grammar download fails.
type DownloadFailedError struct {
	Name string
	Err  error
}

func (e *DownloadFailedError) Error() string {
	return fmt.Sprintf("failed to download grammar %q: %v", e.Name, e.Err)
}

func (e *DownloadFailedError) Unwrap() error {
	return e.Err
}

// IncompatibleABIError is returned when a grammar's ABI version is outside
// the compatible range for the current tree-sitter runtime.
type IncompatibleABIError struct {
	Name       string
	AbiVersion uint32
	MinVersion uint32
	MaxVersion uint32
}

func (e *IncompatibleABIError) Error() string {
	return fmt.Sprintf(
		"grammar %q has ABI version %d, but runtime supports %d–%d",
		e.Name, e.AbiVersion, e.MinVersion, e.MaxVersion,
	)
}

// GrammarMetadataOnlyError is returned when a pack exists but has no tree-sitter
// grammar binary (CSymbol is empty). These packs provide file-detection metadata
// (extensions, filenames) but cannot be loaded for parsing or analysis.
type GrammarMetadataOnlyError struct {
	Name string
}

func (e *GrammarMetadataOnlyError) Error() string {
	return fmt.Sprintf("grammar %q is metadata-only (no tree-sitter parser available)", e.Name)
}

// GrammarStaleError is returned when an installed grammar's version does not
// match the currently running aide version. The CompositeLoader handles this
// by re-downloading the grammar automatically when auto-download is enabled.
type GrammarStaleError struct {
	Name             string
	InstalledVersion string
	WantVersion      string
}

func (e *GrammarStaleError) Error() string {
	return fmt.Sprintf(
		"grammar %q is stale (installed: %s, want: %s)",
		e.Name, e.InstalledVersion, e.WantVersion,
	)
}

// CompositeLoader tries multiple loaders in priority order:
// 1. Built-in grammars (compiled-in via CGO)
// 2. Dynamic grammars (loaded from local cache via purego)
// 3. Auto-download (fetches from GitHub, caches, then loads)
// downloadFailure tracks a failed download attempt for negative caching.
type downloadFailure struct {
	lastAttempt time.Time
	attempts    int
	lastErr     error
}

// backoff returns the current backoff duration for this failure using
// exponential backoff with full jitter, matching the httputil pattern.
func (f *downloadFailure) backoff() time.Duration {
	delay := downloadBackoffBase
	for i := 1; i < f.attempts; i++ {
		delay *= 2
		if delay > downloadBackoffMax {
			delay = downloadBackoffMax
			break
		}
	}
	// Full jitter: uniform random in [0, delay].
	if delay > 0 {
		delay = time.Duration(rand.Int64N(int64(delay)))
	}
	return delay
}

// shouldRetry returns true if enough time has passed since the last
// failed attempt to allow another download attempt.
func (f *downloadFailure) shouldRetry() bool {
	return time.Since(f.lastAttempt) >= f.backoff()
}

// CompositeLoader combines built-in grammars, a dynamic loader for
// downloaded grammars, and optional auto-download.
type CompositeLoader struct {
	builtin  *BuiltinRegistry
	dynamic  *DynamicLoader
	autoLoad bool // Whether to auto-download missing grammars
	logger   *log.Logger

	mu              sync.RWMutex
	cache           map[string]*tree_sitter.Language // Loaded language cache
	failedDownloads map[string]*downloadFailure      // Negative cache for failed downloads

	// installGroup collapses concurrent Install(name) calls for the same
	// grammar into a single download. Without this, two watcher events for
	// the same new language could each trigger Download, and the second
	// Download would dlclose the handle the first one just handed out —
	// SEGV'ing any in-flight parse. See cmd_mcp.go:rescanForGrammar.
	installGroup singleflight.Group

	onInstall func(name string) // Callback fired after a grammar is newly installed
}

// CompositeLoaderOption configures the CompositeLoader.
type CompositeLoaderOption func(*CompositeLoader)

// WithAutoDownload enables automatic downloading of missing grammars.
func WithAutoDownload(enabled bool) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		cl.autoLoad = enabled
	}
}

// WithGrammarDir sets the directory for storing downloaded grammars.
// Defaults to ".aide/grammars/" relative to the project root.
func WithGrammarDir(dir string) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		// Preserve any baseURL or version already set on the previous loader.
		prevURL := cl.dynamic.baseURL
		prevVer := cl.dynamic.version
		cl.dynamic = NewDynamicLoader(dir)
		cl.dynamic.baseURL = prevURL
		cl.dynamic.version = prevVer
	}
}

// WithBaseURL sets the URL template for downloading grammar assets.
// Supported placeholders: {version}, {asset}, {name}, {os}, {arch}.
// Defaults to DefaultGrammarURL.
func WithBaseURL(urlTemplate string) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		if urlTemplate != "" {
			cl.dynamic.baseURL = urlTemplate
		}
	}
}

// WithVersion sets the version tag used when downloading grammar assets.
// For release builds this should be the release tag (e.g. "v0.0.39").
// For snapshot/dev builds use "snapshot". An empty string falls back to
// the grammar definition's LatestVersion or "snapshot".
func WithVersion(v string) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		cl.dynamic.version = v
	}
}

// WithLogger sets an optional logger for the CompositeLoader. When set,
// grammar loading events (auto-downloads, staleness detection, first-match
// during scans) are logged. When nil (default), no output is produced.
func WithLogger(l *log.Logger) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		cl.logger = l
	}
}

// WithOnInstall sets a callback that fires after a grammar is newly
// installed (downloaded). The callback receives the grammar name.
// Use this to trigger re-indexing of files matching the new grammar.
func WithOnInstall(fn func(name string)) CompositeLoaderOption {
	return func(cl *CompositeLoader) {
		cl.onInstall = fn
	}
}

// logf logs a message if a logger is configured; no-op otherwise.
func (cl *CompositeLoader) logf(format string, args ...any) {
	if cl.logger != nil {
		cl.logger.Printf(format, args...)
	}
}

// NewCompositeLoader creates a new CompositeLoader with the given options.
func NewCompositeLoader(opts ...CompositeLoaderOption) *CompositeLoader {
	cl := &CompositeLoader{
		builtin:         NewBuiltinRegistry(),
		dynamic:         NewDynamicLoader(""), // Default dir, resolved lazily
		autoLoad:        true,                 // Auto-download enabled by default
		cache:           make(map[string]*tree_sitter.Language),
		failedDownloads: make(map[string]*downloadFailure),
	}

	for _, opt := range opts {
		opt(cl)
	}

	return cl
}

// Load returns the Language for the given name.
func (cl *CompositeLoader) Load(ctx context.Context, name string) (*tree_sitter.Language, error) {
	// Early check: metadata-only packs have no parser and can never be loaded.
	if pack := DefaultPackRegistry().Get(name); pack != nil && !pack.HasParser() {
		return nil, &GrammarMetadataOnlyError{Name: name}
	}

	// Check cache first
	cl.mu.RLock()
	if lang, ok := cl.cache[name]; ok {
		cl.mu.RUnlock()
		return lang, nil
	}
	cl.mu.RUnlock()

	// 1. Try built-in grammars
	if lang, err := cl.builtin.Load(name); err == nil {
		cl.mu.Lock()
		cl.cache[name] = lang
		cl.mu.Unlock()
		return lang, nil
	}

	// 2. Try dynamic loader (local cache)
	lang, dynErr := cl.dynamic.Load(name)
	if dynErr == nil {
		cl.mu.Lock()
		cl.cache[name] = lang
		cl.mu.Unlock()
		return lang, nil
	}

	// 3. Auto-download if enabled — triggered for missing OR stale grammars
	if cl.autoLoad {
		var staleErr *GrammarStaleError
		var notFoundErr *GrammarNotFoundError
		if errors.As(dynErr, &staleErr) {
			cl.logf("grammar %q is stale (installed: %s, want: %s), re-downloading",
				staleErr.Name, staleErr.InstalledVersion, staleErr.WantVersion)
		} else if errors.As(dynErr, &notFoundErr) {
			cl.logf("grammar %q not installed, auto-downloading", notFoundErr.Name)
		}
		if errors.As(dynErr, &staleErr) || errors.As(dynErr, &notFoundErr) {
			// Check negative cache — skip download if a recent attempt failed
			// and the backoff period hasn't elapsed yet.
			cl.mu.RLock()
			failure := cl.failedDownloads[name]
			cl.mu.RUnlock()
			if failure != nil && !failure.shouldRetry() {
				cl.logf("grammar %q download suppressed (attempt %d, backoff not elapsed): %v",
					name, failure.attempts, failure.lastErr)
				return nil, failure.lastErr
			}

			// Coalesce concurrent installs for the same name. All waiters
			// observe the same error (or nil) from the one goroutine that
			// actually runs the download.
			_, installErr, _ := cl.installGroup.Do(name, func() (interface{}, error) {
				return nil, cl.Install(ctx, name)
			})
			if installErr != nil {
				// Record failure for negative caching with backoff.
				cl.mu.Lock()
				prev := cl.failedDownloads[name]
				attempts := 1
				if prev != nil {
					attempts = prev.attempts + 1
				}
				cl.failedDownloads[name] = &downloadFailure{
					lastAttempt: time.Now(),
					attempts:    attempts,
					lastErr:     installErr,
				}
				cl.mu.Unlock()
				cl.logf("grammar %q download failed (attempt %d): %v", name, attempts, installErr)
				return nil, installErr
			}

			// Download succeeded — clear any negative cache entry.
			cl.mu.Lock()
			delete(cl.failedDownloads, name)
			cl.mu.Unlock()

			cl.logf("grammar %q downloaded successfully", name)
			// Try loading again from dynamic cache
			lang, err := cl.dynamic.Load(name)
			if err == nil {
				cl.mu.Lock()
				cl.cache[name] = lang
				cl.mu.Unlock()
				return lang, nil
			}
			// Download succeeded but loading failed (e.g. Dlopen error) —
			// propagate the actual error rather than masking it.
			return nil, err
		}
		// Dynamic loader returned an error that is neither stale nor not-found
		// (e.g. a dlopen failure). Propagate the real error.
		return nil, dynErr
	}

	// Auto-download disabled — if dynamic loader returned a real error (not
	// stale / not-found), propagate it rather than masking it.
	var staleErr *GrammarStaleError
	var notFoundErr *GrammarNotFoundError
	if dynErr != nil && !errors.As(dynErr, &staleErr) && !errors.As(dynErr, &notFoundErr) {
		return nil, dynErr
	}

	return nil, &GrammarNotFoundError{Name: name}
}

// Available returns all grammar names that can be loaded.
func (cl *CompositeLoader) Available() []string {
	seen := make(map[string]bool)
	var names []string

	for _, name := range cl.builtin.Names() {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	for name := range DefaultPackRegistry().DynamicPacks() {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names
}

// Installed returns grammars currently available locally.
func (cl *CompositeLoader) Installed() []GrammarInfo {
	builtinNames := cl.builtin.Names()
	dynamicInfos := cl.dynamic.Installed()
	infos := make([]GrammarInfo, 0, len(builtinNames)+len(dynamicInfos))

	// Built-in grammars
	for _, name := range builtinNames {
		infos = append(infos, GrammarInfo{
			Name:    name,
			BuiltIn: true,
		})
	}

	// Dynamic grammars from manifest
	infos = append(infos, dynamicInfos...)

	return infos
}

// Install downloads a grammar to the local cache.
// On success, fires the onInstall callback if set.
// If the grammar is already installed and matches the current version,
// Install is a no-op and does not fire the callback.
func (cl *CompositeLoader) Install(ctx context.Context, name string) error {
	// Don't download built-in grammars
	if cl.builtin.Has(name) {
		return nil
	}

	pack := DefaultPackRegistry().Get(name)
	if pack == nil {
		return &GrammarNotFoundError{Name: name}
	}
	if !pack.HasParser() {
		return &GrammarMetadataOnlyError{Name: name}
	}

	// Idempotency: skip re-download if the grammar is already installed at
	// the current version. Stale entries still fall through to Download so
	// they get refreshed.
	if cl.dynamic.isInstalledFresh(name) {
		return nil
	}

	if err := cl.dynamic.Download(ctx, name, pack); err != nil {
		return err
	}

	// Fire the install callback (e.g. to trigger re-indexing of matching files).
	if cl.onInstall != nil {
		cl.onInstall(name)
	}

	return nil
}

// Remove deletes a grammar from the local cache.
func (cl *CompositeLoader) Remove(name string) error {
	cl.mu.Lock()
	delete(cl.cache, name)
	cl.mu.Unlock()

	return cl.dynamic.Remove(name)
}

// GrammarsNeedingRescan returns grammar names that were installed but whose
// project re-scan did not complete (e.g. process restarted mid-scan).
func (cl *CompositeLoader) GrammarsNeedingRescan() []string {
	entries := cl.dynamic.manifest.entries()
	var names []string
	for name, entry := range entries {
		if entry.NeedsRescan {
			names = append(names, name)
		}
	}
	return names
}

// MarkRescanComplete clears the NeedsRescan flag for a grammar in the
// manifest and persists the change to disk.
func (cl *CompositeLoader) MarkRescanComplete(name string) {
	cl.dynamic.manifest.mu.Lock()
	if entry, ok := cl.dynamic.manifest.data.Grammars[name]; ok {
		entry.NeedsRescan = false
	}
	cl.dynamic.manifest.mu.Unlock()
	_ = cl.dynamic.manifest.save()
}

// SetOnInstall sets or replaces the callback fired after a grammar is newly
// installed. This is useful when the callback depends on objects that are
// created after the loader (e.g. the code indexer).
func (cl *CompositeLoader) SetOnInstall(fn func(name string)) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.onInstall = fn
}

// GenerateLockFile creates a LockFile from the current dynamic grammar manifest.
func (cl *CompositeLoader) GenerateLockFile() *LockFile {
	return LockFileFromManifest(cl.dynamic.manifest)
}

// InstallFromLock installs all grammars listed in a lock file that are not
// already present locally. Returns the names of grammars that were installed.
func (cl *CompositeLoader) InstallFromLock(ctx context.Context, lf *LockFile) ([]string, error) {
	installed := make(map[string]bool)
	for _, info := range cl.Installed() {
		installed[info.Name] = true
	}

	var installedNames []string
	var errs []error
	for _, name := range lf.Names() {
		if installed[name] {
			continue
		}
		if err := cl.Install(ctx, name); err != nil {
			// Record the failure but continue installing remaining grammars.
			errs = append(errs, fmt.Errorf("installing %s: %w", name, err))
			continue
		}
		installedNames = append(installedNames, name)
	}
	if len(errs) > 0 {
		return installedNames, fmt.Errorf("%d grammar(s) failed to install: %w", len(errs), errors.Join(errs...))
	}
	return installedNames, nil
}
