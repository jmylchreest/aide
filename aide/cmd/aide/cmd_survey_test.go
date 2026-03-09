package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// TestCmdSurveyRun_EntrypointsWithCodeIndex verifies that cmdSurveyRun wires
// the code index into the entrypoints analyzer when the code store exists.
func TestCmdSurveyRun_EntrypointsWithCodeIndex(t *testing.T) {
	// Create temp project directory with .aide/memory/ structure
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Create a Go source file so topology/entrypoints have something to find
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-create the code store and populate it with a main symbol
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	err = codeStore.AddSymbol(&code.Symbol{
		ID:        "sym-main-1",
		Name:      "main",
		Kind:      "function",
		FilePath:  "main.go",
		Language:  "go",
		StartLine: 3,
		EndLine:   3,
		Signature: "func main()",
	})
	if err != nil {
		codeStore.Close()
		t.Fatal(err)
	}
	codeStore.Close()

	// chdir into the project so os.Getwd() returns the project root
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	// Run the entrypoints analyzer via CLI
	err = cmdSurveyRun(dbPath, []string{"--analyzer=entrypoints"})
	if err != nil {
		t.Fatalf("cmdSurveyRun failed: %v", err)
	}

	// Verify that the survey store now has entrypoint entries
	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatalf("failed to open survey store: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ListEntries(survey.SearchOptions{
		Analyzer: survey.AnalyzerEntrypoints,
		Kind:     survey.KindEntrypoint,
	})
	if err != nil {
		t.Fatalf("failed to list entries: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected entrypoint entries from code index, got 0")
	}

	// Verify at least one entry is the Go main entrypoint
	foundMain := false
	for _, e := range entries {
		if e.Kind == survey.KindEntrypoint && e.FilePath == "main.go" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Error("expected to find main.go entrypoint entry")
	}
}

// TestCmdSurveyRun_EntrypointsWithoutCodeIndex verifies graceful fallback
// when the code index doesn't exist (user hasn't run 'aide code index').
func TestCmdSurveyRun_EntrypointsWithoutCodeIndex(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// No code store created — should fall back gracefully

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	// Should succeed without error, just produce 0 entries
	err = cmdSurveyRun(dbPath, []string{"--analyzer=entrypoints"})
	if err != nil {
		t.Fatalf("cmdSurveyRun failed: %v", err)
	}

	// Verify survey store has 0 entrypoint entries (graceful degradation)
	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatalf("failed to open survey store: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ListEntries(survey.SearchOptions{
		Analyzer: survey.AnalyzerEntrypoints,
	})
	if err != nil {
		t.Fatalf("failed to list entries: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entrypoint entries without code index, got %d", len(entries))
	}
}

// TestCmdSurveyRun_AllAnalyzers verifies running all analyzers together.
func TestCmdSurveyRun_AllAnalyzers(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Create a Go project structure
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	// Run all analyzers (no --analyzer flag)
	err = cmdSurveyRun(dbPath, nil)
	if err != nil {
		t.Fatalf("cmdSurveyRun failed: %v", err)
	}

	// Verify survey store has topology entries (at minimum, from go.mod)
	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatalf("failed to open survey store: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ListEntries(survey.SearchOptions{
		Analyzer: survey.AnalyzerTopology,
	})
	if err != nil {
		t.Fatalf("failed to list topology entries: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected topology entries from go.mod project, got 0")
	}
}

// TestCmdSurveyRun_AnalyserAlias verifies that --analyser= (British spelling)
// works as an alias for --analyzer= via parseFlag.
func TestCmdSurveyRun_AnalyserAlias(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	// Use British spelling --analyser=
	err = cmdSurveyRun(dbPath, []string{"--analyser=topology"})
	if err != nil {
		t.Fatalf("cmdSurveyRun with --analyser= failed: %v", err)
	}

	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatalf("failed to open survey store: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ListEntries(survey.SearchOptions{Analyzer: survey.AnalyzerTopology})
	if err != nil {
		t.Fatalf("failed to list entries: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected topology entries from --analyser= alias, got 0")
	}
}

// TestCmdSurveyList_JSON verifies that --json flag produces valid JSON output.
func TestCmdSurveyList_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Pre-populate the survey store with a test entry.
	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatal(err)
	}
	err = ss.AddEntry(&survey.Entry{
		Analyzer: "topology",
		Kind:     "module",
		Name:     "test-module",
		Title:    "Test Module",
	})
	if err != nil {
		ss.Close()
		t.Fatal(err)
	}
	ss.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdSurveyList(dbPath, []string{"--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdSurveyList --json failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Verify it's valid JSON array
	var entries []*survey.Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in JSON output, got %d", len(entries))
	}
	if entries[0].Name != "test-module" {
		t.Errorf("expected entry name 'test-module', got %q", entries[0].Name)
	}
}

