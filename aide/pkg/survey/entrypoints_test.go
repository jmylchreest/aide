package survey

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockCodeSearcher implements CodeSearcher for testing.
type mockCodeSearcher struct {
	symbols    []SymbolHit
	references []ReferenceHit
	symErr     error
	refErr     error
}

func (m *mockCodeSearcher) FindSymbols(query string, kind string, limit int) ([]SymbolHit, error) {
	if m.symErr != nil {
		return nil, m.symErr
	}
	var results []SymbolHit
	for _, s := range m.symbols {
		if query != "" && s.Name != query {
			continue
		}
		if kind != "" && s.Kind != kind {
			continue
		}
		results = append(results, s)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *mockCodeSearcher) FindReferences(symbolName string, kind string, limit int) ([]ReferenceHit, error) {
	if m.refErr != nil {
		return nil, m.refErr
	}
	var results []ReferenceHit
	for _, r := range m.references {
		// The real code index does prefix/fuzzy matching via Bleve,
		// so we simulate with contains matching.
		if symbolName != "" && !strings.Contains(r.Symbol, symbolName) {
			continue
		}
		if kind != "" && r.Kind != kind {
			continue
		}
		results = append(results, r)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func TestRunEntrypoints_NilCodeSearcher(t *testing.T) {
	// With nil code searcher, RunEntrypoints falls back to file scanning.
	// Using a non-existent path ensures no files are found.
	result, err := RunEntrypoints("/tmp/nonexistent-path-for-test", nil)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries with nil code searcher and no files, got %d", len(result.Entries))
	}
}

func TestRunEntrypoints_GoMain(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
			{Name: "main", Kind: "function", FilePath: "cmd/cli/main.go", Line: 5, Language: "go"},
			{Name: "mainHelper", Kind: "function", FilePath: "internal/util.go", Line: 20, Language: "go"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	// Should find exactly 2 main() entries (not mainHelper)
	var mainEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && e.Metadata["type"] == "main" && e.Metadata["language"] == "go" {
			mainEntries = append(mainEntries, e)
		}
	}
	if len(mainEntries) != 2 {
		t.Errorf("expected 2 Go main() entries, got %d", len(mainEntries))
	}
}

func TestRunEntrypoints_GoInit(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "init", Kind: "function", FilePath: "internal/config.go", Line: 15, Language: "go"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var initEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && e.Metadata["type"] == "init" {
			initEntries = append(initEntries, e)
		}
	}
	if len(initEntries) != 1 {
		t.Errorf("expected 1 Go init() entry, got %d", len(initEntries))
	}
}

func TestRunEntrypoints_GoHTTPHandlers(t *testing.T) {
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "HandleFunc", Kind: "call", FilePath: "internal/server.go", Line: 42},
			{Symbol: "ListenAndServe", Kind: "call", FilePath: "cmd/server/main.go", Line: 100},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var httpEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && (e.Metadata["type"] == "http_handler" || e.Metadata["type"] == "server_start") {
			httpEntries = append(httpEntries, e)
		}
	}
	// HandleFunc matches "HandleFunc" (http_handler), ListenAndServe matches
	// "ListenAndServe" (server_start). With file:line dedup, we get 2 unique entries.
	if len(httpEntries) != 2 {
		t.Errorf("expected 2 HTTP-related entries, got %d", len(httpEntries))
	}
}

func TestRunEntrypoints_GoGRPC(t *testing.T) {
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "RegisterUserServiceServer", Kind: "call", FilePath: "cmd/server/main.go", Line: 55},
			{Symbol: "RegisterHealthServer", Kind: "call", FilePath: "cmd/server/main.go", Line: 60},
			{Symbol: "RegisterCallback", Kind: "call", FilePath: "internal/hooks.go", Line: 10}, // Should NOT match
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var grpcEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && e.Metadata["type"] == "grpc_service" {
			grpcEntries = append(grpcEntries, e)
		}
	}
	if len(grpcEntries) != 2 {
		t.Errorf("expected 2 gRPC service entries, got %d", len(grpcEntries))
	}
}

func TestRunEntrypoints_RustMain(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "src/main.rs", Line: 1, Language: "rust"},
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var rustEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && e.Metadata["language"] == "rust" {
			rustEntries = append(rustEntries, e)
		}
	}
	if len(rustEntries) != 1 {
		t.Errorf("expected 1 Rust main() entry, got %d", len(rustEntries))
	}
	if len(rustEntries) > 0 && rustEntries[0].FilePath != "src/main.rs" {
		t.Errorf("Rust entry FilePath = %q, want %q", rustEntries[0].FilePath, "src/main.rs")
	}
}

