package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// cmdCodeDispatcher routes code subcommands.
func cmdCodeDispatcher(dbPath string, args []string) error {
	return dispatchSubcmd("code", args, printCodeUsage, []subcmd{
		{name: "index", handler: func(a []string) error { return cmdCodeIndex(dbPath, a) }},
		{name: "search", handler: func(a []string) error { return cmdCodeSearch(dbPath, a) }},
		{name: "symbols", handler: func(a []string) error { return cmdCodeSymbols(dbPath, a) }},
		{name: "references", aliases: []string{"refs"}, handler: func(a []string) error { return cmdCodeReferences(dbPath, a) }},
		{name: "read-check", handler: func(a []string) error { return cmdCodeReadCheck(dbPath, a) }},
		{name: "clear", handler: func(a []string) error { return cmdCodeClear(dbPath) }},
		{name: "stats", handler: func(a []string) error { return cmdCodeStats(dbPath) }},
		{name: "reconcile", handler: func(a []string) error { return cmdCodeReconcile(dbPath) }},
	})
}

func cmdCodeReconcile(dbPath string) error {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	if backend.UsingGRPC() {
		return fmt.Errorf("a daemon is currently holding the code index; stop the MCP server first, then run `aide code reconcile` (or run `aide code clear && aide code index` for a full rebuild)")
	}

	cs, err := backend.openCodeStore()
	if err != nil {
		return fmt.Errorf("failed to open code store: %w", err)
	}
	defer cs.Close()

	idx := NewIndexerFromStore(cs, newGrammarLoader(dbPath, nil), projectRoot(dbPath))
	res, err := idx.Reconcile()
	if err != nil {
		return fmt.Errorf("reconcile failed: %w", err)
	}
	fmt.Printf("Reconciled: checked %d, removed %d, refreshed %d, errors %d\n",
		res.Checked, res.Removed, res.Refreshed, res.Errors)
	return nil
}

func printCodeUsage() {
	fmt.Println(`aide code - Index and search code symbols and references

Usage:
  aide code <subcommand> [arguments]

Subcommands:
  index      Index code in directories (incremental by default)
  search     Search for symbols by name/signature
  symbols    List symbols in a file
  references Find all call sites/usages of a symbol
  read-check Check if a file is indexed and unchanged
  clear      Clear the code index
  stats      Show indexing statistics
  reconcile  Drop stale index entries (deleted files, newly-ignored paths) and refresh modified ones

Options:
  index [paths...]:
    --force      Re-index even if file hasn't changed

  search <query>:
    --kind=TYPE    Filter by kind (function, method, class, interface, type)
    --lang=LANG    Filter by language (typescript, go, python)
    --file=PATH    Filter by file path pattern
    --limit=N      Max results (default 20)
    --json         Output as JSON

  symbols <file>:
    --json         Output as JSON

  references <symbol>:
    --kind=TYPE    Filter by kind (call, type_ref)
    --file=PATH    Filter by file path pattern
    --limit=N      Max results (default 50)
    --json         Output as JSON

  read-check <file>:
    --json         Output as JSON

Examples:
  aide code index                     # Index current directory
  aide code index src/ lib/           # Index specific directories
  aide code search "getUser"          # Search for symbols
  aide code search "User" --kind=interface
  aide code symbols src/auth.ts       # List symbols in file
  aide code refs getUserById          # Find all calls to getUserById
  aide code read-check src/auth.ts    # Check if file is indexed and fresh
  aide code clear                     # Clear all indexed data`)
}

// getCodeStorePaths returns the paths for code index database and search index.
func getCodeStorePaths(dbPath string) (string, string) {
	baseDir := filepath.Dir(dbPath)
	codeDir := filepath.Join(baseDir, "code")
	return filepath.Join(codeDir, "index.db"), filepath.Join(codeDir, "search.bleve")
}

