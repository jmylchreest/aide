package store

import (
	"fmt"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
)

func TestReplaceFindingsBatchesPreserveSearchAndOtherAnalyzers(t *testing.T) {
	s, err := NewFindingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	makeItems := func(title string) []*findings.Finding {
		items := make([]*findings.Finding, 300)
		for i := range items {
			items[i] = &findings.Finding{Analyzer: findings.AnalyzerComplexity, FilePath: fmt.Sprintf("file%d.go", i), Title: title, Severity: findings.SevWarning}
		}
		return items
	}
	if err := s.AddFinding(&findings.Finding{Analyzer: findings.AnalyzerSecrets, FilePath: ".env", Title: "retained", Severity: findings.SevWarning}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"obsolete", "replacement"} {
		if err := s.ReplaceFindingsForAnalyzer(findings.AnalyzerComplexity, makeItems(title)); err != nil {
			t.Fatal(err)
		}
		items, err := s.SearchFindings(title, findings.SearchOptions{Limit: -1})
		if err != nil || len(items) != 300 {
			t.Fatalf("search %s: %d results, %v", title, len(items), err)
		}
	}
	old, err := s.SearchFindings("obsolete", findings.SearchOptions{Limit: -1})
	if err != nil || len(old) != 0 {
		t.Fatalf("obsolete search docs retained: %d, %v", len(old), err)
	}
	stats, err := s.Stats(findings.SearchOptions{})
	if err != nil || stats.Total != 301 || stats.ByAnalyzer[findings.AnalyzerSecrets] != 1 {
		t.Fatalf("replacement touched other analyzer: %+v, %v", stats, err)
	}
	if err := s.ReplaceFindingsForAnalyzer(findings.AnalyzerComplexity, nil); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchFindings("replacement", findings.SearchOptions{Limit: -1})
	if err != nil || len(items) != 0 {
		t.Fatalf("clear retained search docs: %d, %v", len(items), err)
	}
}
