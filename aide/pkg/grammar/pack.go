package grammar

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

//go:embed packs/*/pack.json packs/index.json packs/index.d/*.json
var embeddedPacks embed.FS

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *PackRegistry
	errDefaultRegistry  error
)

// DefaultPackRegistry returns a lazily-initialised singleton PackRegistry
// pre-loaded with all embedded packs. It is safe for concurrent use.
// All standalone functions (DetectLanguage, SupportedFile, etc.) use this.
func DefaultPackRegistry() *PackRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry, errDefaultRegistry = NewPackRegistry()
		if errDefaultRegistry != nil {
			// This should never happen with valid embedded data.
			// Fall back to an empty registry rather than panicking.
			defaultRegistry = &PackRegistry{
				packs:          make(map[string]*Pack),
				extLookup:      make(map[string]string),
				filenameLookup: make(map[string]string),
				shebangLookup:  make(map[string]string),
				aliasLookup:    make(map[string]string),
			}
		}
	})
	return defaultRegistry
}

// Pack is the in-memory representation of a pack.json file.
// It contains all per-language metadata aide needs for analysis.
type Pack struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	CSymbol       string `json:"c_symbol,omitempty"`
	SourceRepo    string `json:"source_repo,omitempty"`
	SourceTag     string `json:"source_tag,omitempty"`
	SourceSrcDir  string `json:"source_src_dir,omitempty"`
	// Notes is a free-form human-readable description of the pack's
	// applicability and any intentional gaps in the schema. Used to
	// document, for example, that a markup pack deliberately omits
	// test/framework conventions because none exist for that language.
	Notes string `json:"notes,omitempty"`

	Meta         PackMeta          `json:"meta"`
	Queries      PackQueries       `json:"queries,omitempty"`
	Complexity   *PackComplexity   `json:"complexity,omitempty"`
	Imports      *PackImports      `json:"imports,omitempty"`
	Tokenisation *PackTokenisation `json:"tokenisation,omitempty"`
	Entrypoints  *PackEntrypoints  `json:"entrypoints,omitempty"`
	Security     *PackSecurity     `json:"security,omitempty"`
	Deadcode     *PackDeadcode     `json:"deadcode,omitempty"`
	Comments     *PackComments     `json:"comments,omitempty"`
	Files        *PackFiles        `json:"files,omitempty"`
}

// PackComments declares a language's comment delimiters. Used by analyzers that
// need to distinguish comment text from code (todos, security skip-comment heuristic).
type PackComments struct {
	// Line lists prefixes that start a single-line comment, e.g. ["//"], ["#"], ["--"].
	Line []string `json:"line,omitempty"`
	// Block lists [open, close] delimiter pairs for multi-line comments, e.g. [["/*","*/"]].
	Block [][2]string `json:"block,omitempty"`
}

// PackDeadcode configures language-specific behaviour for the deadcode analyzer.
type PackDeadcode struct {
	// SuppressionPatterns are regexes matched against the line(s) immediately
	// above a symbol's declaration. A match suppresses the deadcode finding for
	// that symbol. Use this for `//nolint:unused`, `// aide:keep`, JSDoc
	// `@public`, etc. Matched against the raw text of each preceding line.
	SuppressionPatterns []string `json:"suppression_patterns,omitempty"`
	// BlockSuppressionPatterns are regexes that, when matched on a line, mark
	// the entire braced block that opens after that line as suppressed. Any
	// symbol whose declaration line falls within that block is excluded from
	// dead-code findings. Use this for `#[cfg(test)] mod tests { ... }`,
	// `#[zbus::interface] impl Foo { ... }`, `#[pymethods] impl Foo { ... }`,
	// etc. — attributes on a mod/impl/fn whose contents are reflective entry
	// points.
	BlockSuppressionPatterns []string `json:"block_suppression_patterns,omitempty"`
	// ExportedRule selects how exported-ness is determined from a symbol's name.
	// Recognised values:
	//   - "first_char_uppercase"  Go/Java/C# convention
	//   - "no_leading_underscore" Python/Ruby convention
	// Empty means the analyzer has no language hint and treats every symbol as
	// non-exported (i.e. all are subject to dead-code analysis).
	ExportedRule string `json:"exported_rule,omitempty"`
	// TestFilePatterns are doublestar-style globs (matched against the
	// symbol's project-relative FilePath) that mark a file as a test file.
	// Symbols in test files are skipped by the deadcode analyzer because
	// their callers are typically the test harness, not other source code.
	TestFilePatterns []string `json:"test_file_patterns,omitempty"`
	// TestFunctionPrefixes are case-sensitive name prefixes that mark a
	// function as a test/benchmark/example/fuzz target invoked by tooling
	// rather than by other code (Go's "Test", "Benchmark", "Example",
	// "Fuzz"; Python's "test_"; etc.).
	TestFunctionPrefixes []string `json:"test_function_prefixes,omitempty"`
	// FrameworkHookNames are case-insensitive symbol names that act as
	// framework lifecycle hooks (setup, teardown, beforeEach, ...) or
	// language-level entry points (main, init). They're called by the
	// runtime/framework, not by user code, so the deadcode analyzer
	// shouldn't flag them as unreferenced.
	FrameworkHookNames []string `json:"framework_hook_names,omitempty"`
}

