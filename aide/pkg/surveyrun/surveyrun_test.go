package surveyrun

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

type unreadableAssignments struct {
	store.SurveyStore
	replaced bool
}

func (s *unreadableAssignments) ListEntries(survey.SearchOptions) ([]*survey.Entry, error) {
	return nil, errors.New("assignment read failed")
}

func (s *unreadableAssignments) ReplaceEntriesForAnalyzer(string, []*survey.Entry) error {
	s.replaced = true
	return nil
}

func TestModuleRunPreservesStoreWhenAssignmentsCannotBeRead(t *testing.T) {
	ss := &unreadableAssignments{}
	cs, err := store.NewCodeStore(filepath.Join(t.TempDir(), "code.db"), filepath.Join(t.TempDir(), "search"))
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	results := Run(t.TempDir(), []string{survey.AnalyzerModules}, ss, cs)
	if len(results) != 1 || !strings.Contains(results[0].Err, "assignment read failed") {
		t.Fatalf("expected prior assignment read failure, got %+v", results)
	}
	if ss.replaced {
		t.Fatal("replaced existing assignments after a failed read")
	}
}

func TestRunAllWithoutCodeIndex(t *testing.T) {
	root := t.TempDir()
	for rel, content := range map[string]string{
		"go.mod":  "module example.com/proj\n",
		"main.go": "package main\n\nfunc main() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	ss, err := store.NewSurveyStore(filepath.Join(t.TempDir(), "survey"))
	if err != nil {
		t.Fatalf("NewSurveyStore: %v", err)
	}
	defer ss.Close()

	results := Run(root, nil, ss, nil)
	if len(results) != len(AllAnalyzers()) {
		t.Fatalf("expected %d results, got %d", len(AllAnalyzers()), len(results))
	}

	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Analyzer] = r
	}

	if r := byName[survey.AnalyzerTopology]; r.Err != "" || r.Entries == 0 {
		t.Errorf("topology = %+v, want entries with no error", r)
	}
	// Not a git repo: churn degrades to zero entries without failing.
	if r := byName[survey.AnalyzerChurn]; r.Err != "" {
		t.Errorf("churn on non-git dir errored: %+v", r)
	}
	// No code index: modules reports the error instead of guessing.
	if r := byName[survey.AnalyzerModules]; r.Err == "" {
		t.Errorf("modules without code index should error, got %+v", r)
	}

	stored, err := ss.ListEntries(survey.SearchOptions{Analyzer: survey.AnalyzerTopology, Limit: 500})
	if err != nil || len(stored) == 0 {
		t.Fatalf("topology entries not stored: %v", err)
	}

	out := FormatResults(results)
	if !strings.Contains(out, "topology: ") || !strings.Contains(out, "modules: error:") {
		t.Errorf("FormatResults output unexpected:\n%s", out)
	}
}

func TestRunUnknownAnalyzer(t *testing.T) {
	ss, err := store.NewSurveyStore(filepath.Join(t.TempDir(), "survey"))
	if err != nil {
		t.Fatalf("NewSurveyStore: %v", err)
	}
	defer ss.Close()

	results := Run(t.TempDir(), []string{"nonsense"}, ss, nil)
	if len(results) != 1 || results[0].Err == "" {
		t.Errorf("unknown analyzer should produce an error result, got %+v", results)
	}
}
