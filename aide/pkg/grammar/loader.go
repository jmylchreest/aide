// Package grammar provides a hybrid grammar loading system for tree-sitter languages.
//
// It supports two modes:
//   - Compiled-in (built-in): 9 core grammars linked via CGO at build time
//   - Dynamic: grammars downloaded as shared libraries and loaded via purego at runtime
//
// The CompositeLoader tries built-in first, then dynamic, then auto-downloads.
package grammar

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
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

// ErrGrammarNotFound is returned when a grammar is not available.
type ErrGrammarNotFound struct {
	Name string
}

func (e *ErrGrammarNotFound) Error() string {
	return fmt.Sprintf("grammar %q not found", e.Name)
}

// ErrDownloadFailed is returned when a grammar download fails.
type ErrDownloadFailed struct {
	Name string
	Err  error
}

func (e *ErrDownloadFailed) Error() string {
	return fmt.Sprintf("failed to download grammar %q: %v", e.Name, e.Err)
}

func (e *ErrDownloadFailed) Unwrap() error {
	return e.Err
}

// ErrIncompatibleABI is returned when a grammar's ABI version is outside
// the compatible range for the current tree-sitter runtime.
type ErrIncompatibleABI struct {
	Name       string
	AbiVersion uint32
	MinVersion uint32
	MaxVersion uint32
}

func (e *ErrIncompatibleABI) Error() string {
	return fmt.Sprintf(
		"grammar %q has ABI version %d, but runtime supports %d–%d",
		e.Name, e.AbiVersion, e.MinVersion, e.MaxVersion,
	)
}

// CompositeLoader tries multiple loaders in priority order:
// 1. Built-in grammars (compiled-in via CGO)
// 2. Dynamic grammars (loaded from local cache via purego)
// 3. Auto-download (fetches from GitHub, caches, then loads)
type CompositeLoader struct {
	builtin  *BuiltinRegistry
	dynamic  *DynamicLoader
	autoLoad bool // Whether to auto-download missing grammars

	mu    sync.RWMutex
	cache map[string]*tree_sitter.Language // Loaded language cache
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
		cl.dynamic = NewDynamicLoader(dir)
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

// NewCompositeLoader creates a new CompositeLoader with the given options.
func NewCompositeLoader(opts ...CompositeLoaderOption) *CompositeLoader {
	cl := &CompositeLoader{
		builtin:  NewBuiltinRegistry(),
		dynamic:  NewDynamicLoader(""), // Default dir, resolved lazily
		autoLoad: true,                 // Auto-download enabled by default
		cache:    make(map[string]*tree_sitter.Language),
	}

	for _, opt := range opts {
		opt(cl)
	}

	return cl
}

// Load returns the Language for the given name.
func (cl *CompositeLoader) Load(ctx context.Context, name string) (*tree_sitter.Language, error) {
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
	if lang, err := cl.dynamic.Load(name); err == nil {
		cl.mu.Lock()
		cl.cache[name] = lang
		cl.mu.Unlock()
		return lang, nil
	}

	// 3. Auto-download if enabled
	if cl.autoLoad {
		if err := cl.Install(ctx, name); err != nil {
			return nil, err
		}
		// Try loading again from dynamic cache
		if lang, err := cl.dynamic.Load(name); err == nil {
			cl.mu.Lock()
			cl.cache[name] = lang
			cl.mu.Unlock()
			return lang, nil
		}
	}

	return nil, &ErrGrammarNotFound{Name: name}
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

	for name := range DynamicGrammars {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names
}

// Installed returns grammars currently available locally.
func (cl *CompositeLoader) Installed() []GrammarInfo {
	var infos []GrammarInfo

	// Built-in grammars
	for _, name := range cl.builtin.Names() {
		infos = append(infos, GrammarInfo{
			Name:    name,
			BuiltIn: true,
		})
	}

	// Dynamic grammars from manifest
	for _, info := range cl.dynamic.Installed() {
		infos = append(infos, info)
	}

	return infos
}

// Install downloads a grammar to the local cache.
func (cl *CompositeLoader) Install(ctx context.Context, name string) error {
	// Don't download built-in grammars
	if cl.builtin.Has(name) {
		return nil
	}

	grammarDef, ok := DynamicGrammars[name]
	if !ok {
		return &ErrGrammarNotFound{Name: name}
	}

	return cl.dynamic.Download(ctx, name, grammarDef)
}

// Remove deletes a grammar from the local cache.
func (cl *CompositeLoader) Remove(name string) error {
	cl.mu.Lock()
	delete(cl.cache, name)
	cl.mu.Unlock()

	return cl.dynamic.Remove(name)
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
	for _, name := range lf.Names() {
		if installed[name] {
			continue
		}
		if err := cl.Install(ctx, name); err != nil {
			return installedNames, fmt.Errorf("installing %s: %w", name, err)
		}
		installedNames = append(installedNames, name)
	}
	return installedNames, nil
}
