package memory

import (
	"math"
	"testing"
	"time"
)

func TestDefaultScoringConfig(t *testing.T) {
	cfg := DefaultScoringConfig()

	// Weights should sum to 1.0.
	sum := cfg.WeightCategory + cfg.WeightProvenance + cfg.WeightRecency + cfg.WeightAccess
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("weights sum to %f, want 1.0", sum)
	}

	// All category scores should be in [0, 1].
	for cat, score := range cfg.CategoryScores {
		if score < 0 || score > 1 {
			t.Errorf("category %q score %f out of [0,1]", cat, score)
		}
	}
}

func TestScoreMemory_Deterministic(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	m := &Memory{
		Category:    CategoryLearning,
		Tags:        []string{"project:aide", "source:discovered", "verified:true"},
		AccessCount: 3,
		CreatedAt:   now.Add(-7 * 24 * time.Hour), // 7 days ago
	}

	score1 := ScoreMemory(m, now, cfg)
	score2 := ScoreMemory(m, now, cfg)

	if score1 != score2 {
		t.Errorf("non-deterministic: %f != %f", score1, score2)
	}
	if score1 < 0 || score1 > 1 {
		t.Errorf("score %f out of [0,1]", score1)
	}
}

func TestScoreMemory_ManualOverride(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	m := &Memory{
		Category:  CategoryLearning,
		Priority:  0.95,
		CreatedAt: now.Add(-365 * 24 * time.Hour), // very old
	}

	score := ScoreMemory(m, now, cfg)
	if math.Abs(score-0.95) > 0.001 {
		t.Errorf("manual override: got %f, want ~0.95", score)
	}
}

func TestScoreMemory_ManualOverrideClamped(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	m := &Memory{
		Category:  CategoryLearning,
		Priority:  1.5, // exceeds range
		CreatedAt: now,
	}

	score := ScoreMemory(m, now, cfg)
	if score != 1.0 {
		t.Errorf("clamped override: got %f, want 1.0", score)
	}
}

func TestScoreMemory_CategoryPriorityOrder(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	// All memories created now, no access, same provenance — so only
	// category weight matters (recency=1.0, access=0.0, provenance=same).
	tags := []string{"source:discovered"}

	categories := []struct {
		cat  Category
		tags []string
	}{
		{CategoryLearning, append([]string{"scope:global"}, tags...)}, // user prefs
		{CategoryAbandoned, tags},
		{CategoryBlocker, tags},
		{CategoryIssue, tags},
		{CategoryDiscovery, tags},
		{CategoryLearning, tags}, // project learning (no scope:global)
		{"session", tags},
	}

	var prev = 2.0 // above max
	for i, tc := range categories {
		m := &Memory{
			Category:  tc.cat,
			Tags:      tc.tags,
			CreatedAt: now,
		}
		score := ScoreMemory(m, now, cfg)
		if score >= prev {
			t.Errorf("category at index %d (%s) scored %f >= previous %f — priority order violated",
				i, tc.cat, score, prev)
		}
		prev = score
	}
}

func TestScoreMemory_ProvenanceBoost(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	base := &Memory{Category: CategoryLearning, CreatedAt: now}
	withUser := &Memory{Category: CategoryLearning, Tags: []string{"source:user"}, CreatedAt: now}
	withVerified := &Memory{Category: CategoryLearning, Tags: []string{"verified:true"}, CreatedAt: now}
	withDiscovered := &Memory{Category: CategoryLearning, Tags: []string{"source:discovered"}, CreatedAt: now}
	withBoth := &Memory{Category: CategoryLearning, Tags: []string{"source:user", "verified:true"}, CreatedAt: now}

	scoreBase := ScoreMemory(base, now, cfg)
	scoreUser := ScoreMemory(withUser, now, cfg)
	scoreVerified := ScoreMemory(withVerified, now, cfg)
	scoreDiscovered := ScoreMemory(withDiscovered, now, cfg)
	scoreBoth := ScoreMemory(withBoth, now, cfg)

	if scoreUser <= scoreBase {
		t.Errorf("source:user should boost: %f <= %f", scoreUser, scoreBase)
	}
	if scoreVerified <= scoreBase {
		t.Errorf("verified:true should boost: %f <= %f", scoreVerified, scoreBase)
	}
	if scoreDiscovered <= scoreBase {
		t.Errorf("source:discovered should boost: %f <= %f", scoreDiscovered, scoreBase)
	}
	if scoreUser <= scoreVerified {
		t.Errorf("source:user should boost more than verified:true: %f <= %f", scoreUser, scoreVerified)
	}
	if scoreBoth <= scoreUser {
		t.Errorf("combined tags should boost more: %f <= %f", scoreBoth, scoreUser)
	}
}

