package store

import (
	"os"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/survey"
)

func setupTestSurveyStore(t *testing.T) (*SurveyStoreImpl, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-survey-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	ss, err := NewSurveyStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create survey store: %v", err)
	}

	cleanup := func() {
		ss.Close()
		os.RemoveAll(tmpDir)
	}

	return ss, cleanup
}

func TestSurveyStore_AddAndGet(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entry := &survey.Entry{
		Analyzer: survey.AnalyzerTopology,
		Kind:     survey.KindModule,
		Name:     "aide/pkg/store",
		FilePath: "aide/pkg/store",
		Title:    "Store package",
		Detail:   "BoltDB + Bleve storage layer",
		Metadata: map[string]string{"language": "go"},
	}

	if err := ss.AddEntry(entry); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	if entry.ID == "" {
		t.Fatal("expected ID to be assigned")
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}

	// Get it back
	got, err := ss.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	if got.Name != "aide/pkg/store" {
		t.Errorf("expected Name %q, got %q", "aide/pkg/store", got.Name)
	}
	if got.Analyzer != survey.AnalyzerTopology {
		t.Errorf("expected Analyzer %q, got %q", survey.AnalyzerTopology, got.Analyzer)
	}
	if got.Kind != survey.KindModule {
		t.Errorf("expected Kind %q, got %q", survey.KindModule, got.Kind)
	}
	if got.Metadata["language"] != "go" {
		t.Errorf("expected Metadata[language] %q, got %q", "go", got.Metadata["language"])
	}
}

func TestSurveyStore_GetNotFound(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	_, err := ss.GetEntry("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSurveyStore_Delete(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entry := &survey.Entry{
		Analyzer: survey.AnalyzerTopology,
		Kind:     survey.KindModule,
		Name:     "test-mod",
		Title:    "Test module",
	}
	if err := ss.AddEntry(entry); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	if err := ss.DeleteEntry(entry.ID); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	_, err := ss.GetEntry(entry.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSurveyStore_ListEntries(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "mod-a", Title: "Module A"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindTechStack, Name: "go", Title: "Go language"},
		{Analyzer: survey.AnalyzerEntrypoints, Kind: survey.KindEntrypoint, Name: "main", Title: "Main entry", FilePath: "cmd/main.go"},
		{Analyzer: survey.AnalyzerChurn, Kind: survey.KindChurn, Name: "hot-file", Title: "Hot file", FilePath: "pkg/hot.go"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	// List all
	all, err := ss.ListEntries(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 entries, got %d", len(all))
	}

	// Filter by analyzer
	topOnly, err := ss.ListEntries(survey.SearchOptions{Analyzer: survey.AnalyzerTopology})
	if err != nil {
		t.Fatalf("ListEntries(topology): %v", err)
	}
	if len(topOnly) != 2 {
		t.Errorf("expected 2 topology entries, got %d", len(topOnly))
	}

	// Filter by kind
	modOnly, err := ss.ListEntries(survey.SearchOptions{Kind: survey.KindModule})
	if err != nil {
		t.Fatalf("ListEntries(module): %v", err)
	}
	if len(modOnly) != 1 {
		t.Errorf("expected 1 module entry, got %d", len(modOnly))
	}

	// Filter by limit
	limited, err := ss.ListEntries(survey.SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListEntries(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(limited))
	}
}

func TestSurveyStore_SearchEntries(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "authentication", Title: "Auth module", Detail: "Handles JWT authentication"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "database", Title: "Database layer", Detail: "PostgreSQL access"},
		{Analyzer: survey.AnalyzerEntrypoints, Kind: survey.KindEntrypoint, Name: "api-server", Title: "API server entry", FilePath: "cmd/api/main.go"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	// Search by name
	results, err := ss.SearchEntries("authentication", survey.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchEntries: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'authentication'")
	}
	if results[0].Entry.Name != "authentication" {
		t.Errorf("expected first result Name %q, got %q", "authentication", results[0].Entry.Name)
	}

	// Search with analyzer filter
	results, err = ss.SearchEntries("server", survey.SearchOptions{Analyzer: survey.AnalyzerEntrypoints})
	if err != nil {
		t.Fatalf("SearchEntries: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'server' with entrypoints filter")
	}
	for _, r := range results {
		if r.Entry.Analyzer != survey.AnalyzerEntrypoints {
			t.Errorf("expected Analyzer %q, got %q", survey.AnalyzerEntrypoints, r.Entry.Analyzer)
		}
	}
}

func TestSurveyStore_GetFileEntries(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "store", Title: "Store", FilePath: "pkg/store"},
		{Analyzer: survey.AnalyzerEntrypoints, Kind: survey.KindEntrypoint, Name: "main", Title: "Main", FilePath: "cmd/main.go"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindTechStack, Name: "go", Title: "Go", FilePath: "pkg/store"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	got, err := ss.GetFileEntries("pkg/store")
	if err != nil {
		t.Fatalf("GetFileEntries: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries for pkg/store, got %d", len(got))
	}
}

func TestSurveyStore_ClearAnalyzer(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "a", Title: "A"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "b", Title: "B"},
		{Analyzer: survey.AnalyzerChurn, Kind: survey.KindChurn, Name: "c", Title: "C"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	count, err := ss.ClearAnalyzer(survey.AnalyzerTopology)
	if err != nil {
		t.Fatalf("ClearAnalyzer: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 cleared, got %d", count)
	}

	// Verify only churn remains
	all, err := ss.ListEntries(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 entry remaining, got %d", len(all))
	}
	if all[0].Analyzer != survey.AnalyzerChurn {
		t.Errorf("expected remaining entry to be churn, got %q", all[0].Analyzer)
	}
}

func TestSurveyStore_ReplaceEntriesForAnalyzer(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	// Add initial entries
	initial := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "old-a", Title: "Old A"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "old-b", Title: "Old B"},
		{Analyzer: survey.AnalyzerChurn, Kind: survey.KindChurn, Name: "keep", Title: "Keep this"},
	}
	for _, e := range initial {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	// Replace topology entries
	replacement := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "new-x", Title: "New X"},
	}
	if err := ss.ReplaceEntriesForAnalyzer(survey.AnalyzerTopology, replacement); err != nil {
		t.Fatalf("ReplaceEntriesForAnalyzer: %v", err)
	}

	// Verify: 1 topology + 1 churn = 2 total
	all, err := ss.ListEntries(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entries after replace, got %d", len(all))
	}

	// Verify the topology entry is the new one
	topEntries, err := ss.ListEntries(survey.SearchOptions{Analyzer: survey.AnalyzerTopology})
	if err != nil {
		t.Fatalf("ListEntries(topology): %v", err)
	}
	if len(topEntries) != 1 {
		t.Fatalf("expected 1 topology entry, got %d", len(topEntries))
	}
	if topEntries[0].Name != "new-x" {
		t.Errorf("expected Name %q, got %q", "new-x", topEntries[0].Name)
	}
}

