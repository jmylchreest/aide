package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	bolt "go.etcd.io/bbolt"
)

func BenchmarkReplaceFindings(b *testing.B) {
	s, err := NewFindingsStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	items := make([]*findings.Finding, 100)
	for i := range items {
		items[i] = &findings.Finding{Analyzer: findings.AnalyzerComplexity, FilePath: fmt.Sprintf("file%d.go", i), Title: "Complex function", Severity: findings.SevWarning}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := s.ReplaceFindingsForAnalyzer(findings.AnalyzerComplexity, items); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFindingsStatsLarge(b *testing.B) {
	s, err := NewFindingsStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(BucketFindings)
		for i := 0; i < 40000; i++ {
			f := findings.Finding{ID: fmt.Sprint(i), Analyzer: "security", Severity: "warning", FilePath: "file.c", Detail: strings.Repeat("detail ", 150)}
			data, err := json.Marshal(&f)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(f.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		stats, err := s.Stats(findings.SearchOptions{})
		if err != nil || stats.Total != 40000 {
			b.Fatal(stats, err)
		}
	}
}

func BenchmarkFindingsSearchRebuild(b *testing.B) {
	s, err := NewFindingsStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	items := make([]*findings.Finding, 260)
	for i := range items {
		items[i] = &findings.Finding{Analyzer: findings.AnalyzerComplexity, FilePath: fmt.Sprintf("file%d.go", i), Title: "Complex function", Severity: findings.SevWarning}
	}
	if err := s.ReplaceFindingsForAnalyzer(findings.AnalyzerComplexity, items); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(BucketFindingsMeta).Delete([]byte("search_mapping_hash")) }); err != nil {
			b.Fatal(err)
		}
		if err := s.ensureSearchMapping(s.searchPath); err != nil {
			b.Fatal(err)
		}
	}
}
