package main

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"strings"
	"testing"
	"time"
)

func TestTransformationSummaryKeepsEvidenceAndOverheadExplicit(t *testing.T) {
	report := memory.NewTokenTransformations()
	report.ByStage["adapter_change"] = &memory.TokenChange{BeforeBytes: 30, AfterBytes: 90, DeltaBytes: -60, EstimatedTokenDelta: -20, Events: 1}
	text := formatTransformationSummary(report, false)
	if !strings.Contains(text, "-60 bytes") || !strings.Contains(text, "~-20 tokens") || !strings.Contains(text, "Proposed rewrites: unknown") || !strings.Contains(text, "final delivery") {
		t.Fatalf("misleading evidence: %s", text)
	}
}

func TestTokenTimeRange(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	since, until, err := tokenTimeRange([]string{"--since=24h", "--until=2026-09-07T11:00:00Z"}, now)
	if err != nil || !since.Equal(now.Add(-24*time.Hour)) || !until.Equal(now.Add(-time.Hour)) {
		t.Fatalf("range: %v %v %v", since, until, err)
	}
	for _, args := range [][]string{{"--since=bad"}, {"--since=-1h"}, {"--until=bad"}, {"--since=2026-09-08T00:00:00Z", "--until=2026-09-07T00:00:00Z"}} {
		if _, _, err := tokenTimeRange(args, now); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestRetrievalSummaryKeepsDeliveryLimitsExplicit(t *testing.T) {
	text := formatRetrievalEvidence(map[string]string{"retrieval_status": "range", "source_verification": "current_range_match", "delivered_start_line": "2", "delivered_end_line": "3", "retrieval_method": "shell_sed"})
	if !strings.Contains(text, "lines 2-3") || !strings.Contains(text, "undisplayed source version unverified") || strings.Contains(text, "saved") {
		t.Fatalf("misleading range evidence: %s", text)
	}
	if text := formatRetrievalEvidence(map[string]string{"retrieval_status": "unclassified_shell"}); !strings.Contains(text, "coverage unknown") {
		t.Fatalf("misleading shell coverage: %s", text)
	}
	if formatRetrievalEvidence(nil) != "" {
		t.Fatal("legacy event should not acquire evidence")
	}
}

func TestRetrievalWindowSummaryDoesNotClaimSavings(t *testing.T) {
	report := &memory.TokenRetrievals{Windows: []*memory.RetrievalWindow{{Host: "host", SessionID: "s", Epoch: "w", Events: 2, Comparison: &memory.TokenChange{DeltaBytes: -90, EstimatedTokenDelta: -30}}, {Host: "host", SessionID: "s", Epoch: "gap", Issues: []string{"missing_payload"}}}}
	compact := formatRetrievalWindows(report, false)
	if !strings.Contains(compact, "2 windows") || !strings.Contains(compact, "1 conditional comparison") || strings.Contains(compact, "-90") {
		t.Fatalf("verbose or misleading summary: %s", compact)
	}
	details := formatRetrievalWindows(report, true)
	if !strings.Contains(details, "90 more bytes") || !strings.Contains(details, "~30 more tokens") || !strings.Contains(details, "comparison unavailable") || strings.Contains(details, "tokens saved") {
		t.Fatalf("misleading detail: %s", details)
	}
}