func TestRunEntrypoints_PythonMain(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "__main__", Kind: "", FilePath: "main.py", Line: 50, Language: "python"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var pyEntries []*Entry
	for _, e := range result.Entries {
		if e.Kind == KindEntrypoint && e.Metadata["language"] == "python" {
			pyEntries = append(pyEntries, e)
		}
	}
	if len(pyEntries) != 1 {
		t.Errorf("expected 1 Python entry, got %d", len(pyEntries))
	}
}

func TestRunEntrypoints_EntryFields(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	if len(result.Entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}

	e := result.Entries[0]
	if e.Analyzer != AnalyzerEntrypoints {
		t.Errorf("Analyzer = %q, want %q", e.Analyzer, AnalyzerEntrypoints)
	}
	if e.Kind != KindEntrypoint {
		t.Errorf("Kind = %q, want %q", e.Kind, KindEntrypoint)
	}
	if e.FilePath != "cmd/app/main.go" {
		t.Errorf("FilePath = %q, want %q", e.FilePath, "cmd/app/main.go")
	}
	if e.Name != "cmd/app/main.go:main()" {
		t.Errorf("Name = %q, want %q", e.Name, "cmd/app/main.go:main()")
	}
	if e.Metadata["line"] != "10" {
		t.Errorf("Metadata[line] = %q, want %q", e.Metadata["line"], "10")
	}
}

// =============================================================================
// Error Resilience Tests
// =============================================================================

func TestRunEntrypoints_SymbolSearchError_Resilient(t *testing.T) {
	// RunEntrypoints silently ignores symbol search errors — each detect*
	// method returns early on error, and RunEntrypoints always returns (result, nil).
	cs := &mockCodeSearcher{
		symErr: errors.New("symbol index corrupted"),
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No entries found because all detect* methods failed silently.
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result.Entries))
	}
}

func TestRunEntrypoints_ReferenceSearchError_Resilient(t *testing.T) {
	// Reference search errors are also silently ignored.
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "HandleFunc", Kind: "function", FilePath: "pkg/api/routes.go", Line: 20, Language: "go"},
		},
		refErr: errors.New("reference search failed"),
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// HandleFunc is not a recognized entrypoint pattern (not "main", "init", etc.)
	// so no entries are produced regardless of refErr. The important thing is
	// that no error is returned.
	_ = result
}

// =============================================================================
// Filtering Tests — generated files, test files, non-Go files
// =============================================================================

func TestRunEntrypoints_ExcludesGeneratedFiles(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
			{Name: "init", Kind: "function", FilePath: "pkg/grpcapi/aidememory.pb.go", Line: 100, Language: "go"},
			{Name: "init", Kind: "function", FilePath: "pkg/api/generated/server_gen.go", Line: 5, Language: "go"},
		},
		references: []ReferenceHit{
			{Symbol: "RegisterUserServiceServer", Kind: "call", FilePath: "pkg/grpcapi/aidememory_grpc.pb.go", Line: 200},
			{Symbol: "RegisterUserServiceServer", Kind: "call", FilePath: "cmd/server/main.go", Line: 55},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	for _, e := range result.Entries {
		if isGeneratedFile(e.FilePath) {
			t.Errorf("generated file should be excluded: %s", e.FilePath)
		}
	}

	// Should have: 1 main from cmd/app, 1 gRPC from cmd/server
	// Should NOT have: init from .pb.go, init from _gen.go, gRPC from _grpc.pb.go
	if len(result.Entries) != 2 {
		for _, e := range result.Entries {
			t.Logf("  entry: %s (%s)", e.Name, e.FilePath)
		}
		t.Errorf("expected 2 entries (excluding generated), got %d", len(result.Entries))
	}
}

func TestRunEntrypoints_ExcludesTestFiles(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
			{Name: "main", Kind: "function", FilePath: "pkg/parser/parser_test.go", Line: 15, Language: "go"},
			{Name: "init", Kind: "function", FilePath: "internal/testdata/helper.go", Line: 5, Language: "go"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	for _, e := range result.Entries {
		if isTestFile(e.FilePath) {
			t.Errorf("test file should be excluded: %s", e.FilePath)
		}
	}

	// Should have: 1 main from cmd/app only
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry (excluding test files), got %d", len(result.Entries))
	}
}

