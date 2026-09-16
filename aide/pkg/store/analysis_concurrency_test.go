package store

import (
	"sync"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
)

// Findings and survey share the same searchableStore implementation.
func TestAnalysisConcurrentClearReplaceSearch(t *testing.T) {
	s, err := NewFindingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				var err error
				switch worker {
				case 0:
					err = s.ReplaceFindingsForAnalyzer("test", []*findings.Finding{{Analyzer: "test", Title: "Current", FilePath: "same.go"}})
				case 1:
					err = s.Clear()
				case 2:
					_, err = s.SearchFindings("Current", findings.SearchOptions{})
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
