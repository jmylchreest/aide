package grammar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// DynamicGrammarDef describes a grammar that can be dynamically loaded.
type DynamicGrammarDef struct {
	// SourceRepo is the GitHub repository (e.g., "tree-sitter/tree-sitter-ruby").
	SourceRepo string
	// CSymbol is the C function name exported by the shared library
	// (e.g., "tree_sitter_ruby").
	CSymbol string
	// LatestVersion is the latest known version of the grammar.
	// Used for downloads when no version is specified.
	LatestVersion string
}

// DynamicGrammars lists all grammars that can be dynamically loaded.
// These are NOT compiled into the binary — they are downloaded as shared
// libraries and loaded via purego.
var DynamicGrammars = map[string]*DynamicGrammarDef{
	"csharp": {
		SourceRepo: "tree-sitter/tree-sitter-c-sharp",
		CSymbol:    "tree_sitter_c_sharp",
	},
	"kotlin": {
		SourceRepo: "tree-sitter-grammars/tree-sitter-kotlin",
		CSymbol:    "tree_sitter_kotlin",
	},
	"scala": {
		SourceRepo: "tree-sitter/tree-sitter-scala",
		CSymbol:    "tree_sitter_scala",
	},
	"groovy": {
		SourceRepo: "amaanq/tree-sitter-groovy",
		CSymbol:    "tree_sitter_groovy",
	},
	"ruby": {
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	},
	"php": {
		SourceRepo: "tree-sitter/tree-sitter-php",
		CSymbol:    "tree_sitter_php",
	},
	"lua": {
		SourceRepo: "tree-sitter-grammars/tree-sitter-lua",
		CSymbol:    "tree_sitter_lua",
	},
	"elixir": {
		SourceRepo: "tree-sitter/tree-sitter-elixir",
		CSymbol:    "tree_sitter_elixir",
	},
	"bash": {
		SourceRepo: "tree-sitter/tree-sitter-bash",
		CSymbol:    "tree_sitter_bash",
	},
	"swift": {
		SourceRepo: "alex-pinkus/tree-sitter-swift",
		CSymbol:    "tree_sitter_swift",
	},
	"ocaml": {
		SourceRepo: "tree-sitter/tree-sitter-ocaml",
		CSymbol:    "tree_sitter_ocaml",
	},
	"elm": {
		SourceRepo: "elm-tooling/tree-sitter-elm",
		CSymbol:    "tree_sitter_elm",
	},
	"sql": {
		SourceRepo: "DerekStride/tree-sitter-sql",
		CSymbol:    "tree_sitter_sql",
	},
	"yaml": {
		SourceRepo: "tree-sitter-grammars/tree-sitter-yaml",
		CSymbol:    "tree_sitter_yaml",
	},
	"toml": {
		SourceRepo: "tree-sitter-grammars/tree-sitter-toml",
		CSymbol:    "tree_sitter_toml",
	},
	"hcl": {
		SourceRepo: "tree-sitter-grammars/tree-sitter-hcl",
		CSymbol:    "tree_sitter_hcl",
	},
	"protobuf": {
		SourceRepo: "coder3101/tree-sitter-proto",
		CSymbol:    "tree_sitter_proto",
	},
	"html": {
		SourceRepo: "tree-sitter/tree-sitter-html",
		CSymbol:    "tree_sitter_html",
	},
	"css": {
		SourceRepo: "tree-sitter/tree-sitter-css",
		CSymbol:    "tree_sitter_css",
	},
}

// DynamicLoader loads tree-sitter grammars from shared libraries at runtime.
// On Unix it uses purego (dlopen); on Windows it uses syscall.LoadDLL.
type DynamicLoader struct {
	mu       sync.RWMutex
	dir      string // Directory containing .so/.dylib/.dll files
	baseURL  string // URL template for downloads
	version  string // Version tag for downloads (e.g. "v0.0.39", "snapshot")
	manifest *manifestStore
	loaded   map[string]*tree_sitter.Language // Cache of loaded languages
	handles  map[string]uintptr               // Open library handles
}

// NewDynamicLoader creates a loader for the given grammar directory.
// If dir is empty, it defaults to ".aide/grammars/" relative to cwd.
func NewDynamicLoader(dir string) *DynamicLoader {
	if dir == "" {
		dir = filepath.Join(".aide", "grammars")
	}

	dl := &DynamicLoader{
		dir:      dir,
		baseURL:  DefaultGrammarURL,
		manifest: newManifestStore(dir),
		loaded:   make(map[string]*tree_sitter.Language),
		handles:  make(map[string]uintptr),
	}

	// Load manifest (ignore errors — it might not exist yet)
	_ = dl.manifest.load()

	return dl
}

