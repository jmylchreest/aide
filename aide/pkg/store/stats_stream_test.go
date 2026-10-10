package store

import (
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	bolt "go.etcd.io/bbolt"
)

func TestStatsStreamResetsOmittedFields(t *testing.T) {
	s, err := NewFindingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		for i, data := range []string{`{"id":"1","analyzer":"security","severity":"critical","accepted":true}`, `{"id":"2","analyzer":"complexity","severity":"warning"}`, `{`, `{"id":"4","analyzer":"todos","severity":"info"}`} {
			if err := b.Put([]byte{byte(i + 1)}, []byte(data)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{false, true} {
		stats, err := s.Stats(findings.SearchOptions{IncludeAccepted: include, Limit: 1})
		want := 2
		if include {
			want = 3
		}
		if err != nil || stats.Total != want || stats.ByAnalyzer["complexity"] != 1 || stats.BySeverity["info"] != 1 {
			t.Fatalf("stats=%+v err=%v", stats, err)
		}
	}
}
