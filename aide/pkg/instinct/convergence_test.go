package instinct

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

// convSeq builds an Edit → user-prompt → Edit sequence on one file, with the
// second edit `gap` after the first and a fixed prompt id so classifications
// can target it.
func convSeq(file, promptText, promptID string, gap time.Duration) []*observe.Event {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	return []*observe.Event{
		{ID: "edit-a", Timestamp: base, Kind: observe.KindToolCall, Name: "Edit", FilePath: file, SessionID: "t"},
		{ID: promptID, Timestamp: base.Add(gap / 2), Kind: observe.KindHook, Name: "user_prompt", SessionID: "t", Attrs: map[string]string{"text": promptText}},
		{ID: "edit-b", Timestamp: base.Add(gap), Kind: observe.KindToolCall, Name: "Edit", FilePath: file, SessionID: "t"},
	}
}

func detectConvergence(events []*observe.Event, cls map[string]Classification) []Proposal {
	return Convergence{}.Detect(events, DefaultConvergenceConfig(), ParserContext{Now: time.Now(), Classifications: cls})
}

func TestConvergence_RequiresLLM(t *testing.T) {
	if !(Convergence{}).Capabilities().RequiresLLM {
		t.Fatal("Convergence should be LLM-tier (RequiresLLM=true)")
	}
	events := convSeq("src/a.ts", "no, keep it sync", "p1", time.Minute)
	runner := NewRunner(Convergence{})
	if got := runner.Run("t", events, nil, nil, RunOpts{Mode: RunDeterministic}); len(got) != 0 {
		t.Errorf("deterministic runner should skip convergence, got %d", len(got))
	}
	if got := runner.Run("t", events, nil, nil, RunOpts{Mode: RunWithLLM}); len(got) != 1 {
		t.Errorf("LLM runner should run convergence, got %d", len(got))
	}
}

// Within the time bound, a corrective marker fires.
func TestConvergence_WithinTimeBoundFires(t *testing.T) {
	events := convSeq("src/a.ts", "no, keep it sync", "p1", 10*time.Minute)
	if props := detectConvergence(events, nil); len(props) != 1 {
		t.Fatalf("expected 1 proposal within the time bound, got %d", len(props))
	}
}

// Beyond MaxMinutesBetween (default 30m), the same sequence does NOT fire even
// though only one event sits between the edits.
func TestConvergence_TimeBoundSuppresses(t *testing.T) {
	events := convSeq("src/a.ts", "no, keep it sync", "p1", 90*time.Minute)
	if props := detectConvergence(events, nil); len(props) != 0 {
		t.Fatalf("expected no proposal beyond the time bound, got %d", len(props))
	}
}

// An LLM classification overrides the marker heuristic: a prompt with a
// corrective marker but classified neutral is NOT treated as a correction.
func TestConvergence_ClassificationOverridesMarker(t *testing.T) {
	events := convSeq("src/a.ts", "no, keep it sync", "p1", time.Minute)
	cls := map[string]Classification{"p1": {Intent: "neutral"}}
	if props := detectConvergence(events, cls); len(props) != 0 {
		t.Fatalf("neutral classification should suppress, got %d proposals", len(props))
	}
	// And a corrective classification on a marker-free prompt still fires.
	events2 := convSeq("src/a.ts", "let's try the other approach", "p1", time.Minute)
	cls2 := map[string]Classification{"p1": {Intent: "corrective"}}
	if props := detectConvergence(events2, cls2); len(props) != 1 {
		t.Fatalf("corrective classification should fire on a marker-free prompt, got %d", len(props))
	}
}