func TestScoreMemory_RecencyDecay(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	recent := &Memory{Category: CategoryLearning, CreatedAt: now.Add(-24 * time.Hour)} // 1 day
	month := &Memory{Category: CategoryLearning, CreatedAt: now.Add(-30 * 24 * time.Hour)}
	old := &Memory{Category: CategoryLearning, CreatedAt: now.Add(-90 * 24 * time.Hour)}

	scoreRecent := ScoreMemory(recent, now, cfg)
	scoreMonth := ScoreMemory(month, now, cfg)
	scoreOld := ScoreMemory(old, now, cfg)

	if scoreRecent <= scoreMonth {
		t.Errorf("1-day should score higher than 30-day: %f <= %f", scoreRecent, scoreMonth)
	}
	if scoreMonth <= scoreOld {
		t.Errorf("30-day should score higher than 90-day: %f <= %f", scoreMonth, scoreOld)
	}
}

func TestScoreMemory_WithDecayOff(t *testing.T) {
	cfg := DefaultScoringConfig()
	cfg.DecayEnabled = false
	now := time.Now()

	recent := &Memory{Category: CategoryLearning, CreatedAt: now}
	old := &Memory{Category: CategoryLearning, CreatedAt: now.Add(-365 * 24 * time.Hour)}

	scoreRecent := ScoreMemory(recent, now, cfg)
	scoreOld := ScoreMemory(old, now, cfg)

	if scoreRecent != scoreOld {
		t.Errorf("with decay disabled, age should not matter: recent=%f old=%f", scoreRecent, scoreOld)
	}
}

func TestScoreMemory_AccessFactor(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	noAccess := &Memory{Category: CategoryLearning, AccessCount: 0, CreatedAt: now}
	someAccess := &Memory{Category: CategoryLearning, AccessCount: 3, CreatedAt: now}
	manyAccess := &Memory{Category: CategoryLearning, AccessCount: 9, CreatedAt: now}

	scoreNone := ScoreMemory(noAccess, now, cfg)
	scoreSome := ScoreMemory(someAccess, now, cfg)
	scoreMany := ScoreMemory(manyAccess, now, cfg)

	if scoreSome <= scoreNone {
		t.Errorf("3 accesses should score higher than 0: %f <= %f", scoreSome, scoreNone)
	}
	if scoreMany <= scoreSome {
		t.Errorf("9 accesses should score higher than 3: %f <= %f", scoreMany, scoreSome)
	}
}

func TestRecencyFactor_Values(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ageDays float64
		want    float64
		delta   float64
	}{
		{"brand new", 0, 1.0, 1e-9},
		{"half-life", 30, 0.5, 1e-9},
		{"two half-lives", 60, 0.25, 1e-9},
		{"three half-lives", 90, 0.125, 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := now.Add(-time.Duration(tt.ageDays*24) * time.Hour)
			got := recencyFactor(created, now, cfg)
			if math.Abs(got-tt.want) > tt.delta {
				t.Errorf("recencyFactor(age=%v days) = %f, want %f", tt.ageDays, got, tt.want)
			}
		})
	}
}

func TestRecencyFactor_WithDecayOff(t *testing.T) {
	cfg := DefaultScoringConfig()
	cfg.DecayEnabled = false
	now := time.Now()

	old := now.Add(-365 * 24 * time.Hour)
	got := recencyFactor(old, now, cfg)
	if got != 1.0 {
		t.Errorf("decay disabled: got %f, want 1.0", got)
	}
}

