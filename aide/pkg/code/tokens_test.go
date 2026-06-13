package code

import (
	"strings"
	"testing"
)

func TestEstimateTokensForText(t *testing.T) {
	if got := EstimateTokensForText(""); got != 0 {
		t.Errorf("empty text: got %d, want 0", got)
	}

	// Scales with length using the language-agnostic default ratio.
	short := EstimateTokensForText(strings.Repeat("x", 30))
	long := EstimateTokensForText(strings.Repeat("x", 300))
	if short <= 0 {
		t.Errorf("non-empty text should cost > 0 tokens, got %d", short)
	}
	if long <= short {
		t.Errorf("longer text should cost more tokens: short=%d long=%d", short, long)
	}

	// Sanity: ~3 chars per token, so 30 chars is ~10 tokens.
	if short < 8 || short > 12 {
		t.Errorf("30 chars: got %d tokens, expected ~10", short)
	}
}
