package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestPreparedContextAccounting(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, payload := range []string{"6", "0", "missing"} {
		attrs := map[string]string{"accounting_version": "1", "observation_stage": "aide_context", "argument_bytes": "99"}
		if payload != "missing" {
			attrs["payload_bytes"] = payload
		}
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindInjection, Name: "memory", SessionID: "s", Tokens: 999, Attrs: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindInjection, Name: "legacy", SessionID: "s", Tokens: 7}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q := stats.Accounting.ByStage["aide_context"]
	if q == nil || q.Bytes != 6 || q.EstimatedTokens != 2 || q.Events != 2 {
		t.Fatalf("context: %+v", q)
	}
	if stats.TotalDelivered != 9 || stats.TotalRead != 0 || stats.Accounting.Arguments.Events != 0 || len(stats.CallsByTool) != 0 || stats.Accounting.Work.Calls != 0 {
		t.Fatalf("cross-boundary contamination: %+v", stats)
	}
	if stats.Accounting.MissingPayload != 1 || stats.Accounting.LegacyEvents != 1 {
		t.Fatalf("coverage: %+v", stats.Accounting)
	}
	for _, b := range stats.Accounting.Activity.Buckets {
		if b.Unmeasured != 0 || len(b.ByStage) != 0 {
			t.Fatalf("injection must not enter result activity: %+v", b)
		}
	}
	events, err := s.ListTokenEvents("s", 0, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("projection missing injection: %d", len(events))
	}
	other, err := s.TokenStats("other", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if other.Accounting.ByStage["aide_context"] != nil {
		t.Fatal("session leakage")
	}
}

func TestPreparedContextStageRequiresInjectionKind(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, kind := range []observe.Kind{observe.KindToolCall, observe.KindSpan, observe.KindHook} {
		if err := s.AddObserveEvent(&observe.Event{Kind: kind, Name: "unrelated", Category: "consume", Attrs: map[string]string{"accounting_version": "1", "observation_stage": "aide_context", "payload_bytes": "9"}}); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TokenStats("", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Accounting.ByStage["aide_context"] != nil {
		t.Fatal("non-injection counted as context")
	}
}