func TestRunEntrypoints_GoMainExcludesNonGoFiles(t *testing.T) {
	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 10, Language: "go"},
			{Name: "main", Kind: "function", FilePath: "src/main.rs", Line: 1, Language: "rust"},
			{Name: "main", Kind: "function", FilePath: "main.c", Line: 5, Language: "c"},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var goMains []*Entry
	for _, e := range result.Entries {
		if e.Metadata["language"] == "go" && e.Metadata["type"] == "main" {
			goMains = append(goMains, e)
		}
	}
	// Only cmd/app/main.go should be a Go main (not .rs or .c)
	if len(goMains) != 1 {
		t.Errorf("expected 1 Go main entry, got %d", len(goMains))
	}
	// Rust main should still be detected by detectRustMain
	var rustMains []*Entry
	for _, e := range result.Entries {
		if e.Metadata["language"] == "rust" {
			rustMains = append(rustMains, e)
		}
	}
	if len(rustMains) != 1 {
		t.Errorf("expected 1 Rust main entry, got %d", len(rustMains))
	}
}

func TestRunEntrypoints_HTTPExactMatch(t *testing.T) {
	// Ensure that symbols like "syscall.Handle" are NOT matched.
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "HandleFunc", Kind: "call", FilePath: "internal/server.go", Line: 42},
			{Symbol: "syscall.Handle", Kind: "call", FilePath: "internal/windows.go", Line: 10},
			{Symbol: "dll.Handle", Kind: "call", FilePath: "internal/dynamic.go", Line: 20},
			{Symbol: "ListenAndServe", Kind: "call", FilePath: "cmd/server/main.go", Line: 100},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var httpEntries []*Entry
	for _, e := range result.Entries {
		if e.Metadata["type"] == "http_handler" || e.Metadata["type"] == "server_start" {
			httpEntries = append(httpEntries, e)
		}
	}
	// HandleFunc (http_handler) + ListenAndServe (server_start) = 2, NOT syscall.Handle or dll.Handle
	if len(httpEntries) != 2 {
		for _, e := range httpEntries {
			t.Logf("  http entry: %s (type=%s)", e.Name, e.Metadata["type"])
		}
		t.Errorf("expected 2 HTTP-related entries (exact match), got %d", len(httpEntries))
	}
}

func TestRunEntrypoints_GRPCDeduplicatesBySymbol(t *testing.T) {
	// Same gRPC service registered in multiple files should only appear once.
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "RegisterUserServiceServer", Kind: "call", FilePath: "cmd/server/main.go", Line: 55},
			{Symbol: "RegisterUserServiceServer", Kind: "call", FilePath: "cmd/server/setup.go", Line: 30},
			{Symbol: "RegisterHealthServer", Kind: "call", FilePath: "cmd/server/main.go", Line: 60},
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var grpcEntries []*Entry
	for _, e := range result.Entries {
		if e.Metadata["type"] == "grpc_service" {
			grpcEntries = append(grpcEntries, e)
		}
	}
	// RegisterUserServiceServer deduped to 1, RegisterHealthServer = 1 → total 2
	if len(grpcEntries) != 2 {
		t.Errorf("expected 2 gRPC entries (deduplicated), got %d", len(grpcEntries))
	}
}

// =============================================================================
// CLI Detection Tests
// =============================================================================

func TestRunEntrypoints_CobraCommands(t *testing.T) {
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "Execute", Kind: "call", FilePath: "cmd/root.go", Line: 30},
			{Symbol: "AddCommand", Kind: "call", FilePath: "cmd/root.go", Line: 40},
			{Symbol: "ExecuteC", Kind: "call", FilePath: "cmd/app/main.go", Line: 15},
			{Symbol: "ExecuteQuery", Kind: "call", FilePath: "pkg/db/query.go", Line: 100}, // Should NOT match
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var cliEntries []*Entry
	for _, e := range result.Entries {
		if e.Metadata["type"] == "cli_root" || e.Metadata["type"] == "cli_command" {
			cliEntries = append(cliEntries, e)
		}
	}
	// Execute (cli_root), AddCommand (cli_command), ExecuteC (cli_root) = 3 (ExecuteQuery does NOT match)
	if len(cliEntries) != 3 {
		for _, e := range cliEntries {
			t.Logf("  cli entry: %s (type=%s)", e.Name, e.Metadata["type"])
		}
		t.Errorf("expected 3 CLI entries, got %d", len(cliEntries))
	}
}