// Load returns a Language by loading the shared library from disk.
// If the loader has a version set (from the running aide release) and the
// installed grammar's version differs, Load returns GrammarStaleError so the
// caller can re-download. Snapshot versions are not checked for staleness.
func (dl *DynamicLoader) Load(name string) (*tree_sitter.Language, error) {
	dl.mu.RLock()
	if lang, ok := dl.loaded[name]; ok {
		dl.mu.RUnlock()
		return lang, nil
	}
	dl.mu.RUnlock()

	dl.mu.Lock()
	defer dl.mu.Unlock()

	// Double-check after acquiring write lock
	if lang, ok := dl.loaded[name]; ok {
		return lang, nil
	}

	// Check manifest for the grammar
	entry := dl.manifest.get(name)
	if entry == nil {
		return nil, &GrammarNotFoundError{Name: name}
	}

	// Check version staleness: if the loader has a non-snapshot version set
	// and the installed grammar was built for a different version, report it
	// as stale so the CompositeLoader can re-download.
	if dl.version != "" && dl.version != "snapshot" &&
		entry.Version != "" && entry.Version != "snapshot" &&
		entry.Version != dl.version {
		return nil, &GrammarStaleError{
			Name:             name,
			InstalledVersion: entry.Version,
			WantVersion:      dl.version,
		}
	}

	// Load the shared library
	libPath := filepath.Join(dl.dir, entry.File)
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("grammar library not found at %s: %w", libPath, err)
	}

	// openAndLoadLanguage is platform-specific (dynamic_unix.go / dynamic_windows.go).
	lang, handle, err := openAndLoadLanguage(libPath, entry.CSymbol)
	if err != nil {
		return nil, fmt.Errorf("grammar %q: %w", name, err)
	}

	dl.loaded[name] = lang
	dl.handles[name] = handle
	return lang, nil
}

// Download fetches a grammar pack archive (.tar.gz) from GitHub and extracts
// it locally. The archive contains the shared library and a pack.json with
// language metadata. If a grammar is already installed, it is replaced.
func (dl *DynamicLoader) Download(ctx context.Context, name string, def *DynamicGrammarDef) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	// Determine version — prefer loader-level version (from aide release),
	// then grammar-specific version, then "snapshot" as a safe fallback.
	version := dl.version
	if version == "" {
		version = def.LatestVersion
	}
	if version == "" {
		version = "snapshot"
	}

	// Clean up any existing installation before re-downloading.
	if dl.manifest.get(name) != nil {
		_ = os.RemoveAll(filepath.Join(dl.dir, name))
	}

	// Evict from in-memory cache so the new library gets loaded fresh.
	delete(dl.loaded, name)
	delete(dl.handles, name)

	// Download and extract the archive.
	sha256sum, hasPack, err := downloadAndExtractGrammarPack(ctx, dl.baseURL, name, version, dl.dir)
	if err != nil {
		return &DownloadFailedError{Name: name, Err: err}
	}

	// Load pack.json into the PackRegistry if present.
	if hasPack {
		packDir := filepath.Join(dl.dir, name)
		if loadErr := DefaultPackRegistry().LoadFromDir(packDir); loadErr != nil {
			// Non-fatal: pack metadata is supplementary. Log but continue.
			_ = loadErr
		}
	}

	// Update manifest.
	dl.manifest.set(name, &ManifestEntry{
		Version:     version,
		File:        LibraryFilename(name),
		SHA256:      sha256sum,
		CSymbol:     def.CSymbol,
		HasPack:     hasPack,
		InstalledAt: time.Now(),
	})
	dl.manifest.setAideVersion(dl.version)

	return dl.manifest.save()
}

// Installed returns info about all locally installed dynamic grammars.
func (dl *DynamicLoader) Installed() []GrammarInfo {
	entries := dl.manifest.entries()
	infos := make([]GrammarInfo, 0, len(entries))
	for name, entry := range entries {
		infos = append(infos, GrammarInfo{
			Name:        name,
			Version:     entry.Version,
			BuiltIn:     false,
			Path:        filepath.Join(dl.dir, entry.File),
			InstalledAt: entry.InstalledAt,
		})
	}
	return infos
}

// Remove deletes a grammar's shared library, pack data, and manifest entry.
func (dl *DynamicLoader) Remove(name string) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	// Close the library handle if loaded
	delete(dl.loaded, name)
	delete(dl.handles, name)

	// Remove the grammar subdirectory (contains library + pack.json).
	grammarDir := filepath.Join(dl.dir, name)
	_ = os.RemoveAll(grammarDir)

	// Remove from manifest
	dl.manifest.remove(name)
	return dl.manifest.save()
}
