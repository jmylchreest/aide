// Package memory provides the core data types for aide.
// This file implements deterministic memory priority scoring for session injection.
package memory

import (
	"math"
	"strings"
	"time"
)

// ScoringConfig holds all tunable parameters for memory scoring.
// All weights, base scores, and decay parameters are deterministic —
// given the same config and memory, the score is always identical.
type ScoringConfig struct {
	// Component weights (should sum to 1.0).
	WeightCategory   float64
	WeightProvenance float64
	WeightRecency    float64
	WeightAccess     float64

	// CategoryScores maps each category to a base importance score [0.0, 1.0].
	// Categories not in the map fall back to DefaultCategoryScore.
	CategoryScores map[Category]float64

	// DefaultCategoryScore is the fallback for categories not in CategoryScores.
	DefaultCategoryScore float64

	// GlobalLearningScore overrides CategoryScores for learning memories
	// that have the scope:global tag (user preferences).
	GlobalLearningScore float64

	// ProvenanceBoosts maps provenance tag prefixes to score boosts.
	// Tags are matched by exact string (e.g., "source:user", "verified:true").
	// Multiple matching tags are additive.
	ProvenanceBoosts map[string]float64

	// RecencyHalfLifeDays is the number of days after which the recency
	// factor decays to 0.5. Exponential decay: factor = 0.5^(age/halfLife).
	RecencyHalfLifeDays float64

	// AccessLogBase controls the log scale for access count scoring.
	// factor = min(1.0, log(accessCount+1) / log(base)).
	AccessLogBase float64

	// ScoringDisabled is the master kill-switch. When true, ScoreMemory
	// is not called and session init uses chronological ULID order.
	// Set via AIDE_MEMORY_SCORING_DISABLED=1.
	ScoringDisabled bool

	// DecayDisabled disables time-based recency decay only.
	// Scoring still runs but recency factor is always 1.0.
	// Set via AIDE_MEMORY_DECAY_DISABLED=1.
	DecayDisabled bool
}

// DefaultScoringConfig returns the default scoring configuration.
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		WeightCategory:   0.50,
		WeightProvenance: 0.15,
		WeightRecency:    0.25,
		WeightAccess:     0.10,

		CategoryScores: map[Category]float64{
			CategoryAbandoned: 0.90,
			CategoryBlocker:   0.85,
			CategoryIssue:     0.80,
			CategoryDiscovery: 0.70,
			CategoryDecision:  0.65,
			CategoryLearning:  0.60,
			"session":         0.40,
			"pattern":         0.60,
			"gotcha":          0.75,
		},
		DefaultCategoryScore: 0.50,
		GlobalLearningScore:  1.00,

		ProvenanceBoosts: map[string]float64{
			"source:user":       0.20,
			"verified:true":     0.10,
			"source:discovered": 0.05,
		},

		RecencyHalfLifeDays: 30,
		AccessLogBase:       10,
	}
}

// ScoreMemory computes a deterministic priority score for a memory.
// Returns a value in [0.0, 1.0] where higher = more important.
//
// If the memory's Priority field is non-zero, it is used as a manual
// override and the computed score is ignored.
//
// Score components:
//   - Category base score (what kind of memory)
//   - Provenance boost (who created it, was it verified)
//   - Recency factor (how old, with exponential decay)
//   - Access factor (how frequently retrieved, log-scaled)
func ScoreMemory(m *Memory, now time.Time, cfg ScoringConfig) float64 {
	// Manual override: non-zero Priority replaces computed score.
	if m.Priority > 0 {
		return clamp(float64(m.Priority), 0.0, 1.0)
	}

	base := categoryWeight(m, cfg)
	provenance := provenanceBoost(m.Tags, cfg)
	recency := recencyFactor(m.CreatedAt, now, cfg)
	access := accessFactor(m.AccessCount, cfg)

	score := base*cfg.WeightCategory +
		provenance*cfg.WeightProvenance +
		recency*cfg.WeightRecency +
		access*cfg.WeightAccess

	return clamp(score, 0.0, 1.0)
}

// ScoredMemory pairs a memory with its computed score for sorting.
type ScoredMemory struct {
	Memory *Memory
	Score  float64
}

