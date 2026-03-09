package survey

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EntrypointsResult holds the output of the entrypoints analyzer.
type EntrypointsResult struct {
	Entries []*Entry
}

// RunEntrypoints uses the code index to find entry points in the codebase.
// If codeSearcher is nil, it falls back to file-scanning heuristics.
func RunEntrypoints(rootDir string, codeSearcher CodeSearcher) (*EntrypointsResult, error) {
	result := &EntrypointsResult{}

	if codeSearcher != nil {
		// Code-index-based detection (preferred — more accurate)
		result.detectGoMain(codeSearcher)
		result.detectGoInit(codeSearcher)
		result.detectGoHTTPHandlers(codeSearcher)
		result.detectGoGRPC(codeSearcher)
		result.detectGoCLI(codeSearcher)
		result.detectPythonMain(codeSearcher)
		result.detectRustMain(codeSearcher)
	} else {
		// Fallback: scan filesystem for common entrypoint patterns
		result.detectEntrypointsByFileScan(rootDir)
	}

	return result, nil
}

// isGeneratedFile returns true if the path looks like a generated file.
func isGeneratedFile(path string) bool {
	return strings.HasSuffix(path, ".pb.go") ||
		strings.HasSuffix(path, ".pb.gw.go") ||
		strings.HasSuffix(path, "_generated.go") ||
		strings.HasSuffix(path, "_gen.go") ||
		strings.Contains(path, "generated") ||
		strings.Contains(path, "vendor/")
}

// isTestFile returns true if the path looks like a test file.
func isTestFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, "_test.rs") ||
		strings.HasSuffix(path, "_test.py") {
		return true
	}
	// Check for test directories (handle both relative and absolute paths)
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "testdata" || p == "test" || p == "tests" {
			return true
		}
	}
	return false
}

// isGoFile returns true if the path is a Go source file.
func isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}

// detectGoMain finds Go main() functions.
// Excludes generated files, test files, and non-Go files.
func (r *EntrypointsResult) detectGoMain(cs CodeSearcher) {
	symbols, err := cs.FindSymbols("main", "function", 100)
	if err != nil {
		return
	}

	for _, sym := range symbols {
		if sym.Name != "main" || sym.Kind != "function" {
			continue
		}
		if !isGoFile(sym.FilePath) {
			continue // Rust main() is handled by detectRustMain
		}
		if isGeneratedFile(sym.FilePath) || isTestFile(sym.FilePath) {
			continue
		}
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:main()", sym.FilePath),
			FilePath: sym.FilePath,
			Title:    "Go main() entry point",
			Detail:   fmt.Sprintf("func main() at %s:%d", sym.FilePath, sym.Line),
			Metadata: map[string]string{
				"language": "go",
				"type":     "main",
				"line":     fmt.Sprintf("%d", sym.Line),
			},
		})
	}
}

// detectGoInit finds Go init() functions.
// Excludes generated files (e.g. .pb.go protobuf boilerplate).
func (r *EntrypointsResult) detectGoInit(cs CodeSearcher) {
	symbols, err := cs.FindSymbols("init", "function", 100)
	if err != nil {
		return
	}

	for _, sym := range symbols {
		if sym.Name != "init" || sym.Kind != "function" {
			continue
		}
		if !isGoFile(sym.FilePath) {
			continue
		}
		if isGeneratedFile(sym.FilePath) || isTestFile(sym.FilePath) {
			continue
		}
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:init()", sym.FilePath),
			FilePath: sym.FilePath,
			Title:    "Go init() function",
			Detail:   fmt.Sprintf("func init() at %s:%d", sym.FilePath, sym.Line),
			Metadata: map[string]string{
				"language": "go",
				"type":     "init",
				"line":     fmt.Sprintf("%d", sym.Line),
			},
		})
	}
}