func TestRunEntrypoints_UrfaveCLI(t *testing.T) {
	cs := &mockCodeSearcher{
		references: []ReferenceHit{
			{Symbol: "App.Run", Kind: "call", FilePath: "cmd/main.go", Line: 20},
			{Symbol: "http.Server.Run", Kind: "call", FilePath: "pkg/server.go", Line: 50}, // Should NOT match (qualifier doesn't match App|cli)
		},
	}

	result, err := RunEntrypoints("/tmp", cs)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	var cliEntries []*Entry
	for _, e := range result.Entries {
		if e.Metadata["type"] == "cli_root" {
			cliEntries = append(cliEntries, e)
		}
	}
	if len(cliEntries) != 1 {
		for _, e := range cliEntries {
			t.Logf("  cli entry: %s (type=%s)", e.Name, e.Metadata["type"])
		}
		t.Errorf("expected 1 urfave/cli entry, got %d", len(cliEntries))
	}
}

// =============================================================================
// File-Scanning Fallback Tests
// =============================================================================

func TestRunEntrypoints_FileScanFallback(t *testing.T) {
	// Create a temp directory with a Go main file
	tmpDir := t.TempDir()

	// Write a Go main file
	goDir := filepath.Join(tmpDir, "cmd", "app")
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goContent := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(filepath.Join(goDir, "main.go"), []byte(goContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a Python main file
	pyContent := "import sys\n\nif __name__ == \"__main__\":\n    main()\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "app.py"), []byte(pyContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a Rust main.rs
	rsDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(rsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rsContent := "fn main() {\n    println!(\"hello\");\n}\n"
	if err := os.WriteFile(filepath.Join(rsDir, "main.rs"), []byte(rsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a test file that should be excluded
	testContent := "package main\n\nfunc main() {\n\t// test helper\n}\n"
	if err := os.WriteFile(filepath.Join(goDir, "main_test.go"), []byte(testContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// nil code searcher triggers file-scanning fallback
	result, err := RunEntrypoints(tmpDir, nil)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	// Should find: Go main, Python main, Rust main.rs
	// Should NOT find: _test.go file
	if len(result.Entries) != 3 {
		for _, e := range result.Entries {
			t.Logf("  entry: %s (%s) detection=%s", e.Name, e.FilePath, e.Metadata["detection"])
		}
		t.Errorf("expected 3 entries from file scan, got %d", len(result.Entries))
	}

	// All should have detection=file_scan metadata
	for _, e := range result.Entries {
		if e.Metadata["detection"] != "file_scan" {
			t.Errorf("expected detection=file_scan for %s, got %q", e.Name, e.Metadata["detection"])
		}
	}
}

func TestRunEntrypoints_FileScanSkipsNonMainPackage(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a Go file in package "lib" (not "main") with func main()
	content := "package lib\n\nfunc main() {\n\t// this should not be detected\n}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "lib.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunEntrypoints(tmpDir, nil)
	if err != nil {
		t.Fatalf("RunEntrypoints: %v", err)
	}

	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for non-main package, got %d", len(result.Entries))
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestIsGeneratedFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"pkg/api/server.go", false},
		{"pkg/api/server.pb.go", true},
		{"pkg/api/server.pb.gw.go", true},
		{"pkg/api/server_generated.go", true},
		{"pkg/api/server_gen.go", true},
		{"vendor/lib/foo.go", true},
		{"cmd/main.go", false},
	}
	for _, tt := range tests {
		if got := isGeneratedFile(tt.path); got != tt.want {
			t.Errorf("isGeneratedFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"pkg/api/server.go", false},
		{"pkg/api/server_test.go", true},
		{"internal/testdata/helper.go", true},
		{"test/fixtures.go", true},
		{"tests/integration.go", true},
		{"cmd/main.go", false},
	}
	for _, tt := range tests {
		if got := isTestFile(tt.path); got != tt.want {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// Note: isHTTPSymbol and isCobraSymbol tests were removed because these functions
// were replaced by data-driven qualifier/name_match patterns in pack.json.
// The equivalent filtering logic is tested through the end-to-end tests above
// (TestRunEntrypoints_HTTPExactMatch, TestRunEntrypoints_CobraCommands).
