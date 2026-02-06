package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// cmdCodeDispatcher routes code subcommands.
func cmdCodeDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printCodeUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "index":
		return cmdCodeIndex(dbPath, subargs)
	case "search":
		return cmdCodeSearch(dbPath, subargs)
	case "symbols":
		return cmdCodeSymbols(dbPath, subargs)
	case "references", "refs":
		return cmdCodeReferences(dbPath, subargs)
	case "clear":
		return cmdCodeClear(dbPath)
	case "stats":
		return cmdCodeStats(dbPath)
	case "help", "-h", "--help":
		printCodeUsage()
		return nil
	default:
		return fmt.Errorf("unknown code subcommand: %s", subcmd)
	}
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
  clear      Clear the code index
  stats      Show indexing statistics

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

Examples:
  aide code index                     # Index current directory
  aide code index src/ lib/           # Index specific directories
  aide code search "getUser"          # Search for symbols
  aide code search "User" --kind=interface
  aide code symbols src/auth.ts       # List symbols in file
  aide code refs getUserById          # Find all calls to getUserById
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
		fmt.Sscanf(l, "%d", &limit)
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
		fmt.Print("[")
		for i, r := range results {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"name":"%s","kind":"%s","signature":"%s","file":"%s","line":%d,"lang":"%s"}`,
				escapeJSON(r.Symbol.Name),
				r.Symbol.Kind,
				escapeJSON(r.Symbol.Signature),
				escapeJSON(r.Symbol.FilePath),
				r.Symbol.StartLine,
				r.Symbol.Language)
		}
		fmt.Println("]")
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
		fmt.Print("[")
		for i, sym := range symbols {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"name":"%s","kind":"%s","signature":"%s","line":%d}`,
				escapeJSON(sym.Name),
				sym.Kind,
				escapeJSON(sym.Signature),
				sym.StartLine)
		}
		fmt.Println("]")
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
		fmt.Sscanf(l, "%d", &limit)
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
		fmt.Print("[")
		for i, ref := range refs {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"symbol":"%s","kind":"%s","file":"%s","line":%d,"col":%d,"context":"%s"}`,
				escapeJSON(ref.SymbolName),
				ref.Kind,
				escapeJSON(ref.FilePath),
				ref.Line,
				ref.Column,
				escapeJSON(ref.Context))
		}
		fmt.Println("]")
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

// padString pads a string to a minimum width.
func padString(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// Indexer provides a reusable indexing interface for the watcher.
type Indexer struct {
	store  store.CodeIndexStore
	parser *code.Parser
}

// NewIndexer creates a new indexer.
func NewIndexer(dbPath string) (*Indexer, error) {
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		return nil, err
	}

	return &Indexer{
		store:  codeStore,
		parser: code.NewParser(),
	}, nil
}

// Close closes the indexer.
func (idx *Indexer) Close() error {
	return idx.store.Close()
}

// IndexFile indexes a single file (symbols and references).
func (idx *Indexer) IndexFile(filePath string) (int, error) {
	// Get relative path
	relPath := filePath
	if abs, err := filepath.Abs(filePath); err == nil {
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, abs); err == nil {
				relPath = rel
			}
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

	// Update file info
	info, _ := os.Stat(filePath)
	modTime := time.Now()
	if info != nil {
		modTime = info.ModTime()
	}

	idx.store.SetFileInfo(&code.FileInfo{
		Path:      relPath,
		ModTime:   modTime,
		SymbolIDs: symbolIDs,
	})

	return len(symbols), nil
}

// RemoveFile removes a file from the index.
func (idx *Indexer) RemoveFile(filePath string) error {
	relPath := filePath
	if abs, err := filepath.Abs(filePath); err == nil {
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, abs); err == nil {
				relPath = rel
			}
		}
	}

	return idx.store.ClearFile(relPath)
}