// detectGoHTTPHandlers finds Go HTTP handler registrations.
// Uses exact symbol matching (not substring) to avoid false positives
// like syscall.Handle or other unrelated symbols containing "Handle".
func (r *EntrypointsResult) detectGoHTTPHandlers(cs CodeSearcher) {
	// Search for specific HTTP-related function calls
	httpSymbols := []string{"HandleFunc", "Handle", "ListenAndServe"}
	seen := make(map[string]bool) // deduplicate by file:line

	for _, pattern := range httpSymbols {
		refs, err := cs.FindReferences(pattern, "call", 50)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			if isGeneratedFile(ref.FilePath) || isTestFile(ref.FilePath) {
				continue
			}
			// Exact match on known HTTP symbols — not substring matching.
			// This prevents false positives like syscall.Handle, dll.Handle, etc.
			if !isHTTPSymbol(ref.Symbol) {
				continue
			}
			key := fmt.Sprintf("%s:%d", ref.FilePath, ref.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:%s", ref.FilePath, ref.Symbol),
				FilePath: ref.FilePath,
				Title:    fmt.Sprintf("HTTP handler: %s", ref.Symbol),
				Detail:   fmt.Sprintf("%s at %s:%d", ref.Symbol, ref.FilePath, ref.Line),
				Metadata: map[string]string{
					"language": "go",
					"type":     "http_handler",
					"line":     fmt.Sprintf("%d", ref.Line),
				},
			})
		}
	}
}

// isHTTPSymbol returns true if the symbol is a known HTTP handler/server function.
// Matches against known net/http and popular framework symbols.
func isHTTPSymbol(sym string) bool {
	// Match the base name: "http.HandleFunc" → "HandleFunc"
	baseName := sym
	qualifier := ""
	if idx := strings.LastIndex(sym, "."); idx >= 0 {
		qualifier = sym[:idx]
		baseName = sym[idx+1:]
	}

	switch baseName {
	case "HandleFunc", "ListenAndServe", "ListenAndServeTLS", "ServeMux":
		// These are unambiguously HTTP-related regardless of qualifier
		return true
	case "Handle":
		// "Handle" alone is ambiguous — could be syscall.Handle, dll.Handle, etc.
		// Only accept if qualifier is empty (standalone) or HTTP-related.
		if qualifier == "" {
			return true
		}
		// Accept known HTTP qualifiers
		lq := strings.ToLower(qualifier)
		return strings.Contains(lq, "http") || strings.Contains(lq, "mux") ||
			strings.Contains(lq, "router") || strings.Contains(lq, "server")
	}
	return false
}

// detectGoGRPC finds gRPC service registrations.
// Excludes generated files — we want the call sites (where services are wired up),
// not the generated RegisterXxxServer function definitions.
func (r *EntrypointsResult) detectGoGRPC(cs CodeSearcher) {
	refs, err := cs.FindReferences("Register", "call", 100)
	if err != nil {
		return
	}

	seen := make(map[string]bool) // deduplicate by symbol name
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Symbol, "Register") || !strings.HasSuffix(ref.Symbol, "Server") {
			continue
		}
		if isGeneratedFile(ref.FilePath) || isTestFile(ref.FilePath) {
			continue
		}
		// Deduplicate by symbol name — a gRPC service registered once is one entrypoint
		if seen[ref.Symbol] {
			continue
		}
		seen[ref.Symbol] = true
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:%s", ref.FilePath, ref.Symbol),
			FilePath: ref.FilePath,
			Title:    fmt.Sprintf("gRPC service registration: %s", ref.Symbol),
			Detail:   fmt.Sprintf("%s at %s:%d", ref.Symbol, ref.FilePath, ref.Line),
			Metadata: map[string]string{
				"language": "go",
				"type":     "grpc_service",
				"line":     fmt.Sprintf("%d", ref.Line),
			},
		})
	}
}

// detectGoCLI finds CLI root command definitions (cobra, urfave/cli, flag-based).
func (r *EntrypointsResult) detectGoCLI(cs CodeSearcher) {
	// Detect cobra root commands: cobra.Command references are CLI entry points
	r.detectCobraCommands(cs)

	// Detect urfave/cli apps
	r.detectUrfaveCLI(cs)
}