func cmdCodeIndex(dbPath string, args []string) error {
	force := hasFlag(args, "--force")

	// Get paths to index (default to current directory)
	var paths []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			paths = append(paths, arg)
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// Progress callback - only works in direct mode (not gRPC)
	var progress func(path string, symbols int)
	showSpinner := !backend.UsingGRPC()
	if showSpinner {
		spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinIdx := 0
		progress = func(path string, symbols int) {
			displayPath := path
			if len(displayPath) > 60 {
				displayPath = "..." + displayPath[len(displayPath)-57:]
			}
			fmt.Fprintf(os.Stderr, "\r%s %s (%d symbols)    ", spinChars[spinIdx], displayPath, symbols)
			spinIdx = (spinIdx + 1) % len(spinChars)
		}
	}

	result, err := backend.IndexCodeWithProgress(paths, force, progress)
	if err != nil {
		return fmt.Errorf("failed to index code: %w", err)
	}

	// Clear the spinner line
	if showSpinner {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}

	// Print summary
	if result.FilesSkipped > 0 {
		fmt.Printf("Indexed %d symbols from %d files (%d unchanged)\n", result.SymbolsIndexed, result.FilesIndexed, result.FilesSkipped)
	} else {
		fmt.Printf("Indexed %d symbols from %d files\n", result.SymbolsIndexed, result.FilesIndexed)
	}

	return nil
}

func cmdCodeSearch(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide code search <query> [--kind=TYPE] [--lang=LANG] [--limit=N]")
	}

	// Parse query (first non-flag argument)
	var query string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			query = arg
			break
		}
	}
	if query == "" {
		return fmt.Errorf("query is required")
	}

	// Parse options
	kind := parseFlag(args, "--kind=")
	language := parseFlag(args, "--lang=")
	filePath := parseFlag(args, "--file=")
	limit := 20
	if l := parseFlag(args, "--limit="); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return fmt.Errorf("invalid --limit= value %q: %w", l, err)
		}
		limit = n
	}
	jsonOutput := hasFlag(args, "--json")

	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// Auto-wrap simple queries with wildcards for substring matching
	// Skip if query already contains Bleve syntax characters
	if query != "" && !containsBleveSyntax(query) {
		query = "*" + query + "*"
	}

	// Search
	results, err := backend.SearchCode(query, kind, language, filePath, limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No matching symbols found")
		return nil
	}

	// Output results
	if jsonOutput {
		symbols := make([]*code.Symbol, len(results))
		for i, r := range results {
			symbols[i] = r.Symbol
		}
		return printJSON(symbols)
	} else {
		for _, r := range results {
			sym := r.Symbol
			// Format: [kind] signature    file:line
			kindPad := padString(fmt.Sprintf("[%s]", sym.Kind), 12)
			sigPad := padString(sym.Signature, 50)
			fmt.Printf("%s %s %s:%d\n", kindPad, sigPad, sym.FilePath, sym.StartLine)
		}
	}

	return nil
}

func cmdCodeSymbols(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide code symbols <file> [--json]")
	}

	// Get file path
	var filePath string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			filePath = arg
			break
		}
	}
	if filePath == "" {
		return fmt.Errorf("file path is required")
	}

	jsonOutput := hasFlag(args, "--json")

	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// Get symbols for file
	symbols, err := backend.GetFileSymbols(filePath)
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	if len(symbols) == 0 {
		fmt.Printf("No symbols found in %s\n", filePath)
		return nil
	}

	// Output
	if jsonOutput {
		return printJSON(symbols)
	} else {
		fmt.Printf("%s (%d symbols):\n", filePath, len(symbols))
		for _, sym := range symbols {
			kindPad := padString(fmt.Sprintf("[%s]", sym.Kind), 12)
			namePad := padString(sym.Name, 30)
			fmt.Printf("  %s %s line %d\n", kindPad, namePad, sym.StartLine)
		}
	}

	return nil
}