// PackFiles groups per-language file-classification patterns shared across
// analyzers. Currently used by survey's entrypoints walker; can be consumed
// elsewhere as needs arise.
type PackFiles struct {
	// GeneratedFilePatterns are doublestar-style globs marking files that
	// were emitted by code generators (protobuf stubs, gRPC gateways, etc.)
	// and should be excluded from analyses where signal-from-author-intent
	// is what's wanted (entrypoints, deadcode, complexity, ...).
	GeneratedFilePatterns []string `json:"generated_file_patterns,omitempty"`
}

// HasParser reports whether this pack has a tree-sitter grammar binary (CSymbol != "").
// Packs without a parser are metadata-only (e.g., dockerfile, json) — they provide
// file-detection metadata but cannot be loaded for parsing or analysis.
func (p *Pack) HasParser() bool {
	return p.CSymbol != ""
}

// PackMeta holds file-detection metadata for a language.
type PackMeta struct {
	Extensions []string `json:"extensions"`
	Filenames  []string `json:"filenames,omitempty"`
	Shebangs   []string `json:"shebangs,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
}

// PackQueries holds tree-sitter query strings for symbol extraction.
type PackQueries struct {
	Tags string `json:"tags,omitempty"`
	Refs string `json:"refs,omitempty"`
}

// PackComplexity holds complexity analysis configuration for a language.
type PackComplexity struct {
	FuncNodeTypes []string `json:"func_node_types"`
	BranchTypes   []string `json:"branch_types"`
	NameField     string   `json:"name_field"`
}

// PackImports holds import extraction configuration for a language.
type PackImports struct {
	Patterns   []ImportPattern `json:"patterns"`
	BlockStart string          `json:"block_start,omitempty"`
	BlockEnd   string          `json:"block_end,omitempty"`
}

// ImportPattern defines a regex pattern for extracting import paths.
type ImportPattern struct {
	Regex   string `json:"regex"`
	Group   int    `json:"group"`
	Context string `json:"context,omitempty"` // "single", "block", or "" (any)
}

// PackTokenisation holds clone-detection tokenisation configuration.
type PackTokenisation struct {
	IdentifierTypes []string `json:"identifier_types,omitempty"`
	LiteralTypes    []string `json:"literal_types,omitempty"`
	KeywordTypes    []string `json:"keyword_types,omitempty"`
}

// PackSecurity holds security analysis rules for a language.
// Rules can use either regex patterns (fast, simple) or tree-sitter queries (precise, structural).
type PackSecurity struct {
	Rules []SecurityRule `json:"rules"`
}

// SecurityRule defines a single security pattern to detect in source code.
type SecurityRule struct {
	ID          string `json:"id"`                    // Unique identifier, e.g., "go-sql-concat"
	Name        string `json:"name"`                  // Human-readable name
	Severity    string `json:"severity"`              // "critical", "warning", "info"
	Category    string `json:"category"`              // "injection", "xss", "traversal", "crypto", "exec", "deserialize", "ssrf"
	Pattern     string `json:"pattern,omitempty"`     // Regex pattern (simple mode)
	Query       string `json:"query,omitempty"`       // Tree-sitter S-expression query (precise mode)
	Description string `json:"description,omitempty"` // Guidance for the LLM / developer
}

// PackEntrypoints defines entry point patterns for a language.
// These drive the survey entrypoints analyzer to find main functions,
// HTTP handlers, CLI roots, gRPC services, and other entry points.
type PackEntrypoints struct {
	// Symbols are code-index-based symbol searches (FindSymbols queries).
	Symbols []EntrypointSymbol `json:"symbols,omitempty"`
	// Refs are code-index-based reference searches (FindReferences queries).
	Refs []EntrypointRef `json:"refs,omitempty"`
	// FilePatterns are file-scan fallback patterns when no code index is available.
	FilePatterns []EntrypointFilePattern `json:"file_patterns,omitempty"`
}

// EntrypointSymbol defines a code-index symbol search for entry point detection.
type EntrypointSymbol struct {
	// Name is the symbol name to search for (e.g., "main", "init").
	Name string `json:"name"`
	// Kind is the symbol kind filter (e.g., "function", "method"). Empty matches any kind.
	Kind string `json:"kind,omitempty"`
	// Type classifies the entry point: "main", "init", "test", etc.
	Type string `json:"type"`
	// Label is a human-readable description (e.g., "Go main function").
	Label string `json:"label"`
	// Exclude is a regex applied to file paths to skip (e.g., "_test\\.go$|vendor/").
	Exclude string `json:"exclude,omitempty"`
	// NameMatch is a regex the full symbol name must match (e.g., "^Test[A-Z]" for Go tests).
	NameMatch string `json:"name_match,omitempty"`
	// FileMatch is a regex the file path must match (e.g., "_test\\.go$" for test files).
	FileMatch string `json:"file_match,omitempty"`
}

// EntrypointRef defines a code-index reference search for entry point detection.
type EntrypointRef struct {
	// Name is the reference name to search for (e.g., "HandleFunc", "Execute").
	Name string `json:"name"`
	// RefKind is the reference kind filter: "call" or "type_ref". Empty defaults to "call".
	RefKind string `json:"ref_kind,omitempty"`
	// Type classifies the entry point: "http_handler", "grpc_service", "cli_root", etc.
	Type string `json:"type"`
	// Label is a human-readable description (e.g., "HTTP handler registration").
	Label string `json:"label"`
	// Qualifier is a regex that the full symbol string must match (e.g., "http|mux|router").
	// This filters the portion before the method name to reduce false positives.
	Qualifier string `json:"qualifier,omitempty"`
	// NameMatch is a regex the full reference name must match (e.g., "^Register.*Server$").
	NameMatch string `json:"name_match,omitempty"`
	// Exclude is a regex applied to file paths to skip.
	Exclude string `json:"exclude,omitempty"`
}

// EntrypointFilePattern defines a file-scan fallback pattern for entry point detection.
// Used when no code index is available.
type EntrypointFilePattern struct {
	// FileMatch is a glob pattern or filename (e.g., "*.go", "main.rs", "index.js").
	FileMatch string `json:"file_match"`
	// Content is a regex to match against file content. Empty means presence-only detection.
	Content string `json:"content,omitempty"`
	// PreContent is a regex that must match at least one line in the file before the Content
	// match is considered valid. Used for language-specific preconditions (e.g., Go's
	// "^package main$" before "^func main()"). Empty means no precondition.
	PreContent string `json:"pre_content,omitempty"`
	// Type classifies the entry point: "main", "server_start", "cli_root", etc.
	Type string `json:"type"`
	// Label is a human-readable description.
	Label string `json:"label"`
}

// PackRegistry holds loaded pack metadata for all known languages.
type PackRegistry struct {
	mu    sync.RWMutex
	packs map[string]*Pack

	// Derived lookup tables, built from pack metadata.
	extLookup      map[string]string // extension -> language name
	filenameLookup map[string]string // filename -> language name
	shebangLookup  map[string]string // interpreter -> language name
	aliasLookup    map[string]string // alias -> language name

	// idx holds the project marker index, loaded from packs/index.json.
	idx indexState
}

// NewPackRegistry creates a PackRegistry pre-loaded with all embedded packs.
func NewPackRegistry() (*PackRegistry, error) {
	r := &PackRegistry{
		packs:          make(map[string]*Pack),
		extLookup:      make(map[string]string),
		filenameLookup: make(map[string]string),
		shebangLookup:  make(map[string]string),
		aliasLookup:    make(map[string]string),
	}

	// Load all embedded packs.
	err := fs.WalkDir(embeddedPacks, "packs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "pack.json" {
			return nil
		}
		data, readErr := embeddedPacks.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading embedded pack %s: %w", path, readErr)
		}
		var pack Pack
		if jsonErr := json.Unmarshal(data, &pack); jsonErr != nil {
			return fmt.Errorf("parsing embedded pack %s: %w", path, jsonErr)
		}
		r.register(&pack)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading embedded packs: %w", err)
	}

	// Load the embedded project marker index, then merge any additional
	// partials in packs/index.d/ — split by topic for maintainability.
	indexData, err := embeddedPacks.ReadFile("packs/index.json")
	if err != nil {
		return nil, fmt.Errorf("reading embedded index: %w", err)
	}
	if err := r.LoadIndex(indexData); err != nil {
		return nil, fmt.Errorf("loading embedded index: %w", err)
	}

	if err := r.loadEmbeddedIndexDir("packs/index.d"); err != nil {
		return nil, fmt.Errorf("loading embedded index.d: %w", err)
	}

	return r, nil
}

// loadEmbeddedIndexDir merges every *.json file under the given embedded
// directory into the registry's project marker index. Files are loaded in
// lexical order; later files override earlier ones via the standard
// (File, Kind) merge semantics.
func (r *PackRegistry) loadEmbeddedIndexDir(dir string) error {
	entries, err := fs.ReadDir(embeddedPacks, dir)
	if err != nil {
		// No index.d directory is fine.
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		data, readErr := embeddedPacks.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			return fmt.Errorf("reading embedded index partial %s: %w", name, readErr)
		}
		if err := r.LoadIndex(data); err != nil {
			return fmt.Errorf("loading embedded index partial %s: %w", name, err)
		}
	}
	return nil
}

// LoadIndexDir merges every *.json file under the given on-disk directory
// into the registry's project marker index. Used for user/team overrides at
// .aide/grammars/index.d/. Files are loaded in lexical order; later files
// (and individual entries) override earlier ones via (File, Kind) keys.
func (r *PackRegistry) LoadIndexDir(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("globbing %s: %w", dir, err)
	}
	sort.Strings(matches)
	for _, p := range matches {
		if err := r.LoadIndexFromFile(p); err != nil {
			return fmt.Errorf("loading index partial %s: %w", p, err)
		}
	}
	return nil
}

// LoadFromDir loads a pack.json from a directory (e.g., .aide/grammars/{name}/).
// If the pack already exists in the registry (e.g., from an embedded pack), the
// on-disk version takes precedence (user/download override).
func (r *PackRegistry) LoadFromDir(dir string) error {
	packPath := filepath.Join(dir, "pack.json")
	data, err := os.ReadFile(packPath)
	if err != nil {
		return err
	}
	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return fmt.Errorf("parsing pack %s: %w", packPath, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(&pack)
	return nil
}

// Get returns a shallow copy of the pack for the given language name, or nil if not found.
// The returned Pack is safe to read but must not be used to modify registry internals.
func (r *PackRegistry) Get(name string) *Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p := r.packs[name]
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// GetByAlias returns a shallow copy of the pack for the given alias, or nil if not found.
func (r *PackRegistry) GetByAlias(alias string) *Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if canonical, ok := r.aliasLookup[alias]; ok {
		if p := r.packs[canonical]; p != nil {
			cp := *p
			return &cp
		}
	}
	return nil
}

// LangForExtension returns the language name for a file extension (e.g., ".go" -> "go").
func (r *PackRegistry) LangForExtension(ext string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lang, ok := r.extLookup[ext]
	return lang, ok
}

// LangForFilename returns the language name for a known filename (e.g., "Makefile" -> "bash").
func (r *PackRegistry) LangForFilename(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lang, ok := r.filenameLookup[name]
	return lang, ok
}

// LangForShebang returns the language name for a shebang interpreter (e.g., "python3" -> "python").
func (r *PackRegistry) LangForShebang(interpreter string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lang, ok := r.shebangLookup[interpreter]
	return lang, ok
}

// NormaliseLang converts a language alias to its canonical name.
// Returns the input unchanged if no alias is found.
func (r *PackRegistry) NormaliseLang(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if canonical, ok := r.aliasLookup[s]; ok {
		return canonical
	}
	// Also check if it's already a canonical name.
	if _, ok := r.packs[s]; ok {
		return s
	}
	return s
}

// All returns all registered pack names.
func (r *PackRegistry) All() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.packs))
	for name := range r.packs {
		names = append(names, name)
	}
	return names
}

// Languages returns all language names with shallow-copied grammar packs.
func (r *PackRegistry) Languages() map[string]*Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*Pack, len(r.packs))
	for k, v := range r.packs {
		cp := *v
		result[k] = &cp
	}
	return result
}

// DynamicPacks returns all packs that represent downloadable grammars —
// those with a non-empty SourceRepo and CSymbol. Built-in packs may also
// have these fields set, but that is fine — the CompositeLoader checks
// the BuiltinRegistry first.
func (r *PackRegistry) DynamicPacks() map[string]*Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*Pack)
	for name, p := range r.packs {
		if p.SourceRepo != "" && p.CSymbol != "" {
			cp := *p
			result[name] = &cp
		}
	}
	return result
}

// register adds a pack to the registry. NOT thread-safe — caller must hold no lock
// (acquires write lock internally).
func (r *PackRegistry) register(pack *Pack) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(pack)
}

// registerLocked adds a pack to the registry. Caller must hold write lock.
func (r *PackRegistry) registerLocked(pack *Pack) {
	r.packs[pack.Name] = pack

	for _, ext := range pack.Meta.Extensions {
		r.extLookup[ext] = pack.Name
	}
	for _, fn := range pack.Meta.Filenames {
		r.filenameLookup[fn] = pack.Name
	}
	for _, sh := range pack.Meta.Shebangs {
		r.shebangLookup[sh] = pack.Name
	}
	for _, alias := range pack.Meta.Aliases {
		r.aliasLookup[alias] = pack.Name
	}
}
