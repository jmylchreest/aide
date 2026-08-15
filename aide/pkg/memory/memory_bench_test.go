package memory

import (
	"testing"
	"time"
)

// The fixture here is a single synthetic in-memory Memory; scoring is a pure
// function over it and the config, so nothing outside the benchmark is read
// or written.

var (
	benchScoreSink     float64
	benchBreakdownSink ScoreBreakdown
)

func benchScoringMemory(now time.Time) *Memory {
	return &Memory{
		ID:           "bench-memory",
		Category:     CategoryLearning,
		Content:      "synthetic memory used for scoring benchmarks",
		Tags:         []string{"testing", "project:bench", "source:discovered", "verified:true", "scope:global"},
		AccessCount:  4,
		CreatedAt:    now.AddDate(0, 0, -12),
		UpdatedAt:    now.AddDate(0, 0, -2),
		LastAccessed: now.AddDate(0, 0, -1),
	}
}

// BenchmarkScoreMemory measures memory scoring with the default config.
func BenchmarkScoreMemory(b *testing.B) {
	cfg := DefaultScoringConfig()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	m := benchScoringMemory(now)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchScoreSink = ScoreMemory(m, now, cfg)
	}
}

// BenchmarkScoreMemoryDetailed measures the breakdown-producing variant.
func BenchmarkScoreMemoryDetailed(b *testing.B) {
	cfg := DefaultScoringConfig()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	m := benchScoringMemory(now)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBreakdownSink = ScoreMemoryDetailed(m, now, cfg)
	}
}