// detectCobraCommands finds cobra.Command root definitions.
func (r *EntrypointsResult) detectCobraCommands(cs CodeSearcher) {
	// Search for known cobra function calls individually
	cobraPatterns := []string{"Execute", "ExecuteC", "AddCommand"}

	seen := make(map[string]bool)
	for _, pattern := range cobraPatterns {
		refs, err := cs.FindReferences(pattern, "call", 100)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			if isGeneratedFile(ref.FilePath) || isTestFile(ref.FilePath) {
				continue
			}
			if !isCobraSymbol(ref.Symbol) {
				continue
			}
			key := fmt.Sprintf("%s:%d", ref.FilePath, ref.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:%s", ref.FilePath, ref.Symbol),
				FilePath: ref.FilePath,
				Title:    fmt.Sprintf("CLI command: %s", ref.Symbol),
				Detail:   fmt.Sprintf("%s at %s:%d", ref.Symbol, ref.FilePath, ref.Line),
				Metadata: map[string]string{
					"language": "go",
					"type":     "cli_root",
					"line":     fmt.Sprintf("%d", ref.Line),
				},
			})
		}
	}
}

// isCobraSymbol returns true if the symbol is a known cobra CLI function.
func isCobraSymbol(sym string) bool {
	baseName := sym
	if idx := strings.LastIndex(sym, "."); idx >= 0 {
		baseName = sym[idx+1:]
	}
	switch baseName {
	case "Execute", "ExecuteC", "AddCommand":
		return true
	}
	return false
}

// detectUrfaveCLI finds urfave/cli app definitions.
func (r *EntrypointsResult) detectUrfaveCLI(cs CodeSearcher) {
	refs, err := cs.FindReferences("Run", "call", 100)
	if err != nil {
		return
	}

	seen := make(map[string]bool)
	for _, ref := range refs {
		if isGeneratedFile(ref.FilePath) || isTestFile(ref.FilePath) {
			continue
		}
		// Look for cli.App.Run patterns
		baseName := ref.Symbol
		if idx := strings.LastIndex(ref.Symbol, "."); idx >= 0 {
			baseName = ref.Symbol[idx+1:]
		}
		if baseName != "Run" {
			continue
		}
		// Only match if the symbol looks CLI-related (contains "App" or "cli")
		if !strings.Contains(ref.Symbol, "App") && !strings.Contains(ref.Symbol, "cli") {
			continue
		}
		key := fmt.Sprintf("%s:%d", ref.FilePath, ref.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:%s", ref.FilePath, ref.Symbol),
			FilePath: ref.FilePath,
			Title:    fmt.Sprintf("CLI app: %s", ref.Symbol),
			Detail:   fmt.Sprintf("%s at %s:%d", ref.Symbol, ref.FilePath, ref.Line),
			Metadata: map[string]string{
				"language": "go",
				"type":     "cli_root",
				"line":     fmt.Sprintf("%d", ref.Line),
			},
		})
	}
}

// detectPythonMain finds Python if __name__ == "__main__" patterns.
func (r *EntrypointsResult) detectPythonMain(cs CodeSearcher) {
	symbols, err := cs.FindSymbols("__main__", "", 50)
	if err != nil {
		return
	}

	for _, sym := range symbols {
		if isTestFile(sym.FilePath) {
			continue
		}
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:__main__", sym.FilePath),
			FilePath: sym.FilePath,
			Title:    "Python entry point",
			Detail:   fmt.Sprintf("__main__ block at %s:%d", sym.FilePath, sym.Line),
			Metadata: map[string]string{
				"language": "python",
				"type":     "main",
				"line":     fmt.Sprintf("%d", sym.Line),
			},
		})
	}
}

