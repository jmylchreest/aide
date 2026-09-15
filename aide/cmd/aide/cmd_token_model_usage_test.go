package main

import (
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestModelUsageCLISeparatesCountersAndCoverage(t *testing.T) {
	u := &memory.TokenModelUsage{Version: 1, Observations: 2, Invalid: 1, Conflicts: 1, BySource: []*memory.ModelUsageSource{{Host: "opencode", Source: "opencode.step_finish.v1", Observations: 2, ObservedTimed: 2, Counters: map[string]*memory.ModelUsageCounter{"reported_output_tokens": {Tokens: 0, Observations: 1}}}}}
	text := formatModelUsage(u, true)
	for _, want := range []string{"Host-reported model usage", "partial coverage", "2 captured", "1 conflicting", "1 invalid", "opencode", "reported_output_tokens: 0 tokens; 1/2 records", "reasoning inclusion unknown", "2 observation-timed", "cost, savings or task quality"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(formatModelUsage(u, false), "reported_output_tokens:") {
		t.Fatal("details leaked into compact summary")
	}
	if !strings.Contains(formatModelUsage(nil, true), "unavailable") {
		t.Fatal("old server implied zero")
	}
}
