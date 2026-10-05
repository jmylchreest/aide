package store

import (
	"fmt"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
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