// detectRustMain finds Rust fn main() functions.
func (r *EntrypointsResult) detectRustMain(cs CodeSearcher) {
	symbols, err := cs.FindSymbols("main", "function", 100)
	if err != nil {
		return
	}

	for _, sym := range symbols {
		if sym.Name != "main" || sym.Kind != "function" || !strings.HasSuffix(sym.FilePath, ".rs") {
			continue
		}
		if isTestFile(sym.FilePath) {
			continue
		}
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerEntrypoints,
			Kind:     KindEntrypoint,
			Name:     fmt.Sprintf("%s:main()", sym.FilePath),
			FilePath: sym.FilePath,
			Title:    "Rust main() entry point",
			Detail:   fmt.Sprintf("fn main() at %s:%d", sym.FilePath, sym.Line),
			Metadata: map[string]string{
				"language": "rust",
				"type":     "main",
				"line":     fmt.Sprintf("%d", sym.Line),
			},
		})
	}
}

// =============================================================================
// File-scanning fallback — used when code index is not available
// =============================================================================

// detectEntrypointsByFileScan walks the filesystem to find obvious entrypoint
// patterns when the code index is unavailable. Less accurate than code-index-based
// detection but provides basic coverage.
func (r *EntrypointsResult) detectEntrypointsByFileScan(rootDir string) {
	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if info.IsDir() {
			base := info.Name()
			// Skip hidden dirs, vendor, node_modules, etc.
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "" {
			relPath = path
		}

		// Skip generated and test files
		if isGeneratedFile(relPath) || isTestFile(relPath) {
			return nil
		}

		switch {
		case strings.HasSuffix(relPath, ".go"):
			r.scanGoFileForEntrypoints(relPath, path)
		case strings.HasSuffix(relPath, ".py"):
			r.scanPythonFileForEntrypoints(relPath, path)
		case strings.HasSuffix(relPath, ".rs") && filepath.Base(relPath) == "main.rs":
			// Rust convention: main.rs is always an entry point
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:main()", relPath),
				FilePath: relPath,
				Title:    "Rust main() entry point",
				Detail:   fmt.Sprintf("main.rs at %s (detected by file scan)", relPath),
				Metadata: map[string]string{
					"language":  "rust",
					"type":      "main",
					"detection": "file_scan",
				},
			})
		}
		return nil
	})
}

// scanGoFileForEntrypoints scans a Go file for func main() declarations.
func (r *EntrypointsResult) scanGoFileForEntrypoints(relPath, absPath string) {
	f, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	var packageName string
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Track package name
		if strings.HasPrefix(line, "package ") {
			packageName = strings.TrimPrefix(line, "package ")
			packageName = strings.TrimSpace(packageName)
			continue
		}

		// Only look for main() in package main
		if packageName != "main" {
			continue
		}

		if line == "func main() {" || line == "func main(){" {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:main()", relPath),
				FilePath: relPath,
				Title:    "Go main() entry point",
				Detail:   fmt.Sprintf("func main() at %s:%d (detected by file scan)", relPath, lineNum),
				Metadata: map[string]string{
					"language":  "go",
					"type":      "main",
					"line":      fmt.Sprintf("%d", lineNum),
					"detection": "file_scan",
				},
			})
			return // Only one main() per file
		}
	}
}

// scanPythonFileForEntrypoints scans a Python file for if __name__ == "__main__" blocks.
func (r *EntrypointsResult) scanPythonFileForEntrypoints(relPath, absPath string) {
	f, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == `if __name__ == "__main__":` || line == `if __name__ == '__main__':` {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerEntrypoints,
				Kind:     KindEntrypoint,
				Name:     fmt.Sprintf("%s:__main__", relPath),
				FilePath: relPath,
				Title:    "Python entry point",
				Detail:   fmt.Sprintf("__main__ block at %s:%d (detected by file scan)", relPath, lineNum),
				Metadata: map[string]string{
					"language":  "python",
					"type":      "main",
					"line":      fmt.Sprintf("%d", lineNum),
					"detection": "file_scan",
				},
			})
			return // Only one per file
		}
	}
}
