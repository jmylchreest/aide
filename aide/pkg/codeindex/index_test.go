package codeindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func openStore(t testing.TB) *store.CodeStore {
	t.Helper()
	dir := t.TempDir()
	s, e := store.NewCodeStore(filepath.Join(dir, "index.db"), filepath.Join(dir, "search.bleve"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func put(t testing.TB, root, name, body string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestSeedAndExternalBranchSwitch(t *testing.T) {
	root := t.TempDir()
	targetRoot := t.TempDir()
	source, target := openStore(t), openStore(t)
	parser := code.NewParser(grammar.NewCompositeLoader())
	defer parser.Close()
	for _, r := range []string{root, targetRoot} {
		put(t, r, "same.go", "package p\nfunc Original(){ Original() }\n")
		put(t, r, "deleted.go", "package p\nfunc Deleted(){}\n")
	}
	run := func(cs *store.CodeStore, r string, seed Seed) Result {
		t.Helper()
		res, e := Run(context.Background(), cs, parser, r, nil, false, seed, nil)
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	if res := run(source, root, nil); res.Indexed != 2 {
		t.Fatal(res)
	}
	os.Remove(filepath.Join(targetRoot, "deleted.go"))
	if res := run(target, targetRoot, source.SeedFile); res.Seeded != 1 || res.Indexed != 1 {
		t.Fatal(res)
	}
	// External Git operations can replace bytes while retaining the timestamp.
	stat, _ := os.Stat(filepath.Join(targetRoot, "same.go"))
	put(t, targetRoot, "same.go", "package p\nfunc Modified(){ Modified() }\n")
	os.Chtimes(filepath.Join(targetRoot, "same.go"), stat.ModTime(), stat.ModTime())
	if res := run(target, targetRoot, source.SeedFile); res.Indexed != 1 || res.Seeded != 0 {
		t.Fatal(res)
	}
	got, _ := target.SearchSymbols("Original", code.SearchOptions{})
	if len(got) != 0 {
		t.Fatal("stale symbols")
	}
	refs, _ := target.SearchReferences(code.ReferenceSearchOptions{SymbolName: "Original"})
	if len(refs) != 0 {
		t.Fatal("stale references")
	}
	got, _ = source.SearchSymbols("Original", code.SearchOptions{})
	if len(got) != 1 {
		t.Fatal("source changed")
	}
	if res := run(target, targetRoot, source.SeedFile); res.Skipped != 1 {
		t.Fatal(res)
	}
	os.Remove(filepath.Join(targetRoot, "same.go"))
	if res := run(target, targetRoot, nil); res.Removed != 1 {
		t.Fatal(res)
	}
}

func TestRunRejectsEscapingPath(t *testing.T) {
	parser := code.NewParser(grammar.NewCompositeLoader())
	defer parser.Close()
	if _, err := Run(context.Background(), openStore(t), parser, t.TempDir(), []string{t.TempDir()}, false, nil, nil); err == nil {
		t.Fatal("accepted foreign source")
	}
}

func BenchmarkCheckoutCreate(b *testing.B) {
	root := b.TempDir()
	targetRoot := b.TempDir()
	for i := 0; i < 128; i++ {
		name := fmt.Sprintf("file%03d.go", i)
		body := fmt.Sprintf("package p\nfunc Function%d(){ Function%d() }\n", i, i)
		put(b, root, name, body)
		if i < 12 {
			body += fmt.Sprintf("func Added%d(){}\n", i)
		}
		put(b, targetRoot, name, body)
	}
	parser := code.NewParser(grammar.NewCompositeLoader())
	defer parser.Close()
	source := openStore(b)
	if _, e := Run(context.Background(), source, parser, root, nil, false, nil, nil); e != nil {
		b.Fatal(e)
	}
	for _, seeded := range []bool{false, true} {
		b.Run(fmt.Sprintf("seeded=%v", seeded), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				dir := b.TempDir()
				var seed Seed
				if seeded {
					seed = source.SeedFile
				}
				b.StartTimer()
				target, e := store.NewCodeStore(filepath.Join(dir, "index.db"), filepath.Join(dir, "search.bleve"))
				if e != nil {
					b.Fatal(e)
				}
				res, e := Run(context.Background(), target, parser, targetRoot, nil, false, seed, nil)
				if e != nil {
					b.Fatal(e)
				}
				b.StopTimer()
				target.Close()
				b.ReportMetric(float64(res.Seeded), "seeded_files/op")
				b.StartTimer()
			}
		})
	}
}
