package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	bolt "go.etcd.io/bbolt"
)

func TestCodeRebuildsInterruptedSearchCommit(t *testing.T) {
	d := t.TempDir()
	open := func() *CodeStore {
		t.Helper()
		s, err := NewCodeStore(filepath.Join(d, "index.db"), filepath.Join(d, "search.bleve"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	// Simulate process death after Bolt commits but before Bleve receives the batch.
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(BucketCodeMeta).Put([]byte("search_dirty"), []byte{1}); err != nil {
			return err
		}
		return s.addSymbolTx(tx, &code.Symbol{Name: "Recovered", Kind: "function", FilePath: "same.go"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = open()
	defer s.Close()
	got, err := s.SearchSymbols("Recovered", code.SearchOptions{})
	if err != nil || len(got) != 1 {
		t.Fatalf("lost committed symbol: %v %v", got, err)
	}
}

func TestCodeConcurrentIndexClearAndSearch(t *testing.T) {
	d := t.TempDir()
	s, err := NewCodeStore(filepath.Join(d, "index.db"), filepath.Join(d, "search.bleve"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				var err error
				switch worker {
				case 0:
					err = s.IndexFiles([]code.FileBatch{{Path: "same.go", Symbols: []*code.Symbol{{Name: "Current", Kind: "function"}}}})
				case 1:
					err = s.Clear()
				case 2:
					_, err = s.SearchSymbols("Current", code.SearchOptions{})
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := s.IndexFiles([]code.FileBatch{{Path: "same.go", Symbols: []*code.Symbol{{Name: "Final", Kind: "function"}}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.SearchSymbols("Final", code.SearchOptions{})
	if err != nil || len(got) != 1 {
		t.Fatalf("final search: %v %v", got, err)
	}
}

func TestCodeBulkSeedVerification(t *testing.T) {
	open := func(name string) *CodeStore {
		t.Helper()
		d := filepath.Join(t.TempDir(), name)
		s, e := NewCodeStore(filepath.Join(d, "index.db"), filepath.Join(d, "search.bleve"))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	source, target := open("source"), open("target")
	f := code.FileBatch{Path: "same.go", ContentHash: "bytes-v1", ParserFingerprint: "parser-v1", ModTime: time.Now(), Symbols: []*code.Symbol{{Name: "Original", Kind: "function"}}, References: []*code.Reference{{SymbolName: "Original", Kind: "call"}}}
	if err := source.IndexFiles([]code.FileBatch{f}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ path, hash, parser string }{{"same.go", "bytes-v2", "parser-v1"}, {"same.go", "bytes-v1", "parser-v2"}, {"renamed.go", "bytes-v1", "parser-v1"}} {
		if _, err := source.SeedFile(want.path, want.hash, want.parser); err == nil {
			t.Fatalf("unverified seed: %+v", want)
		}
	}
	seed, err := source.SeedFile(f.Path, f.ContentHash, f.ParserFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.IndexFiles([]code.FileBatch{seed}); err != nil {
		t.Fatal(err)
	}
	old, err := source.GetFileSymbols(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := target.GetFileSymbols(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if old[0].ID == copied[0].ID {
		t.Fatal("seed retained source record identity")
	}
	f.Symbols = []*code.Symbol{{Name: "Changed", Kind: "function"}}
	f.ContentHash = "bytes-v2"
	f.References = nil
	if err := target.IndexFiles([]code.FileBatch{f}); err != nil {
		t.Fatal(err)
	}
	old, _ = source.GetFileSymbols(f.Path)
	if old[0].Name != "Original" {
		t.Fatal("source mutated")
	}
	refs, _ := target.GetFileReferences(f.Path)
	if len(refs) != 0 {
		t.Fatal("stale references survived replacement")
	}
	matches, _ := target.SearchSymbols("Original", code.SearchOptions{})
	if len(matches) != 0 {
		t.Fatal("stale search document survived replacement")
	}
}
