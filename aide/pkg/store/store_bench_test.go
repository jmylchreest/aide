package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// All fixtures in this file are synthetic and constructed in-memory. Every
// store lives in a fresh b.TempDir() (bbolt DB and bleve index paths are
// derived from it by NewCombinedStore), so no real aide data store is ever
// opened.

// setupBenchCombinedStore mirrors setupTestCombinedStore from
// combined_test.go, building an isolated CombinedStore rooted in b.TempDir().
func setupBenchCombinedStore(b *testing.B) *CombinedStore {
	b.Helper()

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	cs, err := NewCombinedStore(dbPath)
	if err != nil {
		b.Fatalf("failed to create combined store: %v", err)
	}
	b.Cleanup(func() {
		if err := cs.Close(); err != nil {
			b.Errorf("failed to close combined store: %v", err)
		}
	})
	return cs
}

// benchMemory fabricates a synthetic memory entry with unique content.
func benchMemory(i int) *memory.Memory {
	now := time.Now()
	return &memory.Memory{
		Category:  memory.CategoryLearning,
		Content:   fmt.Sprintf("synthetic memory %d covers auth middleware and database pooling", i),
		Tags:      []string{"bench", fmt.Sprintf("topic-%d", i%16)},
		Priority:  1.0,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// benchPreloadMemories inserts n synthetic memories into the store.
func benchPreloadMemories(b *testing.B, cs *CombinedStore, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		if err := cs.AddMemory(benchMemory(i)); err != nil {
			b.Fatalf("preload memory %d: %v", i, err)
		}
	}
}

// Sinks keep the compiler from optimising away retrieval results.
var (
	benchMemorySink []*memory.Memory
	benchSearchSink []SearchResult
)

// BenchmarkBoltStoreAddMemories measures the per-op commit cost of the
// dual-write memory add path (bbolt transaction + bleve index update). A
// fixed pool of memories is rotated so each op rewrites an existing record
// (AddMemory reuses a non-empty ID), keeping the index at a stable size and
// measuring steady-state commit cost instead of "add to an ever-growing
// index".
func BenchmarkBoltStoreAddMemories(b *testing.B) {
	cs := setupBenchCombinedStore(b)

	const memPool = 256
	pool := make([]*memory.Memory, memPool)
	for i := range pool {
		m := benchMemory(i)
		m.ID = fmt.Sprintf("bench-mem-%d", i)
		pool[i] = m
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cs.AddMemory(pool[i%memPool]); err != nil {
			b.Fatalf("AddMemory: %v", err)
		}
	}
}

// BenchmarkStoreSearchMemories pre-loads a few hundred memories then
// measures retrieval latency for a realistic multi-word query.
func BenchmarkStoreSearchMemories(b *testing.B) {
	cs := setupBenchCombinedStore(b)
	benchPreloadMemories(b, cs, 400)

	b.Run("SearchMemories", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, err := cs.SearchMemories("auth middleware database", 10)
			if err != nil {
				b.Fatalf("SearchMemories: %v", err)
			}
			benchMemorySink = got
		}
	})

	b.Run("SearchMemoriesWithScore", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, err := cs.SearchMemoriesWithScore("auth middleware database", 10, nil)
			if err != nil {
				b.Fatalf("SearchMemoriesWithScore: %v", err)
			}
			benchSearchSink = got
		}
	})
}

// BenchmarkStoreListMemories measures ListMemories with a category filter
// over a few hundred pre-loaded memories.
func BenchmarkStoreListMemories(b *testing.B) {
	cs := setupBenchCombinedStore(b)
	benchPreloadMemories(b, cs, 400)

	opts := memory.SearchOptions{
		Category: memory.CategoryLearning,
		Limit:    50,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := cs.ListMemories(opts)
		if err != nil {
			b.Fatalf("ListMemories: %v", err)
		}
		benchMemorySink = got
	}
}
