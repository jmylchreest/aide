package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Mock SurveyStore
// =============================================================================

type mockSurveyStore struct {
	entries    []*survey.Entry
	stats      *survey.Stats
	addErr     error
	getErr     error
	deleteErr  error
	searchErr  error
	listErr    error
	statsErr   error
	clearErr   error
	replaceErr error
}

func newMockSurveyStore() *mockSurveyStore {
	return &mockSurveyStore{
		stats: &survey.Stats{
			Total:      0,
			ByAnalyzer: map[string]int{},
			ByKind:     map[string]int{},
		},
	}
}

func (m *mockSurveyStore) AddEntry(e *survey.Entry) error {
	if m.addErr != nil {
		return m.addErr
	}
	e.ID = "test-id"
	e.CreatedAt = time.Now()
	m.entries = append(m.entries, e)
	return nil
}

func (m *mockSurveyStore) GetEntry(id string) (*survey.Entry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, e := range m.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockSurveyStore) DeleteEntry(id string) error { return m.deleteErr }

func (m *mockSurveyStore) SearchEntries(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	var results []*survey.SearchResult
	for _, e := range m.entries {
		if query == "" || strings.Contains(e.Title, query) || strings.Contains(e.Name, query) {
			if opts.Analyzer != "" && e.Analyzer != opts.Analyzer {
				continue
			}
			if opts.Kind != "" && e.Kind != opts.Kind {
				continue
			}
			if opts.FilePath != "" && !strings.Contains(e.FilePath, opts.FilePath) {
				continue
			}
			results = append(results, &survey.SearchResult{Entry: e, Score: 1.0})
		}
		if opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}
	}
	return results, nil
}

func (m *mockSurveyStore) ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var results []*survey.Entry
	for _, e := range m.entries {
		if opts.Analyzer != "" && e.Analyzer != opts.Analyzer {
			continue
		}
		if opts.Kind != "" && e.Kind != opts.Kind {
			continue
		}
		if opts.FilePath != "" && !strings.Contains(e.FilePath, opts.FilePath) {
			continue
		}
		results = append(results, e)
		if opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}
	}
	return results, nil
}

func (m *mockSurveyStore) GetFileEntries(filePath string) ([]*survey.Entry, error) {
	var results []*survey.Entry
	for _, e := range m.entries {
		if e.FilePath == filePath {
			results = append(results, e)
		}
	}
	return results, nil
}

func (m *mockSurveyStore) ClearAnalyzer(analyzer string) (int, error) {
	if m.clearErr != nil {
		return 0, m.clearErr
	}
	return 0, nil
}

func (m *mockSurveyStore) ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error {
	if m.replaceErr != nil {
		return m.replaceErr
	}
	// Remove old entries for this analyzer, add new ones
	var kept []*survey.Entry
	for _, e := range m.entries {
		if e.Analyzer != analyzer {
			kept = append(kept, e)
		}
	}
	kept = append(kept, newEntries...)
	m.entries = kept
	return nil
}

func (m *mockSurveyStore) ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error {
	return m.replaceErr
}

func (m *mockSurveyStore) Stats(_ survey.SearchOptions) (*survey.Stats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.stats, nil
}

func (m *mockSurveyStore) Clear() error { return m.clearErr }
func (m *mockSurveyStore) Close() error { return nil }

// =============================================================================
// MCP Handler Tests: handleSurveySearch
// =============================================================================

func newTestMCPServer(ss *mockSurveyStore) *MCPServer {
	return &MCPServer{
		surveyStore: ss,
	}
}