func cmdCodeReferences(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide code references <symbol> [--kind=TYPE] [--file=PATH] [--limit=N]")
	}

	// Parse symbol name (first non-flag argument)
	var symbolName string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			symbolName = arg
			break
		}
	}
	if symbolName == "" {
		return fmt.Errorf("symbol name is required")
	}

	// Parse options
	kind := parseFlag(args, "--kind=")
	filePath := parseFlag(args, "--file=")
	limit := 50
	if l := parseFlag(args, "--limit="); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return fmt.Errorf("invalid --limit= value %q: %w", l, err)
		}
		limit = n
	}
	jsonOutput := hasFlag(args, "--json")

	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	// Search references
	refs, err := backend.SearchReferences(symbolName, kind, filePath, limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(refs) == 0 {
		fmt.Printf("No references found for '%s'\n", symbolName)
		return nil
	}

	// Output results
	if jsonOutput {
		return printJSON(refs)
	} else {
		fmt.Printf("References to '%s' (%d found):\n", symbolName, len(refs))
		for _, ref := range refs {
			kindPad := padString(fmt.Sprintf("[%s]", ref.Kind), 12)
			location := fmt.Sprintf("%s:%d:%d", ref.FilePath, ref.Line, ref.Column)
			locationPad := padString(location, 40)
			context := ref.Context
			if len(context) > 60 {
				context = context[:60] + "..."
			}
			fmt.Printf("  %s %s %s\n", kindPad, locationPad, context)
		}
	}

	return nil
}

func cmdCodeClear(dbPath string) error {
	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	symbols, files, err := backend.ClearCode()
	if err != nil {
		return fmt.Errorf("failed to clear code index: %w", err)
	}

	if symbols > 0 || files > 0 {
		fmt.Printf("Cleared %d symbols from %d files\n", symbols, files)
	} else {
		fmt.Println("Code index cleared")
	}

	return nil
}

func cmdCodeStats(dbPath string) error {
	// Create backend (uses gRPC if daemon is running)
	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	stats, err := backend.GetCodeStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Printf("Code Index Statistics:\n")
	fmt.Printf("  Files indexed:      %d\n", stats.Files)
	fmt.Printf("  Symbols indexed:    %d\n", stats.Symbols)
	fmt.Printf("  References indexed: %d\n", stats.References)

	return nil
}

func cmdCodeReadCheck(dbPath string, args []string) error {
	jsonOutput := hasFlag(args, "--json")

	// Extract first non-flag argument as file path
	var filePath string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			filePath = arg
			break
		}
	}
	if filePath == "" {
		return fmt.Errorf("usage: aide code read-check <file> [--json]")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		// Graceful fallback: code store might not be available
		result := &ReadCheckResult{}
		if jsonOutput {
			return printJSON(result)
		}
		fmt.Println("not indexed")
		return nil
	}
	defer backend.Close()

	result, err := backend.ReadCheck(filePath)
	if err != nil {
		result = &ReadCheckResult{}
	}

	if jsonOutput {
		return printJSON(result)
	}

	switch {
	case !result.Indexed:
		fmt.Println("not indexed")
	case result.Fresh:
		fmt.Printf("indexed (fresh): %d symbols\n", result.Symbols)
	default:
		fmt.Printf("indexed (stale): %d symbols\n", result.Symbols)
	}

	return nil
}

// Indexer provides a reusable indexing interface for the watcher.
type Indexer struct {
	store   store.CodeIndexStore
	parser  *code.Parser
	rootDir string // absolute project root for relative path computation
}

// NewIndexer creates a new indexer by opening a new code store.
// Prefer NewIndexerFromStore when a code store is already open.
func NewIndexer(dbPath string) (*Indexer, error) {
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		return nil, err
	}

	return &Indexer{
		store:   codeStore,
		parser:  code.NewParser(newGrammarLoader(dbPath, nil)),
		rootDir: projectRoot(dbPath),
	}, nil
}

// NewIndexerFromStore creates an indexer reusing an existing code store.
// The caller retains ownership of the store — Close() is a no-op.
func NewIndexerFromStore(cs store.CodeIndexStore, loader grammar.Loader, rootDir string) *Indexer {
	return &Indexer{
		store:   cs,
		parser:  code.NewParser(loader),
		rootDir: rootDir,
	}
}

