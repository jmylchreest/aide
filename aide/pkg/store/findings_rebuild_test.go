package store

import (
	"fmt"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	bolt "go.etcd.io/bbolt"
)

func TestFindingsRebuildAcrossBatchBoundaries(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFindingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]*findings.Finding, 260)
	for i := range items {
		items[i] = &findings.Finding{ID: fmt.Sprint(i), Analyzer: "complexity", Title: "rebuildneedle", FilePath: fmt.Sprintf("file%d.go", i)}
	}
	if err := s.ReplaceFindingsForAnalyzer("complexity", items); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptFindings([]string{"0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(BucketFindings).Put([]byte("malformed"), []byte("{")); err != nil {
			return err
		}
		return tx.Bucket(BucketFindingsMeta).Put([]byte("search_mapping_hash"), []byte("old"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewFindingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, accepted := range []bool{false, true} {
		hits, err := s.SearchFindings("rebuildneedle", findings.SearchOptions{Limit: 300, IncludeAccepted: accepted})
		want := 259
		if accepted {
			want = 260
		}
		if err != nil || len(hits) != want {
			t.Fatalf("rebuild accepted=%v got=%d want=%d err=%v", accepted, len(hits), want, err)
		}
	}
	if n, err := s.ClearAnalyzer("complexity"); err != nil || n != 260 {
		t.Fatalf("clear=%d err=%v", n, err)
	}
	hits, err := s.SearchFindings("rebuildneedle", findings.SearchOptions{Limit: 300, IncludeAccepted: true})
	if err != nil || len(hits) != 0 {
		t.Fatalf("stale search results after clear: %d %v", len(hits), err)
	}
}