func TestSurveyStore_ReplaceEntriesForAnalyzerAndFile(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	initial := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "a", Title: "A", FilePath: "pkg/store"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindTechStack, Name: "b", Title: "B", FilePath: "pkg/store"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "c", Title: "C", FilePath: "pkg/code"},
	}
	for _, e := range initial {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	// Replace only topology entries for pkg/store
	replacement := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "new-a", Title: "New A", FilePath: "pkg/store"},
	}
	if err := ss.ReplaceEntriesForAnalyzerAndFile(survey.AnalyzerTopology, "pkg/store", replacement); err != nil {
		t.Fatalf("ReplaceEntriesForAnalyzerAndFile: %v", err)
	}

	// pkg/code entry should remain, pkg/store should have just the new one
	all, err := ss.ListEntries(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entries after replace, got %d", len(all))
	}
}

func TestSurveyStore_Stats(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "a", Title: "A"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindTechStack, Name: "b", Title: "B"},
		{Analyzer: survey.AnalyzerEntrypoints, Kind: survey.KindEntrypoint, Name: "c", Title: "C"},
		{Analyzer: survey.AnalyzerChurn, Kind: survey.KindChurn, Name: "d", Title: "D"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	stats, err := ss.Stats(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Total != 4 {
		t.Errorf("expected Total 4, got %d", stats.Total)
	}
	if stats.ByAnalyzer[survey.AnalyzerTopology] != 2 {
		t.Errorf("expected ByAnalyzer[topology] 2, got %d", stats.ByAnalyzer[survey.AnalyzerTopology])
	}
	if stats.ByAnalyzer[survey.AnalyzerEntrypoints] != 1 {
		t.Errorf("expected ByAnalyzer[entrypoints] 1, got %d", stats.ByAnalyzer[survey.AnalyzerEntrypoints])
	}
	if stats.ByKind[survey.KindModule] != 1 {
		t.Errorf("expected ByKind[module] 1, got %d", stats.ByKind[survey.KindModule])
	}
	if stats.ByKind[survey.KindChurn] != 1 {
		t.Errorf("expected ByKind[churn] 1, got %d", stats.ByKind[survey.KindChurn])
	}
}

func TestSurveyStore_Clear(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		e := &survey.Entry{
			Analyzer: survey.AnalyzerTopology,
			Kind:     survey.KindModule,
			Name:     "mod",
			Title:    "Module",
		}
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	if err := ss.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	all, err := ss.ListEntries(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("ListEntries after clear: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(all))
	}

	stats, err := ss.Stats(survey.SearchOptions{})
	if err != nil {
		t.Fatalf("Stats after clear: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("expected Total 0 after clear, got %d", stats.Total)
	}
}

func TestSurveyStore_ListEntriesFilePath(t *testing.T) {
	ss, cleanup := setupTestSurveyStore(t)
	defer cleanup()

	entries := []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "store", Title: "Store", FilePath: "pkg/store/main.go"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "code", Title: "Code", FilePath: "pkg/code/index.go"},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "survey", Title: "Survey", FilePath: "pkg/survey/types.go"},
	}
	for _, e := range entries {
		if err := ss.AddEntry(e); err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
	}

	// Filter by file path substring
	got, err := ss.ListEntries(survey.SearchOptions{FilePath: "pkg/store"})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry for pkg/store, got %d", len(got))
	}
}