// Close closes the indexer.
func (idx *Indexer) Close() error {
	return idx.store.Close()
}

// IndexFile indexes a single file (symbols and references).
func (idx *Indexer) IndexFile(filePath string) (int, error) {
	span := observe.Start("Indexer.IndexFile", observe.KindSpan).Category("indexer").Subtype("index_file").FilePath(filePath)
	defer span.End()
	// Get relative path from project root
	relPath := filePath
	if abs, err := filepath.Abs(filePath); err == nil {
		if rel, err := filepath.Rel(idx.rootDir, abs); err == nil {
			relPath = rel
		}
	}

	// Parse file for symbols
	symbols, err := idx.parser.ParseFile(filePath)
	if err != nil {
		return 0, err
	}

	// Parse file for references
	refs, _ := idx.parser.ParseFileReferences(filePath)

	// Clear existing symbols and references
	idx.store.ClearFile(relPath)
	idx.store.ClearFileReferences(relPath)

	// Store symbols
	symbolIDs := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		sym.FilePath = relPath
		if err := idx.store.AddSymbol(sym); err != nil {
			continue
		}
		symbolIDs = append(symbolIDs, sym.ID)
	}

	// Store references
	for _, ref := range refs {
		ref.FilePath = relPath
		idx.store.AddReference(ref)
	}

	// Update file info with token estimate
	info, _ := os.Stat(filePath)
	modTime := time.Now()
	var sizeBytes int64
	if info != nil {
		modTime = info.ModTime()
		sizeBytes = info.Size()
	}

	idx.store.SetFileInfo(&code.FileInfo{
		Path:      relPath,
		ModTime:   modTime,
		SymbolIDs: symbolIDs,
		Tokens:    code.EstimateTokensFromSize(relPath, sizeBytes),
		SizeBytes: sizeBytes,
	})

	return len(symbols), nil
}

// ReconcileResult summarises an Indexer.Reconcile pass.
type ReconcileResult struct {
	Checked   int // Total entries inspected
	Removed   int // Entries dropped because the file no longer exists on disk
	Refreshed int // Entries re-indexed because mtime advanced
	Errors    int // Stat / index failures (skipped, not fatal)
}

// Reconcile walks the file index and brings it back in sync with the working
// tree: orphan entries (file deleted on disk) are removed, entries whose path
// now matches an aideignore rule are removed, and stale entries (file mtime
// newer than the indexed mtime) are re-indexed. Then a second sweep walks
// every symbol and reference bucket entry to catch orphan rows whose fileinfo
// was already cleared but whose symbol/reference rows survived.
func (idx *Indexer) Reconcile() (ReconcileResult, error) {
	span := observe.Start("Indexer.Reconcile", observe.KindSpan).Category("indexer").Subtype("reconcile")
	defer span.End()
	var res ReconcileResult
	defer func() {
		span.Attr("checked", strconv.Itoa(res.Checked)).
			Attr("removed", strconv.Itoa(res.Removed)).
			Attr("refreshed", strconv.Itoa(res.Refreshed))
	}()

	infos, err := idx.store.ListAllFileInfo()
	if err != nil {
		return res, fmt.Errorf("list file index: %w", err)
	}

	ignore, _ := aideignore.New(idx.rootDir)
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	for _, info := range infos {
		res.Checked++

		absPath := info.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(idx.rootDir, info.Path)
		}

		stat, statErr := os.Stat(absPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				if rmErr := idx.RemoveFile(absPath); rmErr == nil {
					res.Removed++
				} else {
					res.Errors++
				}
				continue
			}
			res.Errors++
			continue
		}

		if ignore.ShouldIgnoreFile(info.Path) {
			if rmErr := idx.RemoveFile(absPath); rmErr == nil {
				res.Removed++
			} else {
				res.Errors++
			}
			continue
		}

		if !stat.ModTime().Equal(info.ModTime) {
			if _, ixErr := idx.IndexFile(absPath); ixErr == nil {
				res.Refreshed++
			} else {
				res.Errors++
			}
		}
	}

	idx.sweepOrphans(infos, ignore, &res)
	return res, nil
}

