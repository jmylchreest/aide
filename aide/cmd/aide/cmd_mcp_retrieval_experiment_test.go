package main

import (
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestRetrievalExperimentAccounting(t *testing.T) {
	// Count the full-file alternative once, including UTF-8 bytes. Keep the
	// failed attempt and full fallback; they make this workflow more expensive.
	r := newRetrievalTrial("fallback", "αβ\n")
	r.add("outline", "x", false, 1)
	r.add("symbol", "missing", true, 2)
	r.add("full_file", "αβ\n", false, 3)
	if r.ReferenceBytes != 5 || r.ResultBytes != 13 || r.ByteDelta != -8 || r.Failures != 1 || len(r.Steps) != 3 {
		t.Fatalf("lost UTF-8, failure, fallback or signed cost: %+v", r)
	}
	wantTokens := memory.EstimateTextTokens(5) - memory.EstimateTextTokens(1) - memory.EstimateTextTokens(7) - memory.EstimateTextTokens(5)
	if r.EstimatedTokenDelta != wantTokens {
		t.Fatalf("must use central per-result estimator: %+v", r)
	}
	if r.EvidenceChecksPassed || r.ProviderUsage != nil || r.ModelQuality != nil {
		t.Fatal("unperformed checks must not become success, provider usage or model quality")
	}
}
