package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func BenchmarkBoltMemoryPage(b *testing.B) {
	for _, count := range []int{400, 4000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s, err := NewBoltStore(filepath.Join(b.TempDir(), "memory.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { s.Close() })
			at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			for i := 0; i < count; i++ {
				if err := s.AddMemory(&memory.Memory{ID: fmt.Sprintf("mem-%08d", i), Content: "synthetic memory about authentication and database pooling", Category: memory.CategoryLearning, Tags: []string{"bench"}, CreatedAt: at, UpdatedAt: at}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				rows, err := s.ListMemories(memory.SearchOptions{Category: memory.CategoryLearning, Limit: 50})
				if err != nil || len(rows) != 50 {
					b.Fatal(err)
				}
			}
		})
	}
}