// ScoreBreakdown holds the individual component values and weighted
// contributions for a single memory's score. Useful for debugging and
// observability (e.g., aide memory list --scored).
type ScoreBreakdown struct {
	// Final score in [0.0, 1.0].
	Total float64

	// Whether the score is a manual override (Priority field non-zero).
	ManualOverride bool

	// Raw component values (before weighting).
	CategoryRaw   float64
	ProvenanceRaw float64
	RecencyRaw    float64
	AccessRaw     float64

	// Weighted contributions (raw * weight).
	CategoryWeighted   float64
	ProvenanceWeighted float64
	RecencyWeighted    float64
	AccessWeighted     float64
}

// ScoreMemoryDetailed computes the score like ScoreMemory but returns
// a full breakdown of component values for observability.
func ScoreMemoryDetailed(m *Memory, now time.Time, cfg ScoringConfig) ScoreBreakdown {
	if m.Priority > 0 {
		total := clamp(float64(m.Priority), 0.0, 1.0)
		return ScoreBreakdown{Total: total, ManualOverride: true}
	}

	cat := categoryWeight(m, cfg)
	prov := provenanceBoost(m.Tags, cfg)
	rec := recencyFactor(m.CreatedAt, now, cfg)
	acc := accessFactor(m.AccessCount, cfg)

	return ScoreBreakdown{
		Total:              clamp(cat*cfg.WeightCategory+prov*cfg.WeightProvenance+rec*cfg.WeightRecency+acc*cfg.WeightAccess, 0.0, 1.0),
		CategoryRaw:        cat,
		ProvenanceRaw:      prov,
		RecencyRaw:         rec,
		AccessRaw:          acc,
		CategoryWeighted:   cat * cfg.WeightCategory,
		ProvenanceWeighted: prov * cfg.WeightProvenance,
		RecencyWeighted:    rec * cfg.WeightRecency,
		AccessWeighted:     acc * cfg.WeightAccess,
	}
}

// categoryWeight returns the base importance score for a memory's category.
// Learning memories with scope:global get the GlobalLearningScore override.
func categoryWeight(m *Memory, cfg ScoringConfig) float64 {
	// Special case: learning + scope:global = user preferences (highest)
	if m.Category == CategoryLearning && hasTag(m.Tags, "scope:global") {
		return cfg.GlobalLearningScore
	}

	if score, ok := cfg.CategoryScores[m.Category]; ok {
		return score
	}
	return cfg.DefaultCategoryScore
}

// provenanceBoost sums additive boosts from matching provenance tags.
// The result is clamped to [0.0, 1.0].
func provenanceBoost(tags []string, cfg ScoringConfig) float64 {
	var boost float64
	for _, tag := range tags {
		if b, ok := cfg.ProvenanceBoosts[tag]; ok {
			boost += b
		}
	}
	return clamp(boost, 0.0, 1.0)
}

// recencyFactor computes exponential time decay.
// Returns 1.0 for brand-new memories, 0.5 at half-life, approaching 0 for very old.
// When DecayDisabled is true, always returns 1.0.
func recencyFactor(created time.Time, now time.Time, cfg ScoringConfig) float64 {
	if cfg.DecayDisabled {
		return 1.0
	}

	ageDays := now.Sub(created).Hours() / 24.0
	if ageDays <= 0 {
		return 1.0
	}

	halfLife := cfg.RecencyHalfLifeDays
	if halfLife <= 0 {
		halfLife = 30 // safety fallback
	}

	return math.Pow(0.5, ageDays/halfLife)
}

// accessFactor computes a log-scaled score from access count.
// Returns 0.0 for never-accessed, approaches 1.0 at high access counts.
func accessFactor(accessCount uint32, cfg ScoringConfig) float64 {
	if accessCount == 0 {
		return 0.0
	}

	base := cfg.AccessLogBase
	if base <= 1 {
		base = 10 // safety fallback
	}

	return clamp(math.Log10(float64(accessCount)+1)/math.Log10(base), 0.0, 1.0)
}

// hasTag checks if a tag slice contains a specific tag.
func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// hasTagPrefix checks if any tag in the slice starts with the given prefix.
func hasTagPrefix(tags []string, prefix string) bool {
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

// clamp restricts a value to [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