// sweepOrphans handles the post-fileinfo cleanup pass: corrupt rows (empty
// FilePath), in-file orphans (rows whose FilePath matches a tracked file
// but whose ID is missing from that file's SymbolIDs), and orphan-path rows
// (FilePath has no FileInfo entry). The pass is split out from Reconcile
// to keep complexity reasonable.
func (idx *Indexer) sweepOrphans(infos []*code.FileInfo, ignore *aideignore.Matcher, res *ReconcileResult) {
	expectedByPath := make(map[string]map[string]struct{}, len(infos))
	for _, info := range infos {
		ids := make(map[string]struct{}, len(info.SymbolIDs))
		for _, id := range info.SymbolIDs {
			ids[id] = struct{}{}
		}
		expectedByPath[info.Path] = ids
	}

	corruptSymIDs, inFileOrphanIDs, orphanPaths := idx.classifyOrphans(expectedByPath)

	for id := range corruptSymIDs {
		res.Checked++
		if err := idx.store.DeleteSymbol(id); err == nil {
			res.Removed++
		} else {
			res.Errors++
		}
	}
	for id := range inFileOrphanIDs {
		res.Checked++
		if err := idx.store.DeleteSymbol(id); err == nil {
			res.Removed++
		} else {
			res.Errors++
		}
	}
	for p := range orphanPaths {
		res.Checked++
		if !idx.shouldDropPath(p, ignore) {
			continue
		}
		if err := idx.store.ClearFile(p); err == nil {
			_ = idx.store.ClearFileReferences(p)
			res.Removed++
		} else {
			res.Errors++
		}
	}
}

// classifyOrphans partitions symbol/reference rows into three buckets:
// corrupt (empty FilePath), in-file orphan (path tracked but ID not listed
// in FileInfo), and orphan-path (path not tracked at all).
func (idx *Indexer) classifyOrphans(expectedByPath map[string]map[string]struct{}) (corrupt, inFile, paths map[string]struct{}) {
	corrupt = map[string]struct{}{}
	inFile = map[string]struct{}{}
	paths = map[string]struct{}{}

	if syms, err := idx.store.ListAllSymbols(-1); err == nil {
		for _, s := range syms {
			if s.FilePath == "" {
				corrupt[s.ID] = struct{}{}
				continue
			}
			if expected, seen := expectedByPath[s.FilePath]; seen {
				if _, listed := expected[s.ID]; !listed {
					inFile[s.ID] = struct{}{}
				}
				continue
			}
			paths[s.FilePath] = struct{}{}
		}
	}
	if refs, err := idx.store.ListAllReferences(-1); err == nil {
		for _, r := range refs {
			if r.FilePath == "" {
				continue
			}
			if _, seen := expectedByPath[r.FilePath]; seen {
				continue
			}
			paths[r.FilePath] = struct{}{}
		}
	}
	return corrupt, inFile, paths
}

// shouldDropPath reports whether an orphan path should be cleared from the
// index: matches an aideignore rule, or doesn't exist on disk anymore.
func (idx *Indexer) shouldDropPath(p string, ignore *aideignore.Matcher) bool {
	if ignore.ShouldIgnoreFile(p) {
		return true
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(idx.rootDir, p)
	}
	_, err := os.Stat(abs)
	return os.IsNotExist(err)
}

// RemoveFile removes a file from the index, including its symbols, references,
// and file tracking info.
func (idx *Indexer) RemoveFile(filePath string) error {
	relPath := filePath
	if abs, err := filepath.Abs(filePath); err == nil {
		if rel, err := filepath.Rel(idx.rootDir, abs); err == nil {
			relPath = rel
		}
	}

	// Clear references from this file (call sites, type refs, etc.).
	if err := idx.store.ClearFileReferences(relPath); err != nil {
		return fmt.Errorf("clear references: %w", err)
	}

	// Clear symbols and file tracking info.
	return idx.store.ClearFile(relPath)
}
