package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
)

// All fixtures in this file are synthetic and constructed in-memory. Every
// CodeStore lives in a fresh b.TempDir() (bbolt DB + bleve index both inside
// it), so no real code index is ever opened.

// setupBenchCodeStore mirrors setupTestCodeStore from code_test.go, building
// an isolated CodeStore rooted in b.TempDir().
func setupBenchCodeStore(b *testing.B) *CodeStore {
	b.Helper()

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")
	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		b.Fatalf("failed to create code store: %v", err)
	}
	b.Cleanup(func() {
		if err := cs.Close(); err != nil {
			b.Errorf("failed to close code store: %v", err)
		}
	})
	return cs
}

const (
	benchSymbolsPerFile = 20
	benchRefsPerFile    = 10
)

// benchFileBatch fabricates one file's worth of symbols and references.
// File paths are unique per index so repeated calls exercise the append path.
func benchFileBatch(fileIdx int) (string, []*code.Symbol, []*code.Reference) {
	filePath := fmt.Sprintf("pkg/bench/mod%03d/file%03d.go", fileIdx/100, fileIdx%100)

	symbols := make([]*code.Symbol, benchSymbolsPerFile)
	for i := range symbols {
		symbols[i] = &code.Symbol{
			Name:       fmt.Sprintf("fn%d_%d", fileIdx, i),
			Kind:       code.KindFunction,
			Signature:  fmt.Sprintf("func fn%d_%d(id int) (string, error)", fileIdx, i),
			DocComment: "synthetic benchmark symbol",
			FilePath:   filePath,
			StartLine:  i*10 + 1,
			EndLine:    i*10 + 9,
			Language:   "go",
		}
	}

	refs := make([]*code.Reference, benchRefsPerFile)
	for j := range refs {
		refs[j] = &code.Reference{
			SymbolName: fmt.Sprintf("fn%d_%d", fileIdx, (j+1)%benchSymbolsPerFile),
			Kind:       "call",
			FilePath:   filePath,
			Line:       j*10 + 5,
			Column:     4,
			Context:    "synthetic call site",
			Language:   "go",
		}
	}
	return filePath, symbols, refs
}

// benchIndexFiles pre-indexes n files (each via IndexFileBatch) and returns
// their file paths.
func benchIndexFiles(b *testing.B, cs *CodeStore, n int) []string {
	b.Helper()
	paths := make([]string, n)
	mtime := time.Now()
	for i := 0; i < n; i++ {
		filePath, symbols, refs := benchFileBatch(i)
		if err := cs.IndexFileBatch(filePath, symbols, refs, mtime, 2048); err != nil {
			b.Fatalf("preload IndexFileBatch %d: %v", i, err)
		}
		paths[i] = filePath
	}
	return paths
}

// Sinks keep the compiler from optimising away retrieval results.
var (
	benchSymbolSink     []*code.Symbol
	benchCodeSearchSink []*CodeSearchResult
	benchRefSink        []*code.Reference
	benchBoolSink       bool
)

// BenchmarkCodeIndexFileBatch measures the "commit one indexed file" path:
// all symbols, references and FileInfo for a file in a single bbolt
// transaction plus one bleve batch.
func BenchmarkCodeIndexFileBatch(b *testing.B) {
	cs := setupBenchCodeStore(b)
	mtime := time.Now()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filePath, symbols, refs := benchFileBatch(i)
		if err := cs.IndexFileBatch(filePath, symbols, refs, mtime, 2048); err != nil {
			b.Fatalf("IndexFileBatch: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(benchSymbolsPerFile), "symbols/op-file")
	b.ReportMetric(float64(benchRefsPerFile), "refs/op-file")
}

// BenchmarkAddSymbol measures the ad-hoc single-symbol write path (own bbolt
// transaction + own bleve index call per symbol). A fixed pool of symbols is
// rotated so each op rewrites an existing record (AddSymbol reuses a non-empty
// ID), keeping the index at a stable size and measuring steady-state commit
// cost instead of "add to an ever-growing index".
func BenchmarkAddSymbol(b *testing.B) {
	cs := setupBenchCodeStore(b)

	const addPool = 512
	pool := make([]*code.Symbol, addPool)
	for i := range pool {
		filePath, _, _ := benchFileBatch(i / benchSymbolsPerFile)
		pool[i] = &code.Symbol{
			ID:        fmt.Sprintf("bench-addr-%d", i),
			Name:      fmt.Sprintf("solo%d", i),
			Kind:      code.KindFunction,
			Signature: fmt.Sprintf("func solo%d() error", i),
			FilePath:  filePath,
			StartLine: 1,
			EndLine:   9,
			Language:  "go",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cs.AddSymbol(pool[i%addPool]); err != nil {
			b.Fatalf("AddSymbol: %v", err)
		}
	}
}

// BenchmarkSearchSymbols pre-indexes a corpus of a few thousand symbols,
// then measures bleve symbol search and reference lookup latency.
func BenchmarkSearchSymbols(b *testing.B) {
	cs := setupBenchCodeStore(b)

	// 150 files x 20 symbols = 3000 symbols, 1500 references.
	benchIndexFiles(b, cs, 150)

	b.Run("SearchSymbols", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, err := cs.SearchSymbols("fn42", code.SearchOptions{Limit: 20})
			if err != nil {
				b.Fatalf("SearchSymbols: %v", err)
			}
			benchCodeSearchSink = got
		}
	})

	b.Run("SearchReferences", func(b *testing.B) {
		opts := code.ReferenceSearchOptions{SymbolName: "fn42_3", Kind: "call", Limit: 20}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, err := cs.SearchReferences(opts)
			if err != nil {
				b.Fatalf("SearchReferences: %v", err)
			}
			benchRefSink = got
		}
	})
}

// BenchmarkGetFileSymbols measures retrieving all symbols of an indexed file
// via the file-keyed secondary index.
func BenchmarkGetFileSymbols(b *testing.B) {
	cs := setupBenchCodeStore(b)
	paths := benchIndexFiles(b, cs, 64)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := cs.GetFileSymbols(paths[i%len(paths)])
		if err != nil {
			b.Fatalf("GetFileSymbols: %v", err)
		}
		benchSymbolSink = got
	}
}

// BenchmarkClearFile measures the delete path: clearing a file's symbols,
// references and FileInfo. A pool of pre-indexed files is cleared one at a
// time so each op does real delete work; with the 200ms benchtime used for
// measurement, b.N stays well below the pool size.
func BenchmarkClearFile(b *testing.B) {
	cs := setupBenchCodeStore(b)

	const pool = 1024
	paths := benchIndexFiles(b, cs, pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cs.ClearFile(paths[i%pool]); err != nil {
			b.Fatalf("ClearFile: %v", err)
		}
	}
}
