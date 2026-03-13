package findings

import (
	"sync"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

// mockReplaceFindingsStore records calls to the ReplaceFindingsStore interface
// so tests can verify the runner's behavior without a real database.
type mockReplaceFindingsStore struct {
	mu                      sync.Mutex
	replacedAnalyzerAndFile []replaceCall
	replacedAnalyzer        []replaceAnalyzerCall
}

type replaceCall struct {
	Analyzer string
	FilePath string
	Findings []*Finding
}

type replaceAnalyzerCall struct {
	Analyzer string
	Findings []*Finding
}

func (m *mockReplaceFindingsStore) ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, findings []*Finding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replacedAnalyzerAndFile = append(m.replacedAnalyzerAndFile, replaceCall{
		Analyzer: analyzer,
		FilePath: filePath,
		Findings: findings,
	})
	return nil
}

func (m *mockReplaceFindingsStore) ReplaceFindingsForAnalyzer(analyzer string, findings []*Finding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replacedAnalyzer = append(m.replacedAnalyzer, replaceAnalyzerCall{
		Analyzer: analyzer,
		Findings: findings,
	})
	return nil
}

func (m *mockReplaceFindingsStore) Stats(_ SearchOptions) (*Stats, error) {
	return &Stats{}, nil
}

func TestOnChanges_RemoveDeletesFindings(t *testing.T) {
	store := &mockReplaceFindingsStore{}

	runner := NewRunner(store, AnalyzerConfig{}, nil)
	defer runner.Stop()

	absPath := "/project/src/deleted.go"
	expectedPath := toRelPath("", absPath)

	// Simulate a Remove event for a Go file.
	runner.OnChanges(map[string]fsnotify.Op{
		absPath: fsnotify.Remove,
	})

	// The runner should have cleared findings for each per-file analyzer
	// (complexity and secrets) by calling ReplaceFindingsForAnalyzerAndFile
	// with nil findings. The path should be normalised to cwd-relative.
	store.mu.Lock()
	calls := store.replacedAnalyzerAndFile
	store.mu.Unlock()

	if len(calls) != 3 {
		t.Fatalf("expected 3 ReplaceFindingsForAnalyzerAndFile calls (complexity + secrets + security), got %d", len(calls))
	}

	analyzers := map[string]bool{}
	for _, c := range calls {
		analyzers[c.Analyzer] = true
		if c.FilePath != expectedPath {
			t.Errorf("expected file path %q, got %q", expectedPath, c.FilePath)
		}
		if c.Findings != nil {
			t.Errorf("expected nil findings for deletion, got %v", c.Findings)
		}
	}

	if !analyzers[AnalyzerComplexity] {
		t.Error("expected complexity analyzer findings to be cleared")
	}
	if !analyzers[AnalyzerSecrets] {
		t.Error("expected secrets analyzer findings to be cleared")
	}
	if !analyzers[AnalyzerSecurity] {
		t.Error("expected security analyzer findings to be cleared")
	}
}

func TestOnChanges_RemoveDoesNotRunAnalyzers(t *testing.T) {
	store := &mockReplaceFindingsStore{}

	runner := NewRunner(store, AnalyzerConfig{}, nil)
	defer runner.Stop()

	// Send a Remove event. The runner should NOT attempt to read and
	// analyse the file (which would fail since it doesn't exist).
	runner.OnChanges(map[string]fsnotify.Op{
		"/project/src/gone.go": fsnotify.Remove,
	})

	// Wait for any async work to drain.
	runner.Stop()

	// Only the synchronous clear calls should have occurred (no analyzer runs).
	store.mu.Lock()
	analyzerCalls := store.replacedAnalyzerAndFile
	store.mu.Unlock()

	for _, c := range analyzerCalls {
		if c.Findings != nil {
			t.Errorf("analyzer %s produced findings for a deleted file; expected nil", c.Analyzer)
		}
	}
}

func TestOnChanges_NonRemoveStillAnalyzes(t *testing.T) {
	store := &mockReplaceFindingsStore{}

	dir := testdataDir(t)
	// Use an empty ignore matcher so the testdata/ path is not filtered out.
	runner := NewRunner(store, AnalyzerConfig{
		Ignore: aideignore.NewEmpty(),
	}, nil)
	defer runner.Stop()

	// Send a Write event for an existing testdata file (absolute path,
	// as the watcher would provide).
	runner.OnChanges(map[string]fsnotify.Op{
		dir + "/complex_high.go": fsnotify.Write,
	})

	// Wait for async analyzers to complete (WaitAll does not cancel them,
	// unlike Stop).
	runner.WaitAll()

	// Per-file analyzers should have run and stored results (even if 0
	// findings). The calls use non-nil slices (possibly empty), unlike
	// the nil used for deletions.
	store.mu.Lock()
	calls := store.replacedAnalyzerAndFile
	store.mu.Unlock()

	if len(calls) < 2 {
		t.Fatalf("expected at least 2 ReplaceFindingsForAnalyzerAndFile calls for a Write event, got %d", len(calls))
	}
}

func TestOnChanges_PathNormalization(t *testing.T) {
	store := &mockReplaceFindingsStore{}

	dir := testdataDir(t)
	runner := NewRunner(store, AnalyzerConfig{
		Ignore: aideignore.NewEmpty(),
	}, nil)
	defer runner.Stop()

	absPath := dir + "/complex_high.go"
	expectedRelPath := toRelPath("", absPath)

	// Send a Write event with an absolute path.
	runner.OnChanges(map[string]fsnotify.Op{
		absPath: fsnotify.Write,
	})
	runner.WaitAll()

	store.mu.Lock()
	calls := store.replacedAnalyzerAndFile
	store.mu.Unlock()

	// Verify that the store replacement call used the cwd-relative path
	// (matching what findings store uses internally), not the absolute path.
	for _, c := range calls {
		if c.FilePath != expectedRelPath {
			t.Errorf("expected relative path %q for store call, got %q", expectedRelPath, c.FilePath)
		}
	}

	// Also verify that the findings themselves use the same relative path.
	for _, c := range calls {
		for _, f := range c.Findings {
			if f.FilePath != expectedRelPath {
				t.Errorf("expected relative path %q in finding.FilePath, got %q", expectedRelPath, f.FilePath)
			}
		}
	}
}

func TestOnChanges_UnsupportedFileIgnored(t *testing.T) {
	store := &mockReplaceFindingsStore{}

	runner := NewRunner(store, AnalyzerConfig{}, nil)
	defer runner.Stop()

	// A file with an unsupported extension should be skipped entirely.
	runner.OnChanges(map[string]fsnotify.Op{
		"/project/image.png": fsnotify.Remove,
	})

	store.mu.Lock()
	calls := store.replacedAnalyzerAndFile
	store.mu.Unlock()

	if len(calls) != 0 {
		t.Errorf("expected 0 calls for unsupported file, got %d", len(calls))
	}
}