func TestAccessFactor_Values(t *testing.T) {
	cfg := DefaultScoringConfig()

	tests := []struct {
		count uint32
		want  float64
		delta float64
	}{
		{0, 0.0, 1e-9},
		{9, 1.0, 1e-9},      // log10(10)/log10(10) = 1.0
		{99, 1.0, 1e-9},     // clamped at 1.0
		{1, 0.30103, 0.001}, // log10(2)/log10(10) ≈ 0.301
	}

	for _, tt := range tests {
		got := accessFactor(tt.count, cfg)
		if math.Abs(got-tt.want) > tt.delta {
			t.Errorf("accessFactor(%d) = %f, want %f (±%f)", tt.count, got, tt.want, tt.delta)
		}
	}
}

func TestCategoryWeight_GlobalLearning(t *testing.T) {
	cfg := DefaultScoringConfig()

	globalMem := &Memory{Category: CategoryLearning, Tags: []string{"scope:global", "source:user"}}
	projectMem := &Memory{Category: CategoryLearning, Tags: []string{"project:aide"}}

	globalScore := categoryWeight(globalMem, cfg)
	projectScore := categoryWeight(projectMem, cfg)

	if globalScore != cfg.GlobalLearningScore {
		t.Errorf("global learning: got %f, want %f", globalScore, cfg.GlobalLearningScore)
	}
	if globalScore <= projectScore {
		t.Errorf("global learning should score higher than project: %f <= %f", globalScore, projectScore)
	}
}

func TestCategoryWeight_UnknownCategory(t *testing.T) {
	cfg := DefaultScoringConfig()

	m := &Memory{Category: "some-future-category"}
	score := categoryWeight(m, cfg)
	if score != cfg.DefaultCategoryScore {
		t.Errorf("unknown category: got %f, want %f", score, cfg.DefaultCategoryScore)
	}
}

func TestProvenanceBoost_Additive(t *testing.T) {
	cfg := DefaultScoringConfig()

	single := provenanceBoost([]string{"source:user"}, cfg)
	double := provenanceBoost([]string{"source:user", "verified:true"}, cfg)

	expectedSingle := cfg.ProvenanceBoosts["source:user"]
	expectedDouble := cfg.ProvenanceBoosts["source:user"] + cfg.ProvenanceBoosts["verified:true"]

	if math.Abs(single-expectedSingle) > 1e-9 {
		t.Errorf("single tag: got %f, want %f", single, expectedSingle)
	}
	if math.Abs(double-expectedDouble) > 1e-9 {
		t.Errorf("double tag: got %f, want %f", double, expectedDouble)
	}
}

func TestProvenanceBoost_Clamped(t *testing.T) {
	cfg := DefaultScoringConfig()
	// Even with all boosts, should not exceed 1.0
	allTags := []string{"source:user", "verified:true", "source:discovered"}
	got := provenanceBoost(allTags, cfg)
	if got > 1.0 {
		t.Errorf("provenance boost should be clamped: got %f", got)
	}
}

func TestScoreMemory_ZeroPriorityUsesComputed(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	m := &Memory{
		Category:  CategoryAbandoned,
		Tags:      []string{"source:discovered"},
		CreatedAt: now,
		Priority:  0, // zero = use computed
	}

	score := ScoreMemory(m, now, cfg)
	// Should not be zero — computed score includes category weight
	if score == 0 {
		t.Error("zero priority should use computed score, got 0")
	}
}

func TestScoreMemory_FutureCreatedAt(t *testing.T) {
	cfg := DefaultScoringConfig()
	now := time.Now()

	m := &Memory{
		Category:  CategoryLearning,
		CreatedAt: now.Add(24 * time.Hour), // future
	}

	score := ScoreMemory(m, now, cfg)
	if score < 0 || score > 1 {
		t.Errorf("future memory score out of range: %f", score)
	}
}
