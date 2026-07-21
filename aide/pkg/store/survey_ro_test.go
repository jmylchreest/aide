package store

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// TestReadSurveyEntriesRO pins the daemon-less read path: entries written
// through the full store are readable from the bolt file alone, filtered
// by kind, without the bleve index open.
func TestReadSurveyEntriesRO(t *testing.T) {
	dir := t.TempDir()
	ss, err := NewSurveyStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, e := range []*survey.Entry{
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindSubproject, Name: "child-a", FilePath: "child-a", CreatedAt: now},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindSubproject, Name: "child-b", FilePath: "nested/child-b", CreatedAt: now},
		{Analyzer: survey.AnalyzerTopology, Kind: survey.KindModule, Name: "pkg/store", CreatedAt: now},
	} {
		if err := ss.AddEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := ss.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSurveyEntriesRO(dir, survey.KindSubproject, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d subproject entries, want 2", len(got))
	}
	names := map[string]bool{}
	for _, e := range got {
		if e.Kind != survey.KindSubproject {
			t.Errorf("kind filter leaked: %+v", e)
		}
		names[e.Name] = true
	}
	if !names["child-a"] || !names["child-b"] {
		t.Errorf("missing expected subprojects, got %v", names)
	}

	if capped, err := ReadSurveyEntriesRO(dir, "", 1); err != nil || len(capped) != 1 {
		t.Errorf("limit=1 read = %d entries, err %v; want 1, nil", len(capped), err)
	}
}

// TestReadSurveyEntriesROMissing: a directory with no survey.db errors
// rather than fabricating an empty result.
func TestReadSurveyEntriesROMissing(t *testing.T) {
	if _, err := ReadSurveyEntriesRO(t.TempDir(), "", 0); err == nil {
		t.Error("expected error for missing survey.db")
	}
}
