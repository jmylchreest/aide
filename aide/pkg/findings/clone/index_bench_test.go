package clone

import (
	"fmt"
	"testing"
)

func BenchmarkCloneIndexSingletons(b *testing.B) {
	index := NewCloneIndex()
	hashes := make([]HashEntry, 10000)
	for i := range hashes {
		hashes[i] = HashEntry{Hash: uint64(i), TokenIdx: i, StartLine: i, EndLine: i + 1}
	}
	index.AddFile("unique.go", hashes, "go", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		index.ClonePairs(50, true)
	}
}

func BenchmarkCloneIndexBoilerplate(b *testing.B) {
	hashes := make([]HashEntry, 10000)
	for i := range hashes {
		hashes[i] = HashEntry{Hash: uint64(i % 10), TokenIdx: i, StartLine: i, EndLine: i + 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		index := NewCloneIndex()
		index.MaxBucketSize = 100
		for i := 0; i < 10; i++ {
			index.AddFile(fmt.Sprintf("file%d.go", i), hashes, "go", nil)
		}
		index.ClonePairs(50, true)
	}
}