// TestCmdSurveySearch_JSON verifies that search --json produces valid JSON.
func TestCmdSurveySearch_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Pre-populate with a searchable entry
	surveyDir := getSurveyStorePath(dbPath)
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		t.Fatal(err)
	}
	err = ss.AddEntry(&survey.Entry{
		Analyzer: "topology",
		Kind:     "module",
		Name:     "auth-service",
		Title:    "Authentication Service Module",
	})
	if err != nil {
		ss.Close()
		t.Fatal(err)
	}
	ss.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdSurveySearch(dbPath, []string{"auth", "--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdSurveySearch --json failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var entries []*survey.Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in JSON output, got %d", len(entries))
	}
}

// TestCmdSurveyGraph_NoCodeIndex verifies that graph gives a useful error
// when the code index doesn't exist.
func TestCmdSurveyGraph_NoCodeIndex(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	err := cmdSurveyGraph(dbPath, []string{"main"})
	if err == nil {
		t.Fatal("expected error when code index doesn't exist")
	}
	if !strings.Contains(err.Error(), "code index") {
		t.Errorf("error should mention 'code index', got: %v", err)
	}
}

// TestCmdSurveyGraph_NoSymbol verifies that graph requires a symbol name.
func TestCmdSurveyGraph_NoSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	err := cmdSurveyGraph(dbPath, nil)
	if err == nil {
		t.Fatal("expected error when no symbol provided")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error should show usage, got: %v", err)
	}
}

// TestCmdSurveyGraph_WithCodeIndex verifies that graph works when the code
// index has matching symbols.
func TestCmdSurveyGraph_WithCodeIndex(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Create code store with a symbol
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	err = codeStore.AddSymbol(&code.Symbol{
		ID:        "sym-1",
		Name:      "handleRequest",
		Kind:      "function",
		FilePath:  "server.go",
		Language:  "go",
		StartLine: 10,
		EndLine:   20,
		Signature: "func handleRequest()",
	})
	if err != nil {
		codeStore.Close()
		t.Fatal(err)
	}
	codeStore.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdSurveyGraph(dbPath, []string{"handleRequest"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdSurveyGraph failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "handleRequest") {
		t.Errorf("output should contain symbol name, got: %s", output)
	}
	if !strings.Contains(output, "Call graph") {
		t.Errorf("output should contain 'Call graph' header, got: %s", output)
	}
}

// TestCmdSurveyGraph_JSONOutput verifies JSON output from graph command.
func TestCmdSurveyGraph_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Create code store with a symbol
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	err = codeStore.AddSymbol(&code.Symbol{
		ID:        "sym-1",
		Name:      "processData",
		Kind:      "function",
		FilePath:  "processor.go",
		Language:  "go",
		StartLine: 5,
		EndLine:   15,
		Signature: "func processData()",
	})
	if err != nil {
		codeStore.Close()
		t.Fatal(err)
	}
	codeStore.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdSurveyGraph(dbPath, []string{"processData", "--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdSurveyGraph --json failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var graph survey.CallGraph
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &graph); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if graph.Root != "processData" {
		t.Errorf("expected root 'processData', got %q", graph.Root)
	}
}

// TestCmdSurveyGraph_SymbolFlag verifies --symbol= flag works.
func TestCmdSurveyGraph_SymbolFlag(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// Create code store with a symbol
	indexPath, searchPath := getCodeStorePaths(dbPath)
	codeStore, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	err = codeStore.AddSymbol(&code.Symbol{
		ID:        "sym-1",
		Name:      "myFunc",
		Kind:      "function",
		FilePath:  "app.go",
		Language:  "go",
		StartLine: 1,
		EndLine:   5,
		Signature: "func myFunc()",
	})
	if err != nil {
		codeStore.Close()
		t.Fatal(err)
	}
	codeStore.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdSurveyGraph(dbPath, []string{"--symbol=myFunc", "--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdSurveyGraph --symbol= failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var graph survey.CallGraph
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &graph); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if graph.Root != "myFunc" {
		t.Errorf("expected root 'myFunc', got %q", graph.Root)
	}
}

// TestPrintSurveyJSON_EmptySlice verifies that an empty slice produces [].
func TestPrintSurveyJSON_EmptySlice(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printSurveyJSON([]*survey.Entry{})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("printSurveyJSON failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	if output != "[]" {
		t.Errorf("expected '[]', got %q", output)
	}
}

// TestCmdSurveyDispatcher_Graph verifies the dispatcher routes to graph.
func TestCmdSurveyDispatcher_Graph(t *testing.T) {
	tmpDir := t.TempDir()
	aideDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")

	// graph with no symbol should return usage error
	err := cmdSurveyDispatcher(dbPath, []string{"graph"})
	if err == nil {
		t.Fatal("expected error for graph with no symbol")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error should show usage, got: %v", err)
	}
}
