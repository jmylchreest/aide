package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/surveyrun"
)

type outcomeSink struct {
	mu     sync.Mutex
	events []*observe.Event
}

func (s *outcomeSink) Emit(e *observe.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

func (s *outcomeSink) last(t *testing.T, name string) *observe.Event {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		if s.events[i].Name == name {
			return s.events[i]
		}
	}
	t.Fatalf("no observation for %s", name)
	return nil
}

type outcomeIndexStore struct {
	store.CodeIndexStore
	listErr   error
	batchErr  error
	removeErr error
}

func (s outcomeIndexStore) ListAllFileInfo() ([]*code.FileInfo, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.CodeIndexStore.ListAllFileInfo()
}

func (s outcomeIndexStore) IndexFileBatch(path string, symbols []*code.Symbol, refs []*code.Reference, mtime time.Time, size int64) error {
	if s.batchErr != nil {
		return s.batchErr
	}
	return s.CodeIndexStore.IndexFileBatch(path, symbols, refs, mtime, size)
}

func (s outcomeIndexStore) ClearFile(path string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	return s.CodeIndexStore.ClearFile(path)
}

func outcomeIndexer(t *testing.T, failStore bool) (*Indexer, *outcomeSink) {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, ".aide", "memory", "store.db")
	indexPath, searchPath := testCodeStorePaths(t, dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	sink := &outcomeSink{}
	observe.SetDefault(sink)
	t.Cleanup(func() { observe.SetDefault(nil) })
	wrapper := outcomeIndexStore{CodeIndexStore: cs}
	if failStore {
		wrapper.batchErr = errors.New("storage refused batch")
	}
	return NewIndexerFromStore(wrapper, newGrammarLoader(dbPath, nil), root), sink
}

func TestWorkOutcomeIndexerParseFailure(t *testing.T) {
	sink := &outcomeSink{}
	observe.SetDefault(sink)
	t.Cleanup(func() { observe.SetDefault(nil) })
	root := t.TempDir()
	idx := NewIndexerFromStore(nil, newGrammarLoader(filepath.Join(root, ".aide", "memory", "store.db"), nil), root)
	_, err := idx.IndexFile(filepath.Join(root, "missing.go"))
	if err == nil {
		t.Fatal("expected read/parse failure")
	}
	e := sink.last(t, "Indexer.IndexFile")
	if e.Error == "" {
		t.Fatal("failed indexing was recorded without an error")
	}
	if _, ok := e.Attrs["stored_symbols"]; ok {
		t.Fatal("failed indexing claimed stored symbols")
	}
}

func TestWorkOutcomeReconcileFailure(t *testing.T) {
	sink := &outcomeSink{}
	observe.SetDefault(sink)
	t.Cleanup(func() { observe.SetDefault(nil) })
	idx := &Indexer{store: outcomeIndexStore{listErr: errors.New("index unavailable")}, rootDir: t.TempDir()}
	if _, err := idx.Reconcile(); err == nil {
		t.Fatal("expected reconciliation failure")
	}
	if sink.last(t, "Indexer.Reconcile").Error == "" {
		t.Fatal("failed reconciliation was recorded without an error")
	}
}

func TestWorkOutcomeSurveyFailure(t *testing.T) {
	s := newTestMCPServer(newMockSurveyStore())
	result, _, err := s.handleSurveyRun(context.Background(), nil, SurveyRunInput{Analyzer: "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	assertIsError(t, result, "unknown analyzer: bogus")
}

func TestWorkOutcomeIndexerCommittedCounts(t *testing.T) {
	for _, failStore := range []bool{false, true} {
		t.Run(strconv.FormatBool(failStore), func(t *testing.T) {
			idx, sink := outcomeIndexer(t, failStore)
			path := filepath.Join(idx.rootDir, "source.go")
			if err := os.WriteFile(path, []byte("package main\nfunc Answer() int { return 42 }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			count, err := idx.IndexFile(path)
			e := sink.last(t, "Indexer.IndexFile")
			if failStore {
				if err == nil || e.Error == "" {
					t.Fatal("storage failure not returned and observed")
				}
				if _, ok := e.Attrs["stored_symbols"]; ok {
					t.Fatal("failed write claimed committed symbols")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if count < 1 || e.Error != "" || e.Attrs["stored_symbols"] != strconv.Itoa(count) {
				t.Fatalf("incorrect committed work evidence: %+v", e)
			}
			refs, err := idx.store.GetFileReferences("source.go")
			if err != nil {
				t.Fatal(err)
			}
			if e.Attrs["stored_references"] != strconv.Itoa(len(refs)) {
				t.Fatal("reference count disagrees with persisted result")
			}
		})
	}
}

func TestWorkOutcomeReconcilePartialFailure(t *testing.T) {
	idx, sink := outcomeIndexer(t, false)
	if err := idx.store.IndexFileBatch("gone.go", nil, nil, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	idx.store = outcomeIndexStore{CodeIndexStore: idx.store, removeErr: errors.New("cannot remove stale entry")}
	res, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("partial failures should retain existing return contract: %v", err)
	}
	e := sink.last(t, "Indexer.Reconcile")
	if res.Errors == 0 || e.Error == "" || e.Attrs["errors"] != strconv.Itoa(res.Errors) {
		t.Fatalf("partial failure hidden: res=%+v event=%+v", res, e)
	}
}

func TestWorkOutcomeSurveyPartialAndComplete(t *testing.T) {
	for _, fail := range []bool{false, true} {
		results := []surveyrun.Result{{Analyzer: "topology", Entries: 2, Summary: "2 entries"}}
		if fail {
			results = append(results, surveyrun.Result{Analyzer: "modules", Err: "index unavailable"})
		}
		result := surveyRunResult(results)
		if result.IsError != fail {
			t.Fatalf("partial status hidden: %+v", result)
		}
		assertTextContains(t, result, "topology: 2 entries")
		if fail {
			assertTextContains(t, result, "modules: error: index unavailable")
		}
	}
}