func TestHandleSurveySearch_NilStore(t *testing.T) {
	s := &MCPServer{surveyStore: nil}
	result, _, err := s.handleSurveySearch(context.Background(), nil, SurveySearchInput{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "survey store not available")
}

func TestHandleSurveySearch_NoResults(t *testing.T) {
	ms := newMockSurveyStore()
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveySearch(context.Background(), nil, SurveySearchInput{Query: "nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "No survey entries found")
}

func TestHandleSurveySearch_WithResults(t *testing.T) {
	ms := newMockSurveyStore()
	ms.entries = []*survey.Entry{
		{ID: "1", Analyzer: "topology", Kind: "module", Name: "auth-module", Title: "Auth module", FilePath: "pkg/auth"},
		{ID: "2", Analyzer: "topology", Kind: "module", Name: "db-module", Title: "Database module", FilePath: "pkg/db"},
	}
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveySearch(context.Background(), nil, SurveySearchInput{Query: "Auth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "Found 1 entries")
	assertTextContains(t, result, "pkg/auth")
}

func TestHandleSurveySearch_Error(t *testing.T) {
	ms := newMockSurveyStore()
	ms.searchErr = errors.New("index corrupted")
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveySearch(context.Background(), nil, SurveySearchInput{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "search failed")
}

func TestHandleSurveySearch_WithFilters(t *testing.T) {
	ms := newMockSurveyStore()
	ms.entries = []*survey.Entry{
		{ID: "1", Analyzer: "topology", Kind: "module", Name: "auth", Title: "auth", FilePath: "pkg/auth"},
		{ID: "2", Analyzer: "churn", Kind: "churn", Name: "hot-file", Title: "hot-file", FilePath: "pkg/auth/handler.go"},
	}
	s := newTestMCPServer(ms)

	// Filter by analyzer
	result, _, _ := s.handleSurveySearch(context.Background(), nil, SurveySearchInput{
		Query:    "auth",
		Analyzer: "topology",
	})
	assertTextContains(t, result, "Found 1 entries")
}

// =============================================================================
// MCP Handler Tests: handleSurveyList
// =============================================================================

func TestHandleSurveyList_NilStore(t *testing.T) {
	s := &MCPServer{surveyStore: nil}
	result, _, err := s.handleSurveyList(context.Background(), nil, SurveyListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "survey store not available")
}

func TestHandleSurveyList_NoResults(t *testing.T) {
	ms := newMockSurveyStore()
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyList(context.Background(), nil, SurveyListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "No survey entries found")
}

func TestHandleSurveyList_WithEntries(t *testing.T) {
	ms := newMockSurveyStore()
	ms.entries = []*survey.Entry{
		{ID: "1", Analyzer: "topology", Kind: "module", Name: "pkg/auth", Title: "Auth module", FilePath: "pkg/auth"},
		{ID: "2", Analyzer: "topology", Kind: "tech_stack", Name: "Go", Title: "Go language", FilePath: ""},
	}
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyList(context.Background(), nil, SurveyListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "Found 2 entries")
}

func TestHandleSurveyList_FilterByKind(t *testing.T) {
	ms := newMockSurveyStore()
	ms.entries = []*survey.Entry{
		{ID: "1", Analyzer: "topology", Kind: "module", Name: "pkg/auth", Title: "Auth", FilePath: "pkg/auth"},
		{ID: "2", Analyzer: "topology", Kind: "tech_stack", Name: "Go", Title: "Go", FilePath: ""},
	}
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyList(context.Background(), nil, SurveyListInput{Kind: "module"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "Found 1 entries")
}

func TestHandleSurveyList_Error(t *testing.T) {
	ms := newMockSurveyStore()
	ms.listErr = errors.New("store offline")
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyList(context.Background(), nil, SurveyListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "list failed")
}

// =============================================================================
// MCP Handler Tests: handleSurveyStats
// =============================================================================

func TestHandleSurveyStats_NilStore(t *testing.T) {
	s := &MCPServer{surveyStore: nil}
	result, _, err := s.handleSurveyStats(context.Background(), nil, SurveyStatsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "survey store not available")
}

func TestHandleSurveyStats_Empty(t *testing.T) {
	ms := newMockSurveyStore()
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyStats(context.Background(), nil, SurveyStatsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "Total survey entries: 0")
}

func TestHandleSurveyStats_WithData(t *testing.T) {
	ms := newMockSurveyStore()
	ms.stats = &survey.Stats{
		Total:      25,
		ByAnalyzer: map[string]int{"topology": 15, "churn": 10},
		ByKind:     map[string]int{"module": 8, "churn": 10, "tech_stack": 7},
	}
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyStats(context.Background(), nil, SurveyStatsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "Total survey entries: 25")
	assertTextContains(t, result, "By analyzer")
	assertTextContains(t, result, "By kind")
}

func TestHandleSurveyStats_Error(t *testing.T) {
	ms := newMockSurveyStore()
	ms.statsErr = errors.New("db locked")
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyStats(context.Background(), nil, SurveyStatsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "stats failed")
}

// =============================================================================
// MCP Handler Tests: handleSurveyGraph
// =============================================================================

func TestHandleSurveyGraph_EmptySymbol(t *testing.T) {
	s := &MCPServer{}
	result, _, err := s.handleSurveyGraph(context.Background(), nil, SurveyGraphInput{Symbol: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "symbol name is required")
}

func TestHandleSurveyGraph_NoCodeStore(t *testing.T) {
	s := &MCPServer{}
	// codeStoreReady is false by default, so getCodeStore returns nil
	result, _, err := s.handleSurveyGraph(context.Background(), nil, SurveyGraphInput{Symbol: "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "code index not available")
}

// =============================================================================
// MCP Handler Tests: handleSurveyRun
// =============================================================================

func TestHandleSurveyRun_NilStore(t *testing.T) {
	s := &MCPServer{surveyStore: nil}
	result, _, err := s.handleSurveyRun(context.Background(), nil, SurveyRunInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "survey store not available")
}

func TestHandleSurveyRun_UnknownAnalyzer(t *testing.T) {
	ms := newMockSurveyStore()
	s := newTestMCPServer(ms)

	result, _, err := s.handleSurveyRun(context.Background(), nil, SurveyRunInput{Analyzer: "bogus"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertTextContains(t, result, "unknown analyzer: bogus")
}

// =============================================================================
// Formatting helper tests
// =============================================================================

func TestFormatSurveyEntryLine(t *testing.T) {
	tests := []struct {
		name     string
		entry    *survey.Entry
		contains string
	}{
		{
			name:     "with file path",
			entry:    &survey.Entry{Kind: "module", Name: "auth", FilePath: "pkg/auth", Title: "Auth module", Analyzer: "topology"},
			contains: "[MODULE]",
		},
		{
			name:     "without file path uses name",
			entry:    &survey.Entry{Kind: "tech_stack", Name: "Go 1.21", FilePath: "", Title: "Go language", Analyzer: "topology"},
			contains: "Go 1.21",
		},
		{
			name:     "includes analyzer",
			entry:    &survey.Entry{Kind: "churn", Name: "hot.go", FilePath: "hot.go", Title: "High churn", Analyzer: "churn"},
			contains: "(churn)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := formatSurveyEntryLine(tt.entry)
			if !strings.Contains(line, tt.contains) {
				t.Errorf("formatSurveyEntryLine() = %q, want to contain %q", line, tt.contains)
			}
		})
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// =============================================================================
// Mock CodeIndexStore — for adapter tests
// =============================================================================

type mockCodeIndexStore struct {
	symbols       []*code.Symbol
	references    []*code.Reference
	fileRefs      map[string][]*code.Reference
	containingSym map[string]*code.Symbol // key: "file:line"

	searchSymErr  error
	searchRefErr  error
	fileRefsErr   error
	containingErr error
}

func newMockCodeIndexStore() *mockCodeIndexStore {
	return &mockCodeIndexStore{
		fileRefs:      make(map[string][]*code.Reference),
		containingSym: make(map[string]*code.Symbol),
	}
}

func (m *mockCodeIndexStore) AddSymbol(sym *code.Symbol) error          { return nil }
func (m *mockCodeIndexStore) GetSymbol(id string) (*code.Symbol, error) { return nil, nil }
func (m *mockCodeIndexStore) DeleteSymbol(id string) error              { return nil }
func (m *mockCodeIndexStore) GetFileSymbols(filePath string) ([]*code.Symbol, error) {
	return nil, nil
}
func (m *mockCodeIndexStore) GetFileInfo(path string) (*code.FileInfo, error) { return nil, nil }
func (m *mockCodeIndexStore) ListAllFileInfo() ([]*code.FileInfo, error)      { return nil, nil }
func (m *mockCodeIndexStore) SetFileInfo(info *code.FileInfo) error           { return nil }
func (m *mockCodeIndexStore) ClearFile(filePath string) error                 { return nil }
func (m *mockCodeIndexStore) AddReference(ref *code.Reference) error          { return nil }
func (m *mockCodeIndexStore) ClearFileReferences(filePath string) error       { return nil }
func (m *mockCodeIndexStore) Stats() (*code.IndexStats, error)                { return nil, nil }
func (m *mockCodeIndexStore) Clear() error                                    { return nil }
func (m *mockCodeIndexStore) Close() error                                    { return nil }

func (m *mockCodeIndexStore) SearchSymbols(query string, opts code.SearchOptions) ([]*store.CodeSearchResult, error) {
	if m.searchSymErr != nil {
		return nil, m.searchSymErr
	}
	results := make([]*store.CodeSearchResult, len(m.symbols))
	for i, s := range m.symbols {
		results[i] = &store.CodeSearchResult{Symbol: s, Score: 1.0}
	}
	return results, nil
}

func (m *mockCodeIndexStore) SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error) {
	if m.searchRefErr != nil {
		return nil, m.searchRefErr
	}
	return m.references, nil
}

func (m *mockCodeIndexStore) GetFileReferences(filePath string) ([]*code.Reference, error) {
	if m.fileRefsErr != nil {
		return nil, m.fileRefsErr
	}
	return m.fileRefs[filePath], nil
}

func (m *mockCodeIndexStore) GetContainingSymbol(filePath string, line int) (*code.Symbol, error) {
	if m.containingErr != nil {
		return nil, m.containingErr
	}
	key := fmt.Sprintf("%s:%d", filePath, line)
	sym := m.containingSym[key]
	return sym, nil
}

func (m *mockCodeIndexStore) TopReferencedSymbols(limit int, kind string) ([]*code.SymbolRefCount, error) {
	return nil, nil
}

func (m *mockCodeIndexStore) ListAllSymbols(limit int) ([]*code.Symbol, error) {
	return m.symbols, nil
}

// =============================================================================
// codeSearcherAdapter Tests
// =============================================================================

func TestCodeSearcherAdapter_FindSymbols(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.symbols = []*code.Symbol{
		{Name: "getUser", Kind: "function", FilePath: "pkg/auth/user.go", StartLine: 42, Language: "go"},
		{Name: "setUser", Kind: "method", FilePath: "pkg/auth/user.go", StartLine: 88, Language: "go"},
	}
	adapter := &codeSearcherAdapter{store: ms}

	hits, err := adapter.FindSymbols("user", "function", 10)
	if err != nil {
		t.Fatalf("FindSymbols: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	// Verify field mapping
	h := hits[0]
	if h.Name != "getUser" {
		t.Errorf("Name = %q, want %q", h.Name, "getUser")
	}
	if h.Kind != "function" {
		t.Errorf("Kind = %q, want %q", h.Kind, "function")
	}
	if h.FilePath != "pkg/auth/user.go" {
		t.Errorf("FilePath = %q, want %q", h.FilePath, "pkg/auth/user.go")
	}
	if h.Line != 42 {
		t.Errorf("Line = %d, want %d", h.Line, 42)
	}
	if h.Language != "go" {
		t.Errorf("Language = %q, want %q", h.Language, "go")
	}
}

func TestCodeSearcherAdapter_FindSymbols_Error(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.searchSymErr = errors.New("index corrupted")
	adapter := &codeSearcherAdapter{store: ms}

	_, err := adapter.FindSymbols("test", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "index corrupted") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "index corrupted")
	}
}

func TestCodeSearcherAdapter_FindReferences(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.references = []*code.Reference{
		{SymbolName: "getUser", Kind: "call", FilePath: "cmd/main.go", Line: 15},
		{SymbolName: "getUser", Kind: "type_ref", FilePath: "pkg/api/handler.go", Line: 73},
	}
	adapter := &codeSearcherAdapter{store: ms}

	hits, err := adapter.FindReferences("getUser", "call", 10)
	if err != nil {
		t.Fatalf("FindReferences: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	// Verify field mapping
	h := hits[0]
	if h.Symbol != "getUser" {
		t.Errorf("Symbol = %q, want %q", h.Symbol, "getUser")
	}
	if h.Kind != "call" {
		t.Errorf("Kind = %q, want %q", h.Kind, "call")
	}
	if h.FilePath != "cmd/main.go" {
		t.Errorf("FilePath = %q, want %q", h.FilePath, "cmd/main.go")
	}
	if h.Line != 15 {
		t.Errorf("Line = %d, want %d", h.Line, 15)
	}
}

func TestCodeSearcherAdapter_FindReferences_Error(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.searchRefErr = errors.New("search failed")
	adapter := &codeSearcherAdapter{store: ms}

	_, err := adapter.FindReferences("test", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCodeSearcherAdapter_EmptyResults(t *testing.T) {
	ms := newMockCodeIndexStore()
	adapter := &codeSearcherAdapter{store: ms}

	hits, err := adapter.FindSymbols("nothing", "", 0)
	if err != nil {
		t.Fatalf("FindSymbols: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}

	refs, err := adapter.FindReferences("nothing", "", 0)
	if err != nil {
		t.Fatalf("FindReferences: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}

// =============================================================================
// codeGrapherAdapter Tests
// =============================================================================

func TestCodeGrapherAdapter_GetFileReferences(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.fileRefs["pkg/auth/user.go"] = []*code.Reference{
		{SymbolName: "getUser", Kind: "call", FilePath: "pkg/auth/user.go", Line: 55},
		{SymbolName: "setUser", Kind: "call", FilePath: "pkg/auth/user.go", Line: 60},
	}
	adapter := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: ms}}

	hits, err := adapter.GetFileReferences("pkg/auth/user.go")
	if err != nil {
		t.Fatalf("GetFileReferences: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	h := hits[0]
	if h.Symbol != "getUser" {
		t.Errorf("Symbol = %q, want %q", h.Symbol, "getUser")
	}
	if h.Kind != "call" {
		t.Errorf("Kind = %q, want %q", h.Kind, "call")
	}
	if h.FilePath != "pkg/auth/user.go" {
		t.Errorf("FilePath = %q, want %q", h.FilePath, "pkg/auth/user.go")
	}
	if h.Line != 55 {
		t.Errorf("Line = %d, want %d", h.Line, 55)
	}
}

func TestCodeGrapherAdapter_GetFileReferences_Error(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.fileRefsErr = errors.New("file refs failed")
	adapter := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: ms}}

	_, err := adapter.GetFileReferences("any.go")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCodeGrapherAdapter_GetContainingSymbol(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.containingSym["pkg/auth/user.go:55"] = &code.Symbol{
		Name:      "handleAuth",
		Kind:      "function",
		FilePath:  "pkg/auth/user.go",
		StartLine: 50,
		EndLine:   70,
		Language:  "go",
	}
	adapter := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: ms}}

	hit, err := adapter.GetContainingSymbol("pkg/auth/user.go", 55)
	if err != nil {
		t.Fatalf("GetContainingSymbol: %v", err)
	}
	if hit == nil {
		t.Fatal("expected non-nil symbol hit")
	}

	if hit.Name != "handleAuth" {
		t.Errorf("Name = %q, want %q", hit.Name, "handleAuth")
	}
	if hit.Kind != "function" {
		t.Errorf("Kind = %q, want %q", hit.Kind, "function")
	}
	if hit.FilePath != "pkg/auth/user.go" {
		t.Errorf("FilePath = %q, want %q", hit.FilePath, "pkg/auth/user.go")
	}
	if hit.Line != 50 {
		t.Errorf("Line = %d, want %d", hit.Line, 50)
	}
	if hit.EndLine != 70 {
		t.Errorf("EndLine = %d, want %d", hit.EndLine, 70)
	}
	if hit.Language != "go" {
		t.Errorf("Language = %q, want %q", hit.Language, "go")
	}
}

func TestCodeGrapherAdapter_GetContainingSymbol_Error(t *testing.T) {
	ms := newMockCodeIndexStore()
	ms.containingErr = errors.New("containing lookup failed")
	adapter := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: ms}}

	_, err := adapter.GetContainingSymbol("any.go", 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCodeGrapherAdapter_GetContainingSymbol_NotFound(t *testing.T) {
	ms := newMockCodeIndexStore()
	// No containing symbol set
	adapter := &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: ms}}

	hit, err := adapter.GetContainingSymbol("unknown.go", 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit != nil {
		t.Errorf("expected nil hit, got %+v", hit)
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

func assertIsError(t *testing.T, result *mcp.CallToolResult, contains string) {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	text := extractText(result)
	if !strings.Contains(text, contains) {
		t.Errorf("error text %q does not contain %q", text, contains)
	}
}

func assertTextContains(t *testing.T, result *mcp.CallToolResult, contains string) {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	text := extractText(result)
	if !strings.Contains(text, contains) {
		t.Errorf("result text %q does not contain %q", text, contains)
	}
}

func extractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}
