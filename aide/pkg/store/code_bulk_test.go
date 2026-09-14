package store

import (
	"github.com/jmylchreest/aide/aide/pkg/code"
	"path/filepath"
	"testing"
	"time"
)

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
