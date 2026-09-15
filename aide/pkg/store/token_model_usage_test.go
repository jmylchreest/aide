package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func usageEvent(id string, at time.Time, counters map[string]string) *observe.Event {
	attrs := map[string]string{"model_usage_version": "1", "host": "codex", "usage_source": "codex.token_usage_record.v1", "usage_id": id, "usage_time_basis": "source", "usage_source_time": at.Format(time.RFC3339Nano)}
	for k, v := range counters {
		attrs[k] = v
	}
	return &observe.Event{Kind: observe.KindSession, Name: "model_usage", SessionID: "s", Timestamp: at, Attrs: attrs}
}
func TestModelUsageDedupConflictCoverageAndNoEstimateLeak(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	add := func(e *observe.Event) {
		t.Helper()
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	first := usageEvent("a", at, map[string]string{"input_tokens": "10", "cache_read_input_tokens": "0", "output_tokens": "2", "total_tokens": "12"})
	add(first)
	retry := usageEvent("a", at, map[string]string{"input_tokens": "10", "cache_read_input_tokens": "0", "output_tokens": "2", "total_tokens": "12"})
	add(retry)
	if first.ID != retry.ID {
		t.Fatal("retry grew storage")
	}
	add(usageEvent("partial", at, map[string]string{"output_tokens": "0"}))
	add(usageEvent("conflict", at, map[string]string{"input_tokens": "20"}))
	add(usageEvent("conflict", at.Add(-2*time.Hour), map[string]string{"input_tokens": "30"}))
	add(usageEvent("invalid", at, map[string]string{"input_tokens": "-1"}))
	stats, err := s.TokenStats("s", at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	u := stats.Accounting.ModelUsage
	if u == nil || u.Observations != 2 || u.Conflicts != 1 || u.Invalid != 1 || len(u.BySource) != 1 {
		t.Fatalf("summary %+v", u)
	}
	g := u.BySource[0]
	if g.SourceTimed != 2 || g.ObservedTimed != 0 || g.Counters["input_tokens"].Tokens != 10 || g.Counters["input_tokens"].Observations != 1 || g.Counters["output_tokens"].Observations != 2 || g.Counters["cache_read_input_tokens"].Tokens != 0 {
		t.Fatalf("coverage %+v", g)
	}
	if _, ok := g.Counters["reasoning_output_tokens"]; ok {
		t.Fatal("missing imputed")
	}
	if stats.Sessions != 1 {
		t.Fatalf("usage-only session lost: %d", stats.Sessions)
	}
	if stats.EventCount != 0 || stats.TotalRead != 0 || stats.Accounting.Work.Calls != 0 || len(stats.Accounting.ByStage) != 0 {
		t.Fatalf("usage leaked: %+v", stats)
	}
}
func TestModelUsageObservedRetriesAndValidation(t *testing.T) {
	at := time.Now()
	for name, counters := range map[string]map[string]string{
		"unsafe": {"input_tokens": "9007199254740992"}, "decimal": {"input_tokens": "1.5"}, "bool": {"input_tokens": "true"}, "empty": {}, "cache": {"input_tokens": "1", "cache_read_input_tokens": "2"}, "total": {"input_tokens": "1", "output_tokens": "2", "total_tokens": "4"}, "reasoning": {"output_tokens": "1", "reasoning_output_tokens": "2"},
	} {
		t.Run(name, func(t *testing.T) {
			a := newModelUsage()
			a.observe(usageEvent(name, at, counters))
			if got := a.result("s", time.Time{}, time.Time{}); got.Invalid != 1 || got.Observations != 0 {
				t.Fatalf("%+v", got)
			}
		})
	}
	a := newModelUsage()
	e := usageEvent("same", at, map[string]string{"input_tokens": "0"})
	e.Attrs["usage_time_basis"] = "observed"
	a.observe(e)
	e2 := usageEvent("same", at.Add(time.Hour), map[string]string{"input_tokens": "0"})
	e2.Attrs["usage_time_basis"] = "observed"
	a.observe(e2)
	got := a.result("s", time.Time{}, time.Time{})
	if got.Observations != 1 || got.Conflicts != 0 || got.BySource[0].ObservedTimed != 1 {
		t.Fatalf("%+v", got)
	}
	if got := a.result("s", at.Add(time.Minute), time.Time{}); got.Observations != 0 {
		t.Fatal("retry moved time window")
	}
}

func TestObserveListsOrderBySourceTimestampAfterImport(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now()
	for _, e := range []*observe.Event{
		{ID: "01", Kind: observe.KindToolCall, Name: "recent", Category: "consume", Timestamp: at},
		{ID: "03", Kind: observe.KindToolCall, Name: "imported-old", Category: "consume", Timestamp: at.Add(-time.Hour)},
		{ID: "02", Kind: observe.KindToolCall, Name: "recent-tie", Category: "consume", Timestamp: at},
	} {
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.ListObserveEvents(ObserveFilter{Since: at.Add(-time.Minute), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Name != "recent-tie" {
		t.Fatalf("time filter/order: %+v", events)
	}
	tokens, err := s.ListTokenEvents("", 1, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Tool != "recent-tie" {
		t.Fatalf("token order: %+v", tokens)
	}
}
func TestModelUsageSourceTimesFanoutOldCLIAndOverflow(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	late := usageEvent("same", at.Add(time.Hour), map[string]string{"output_tokens": "0"})
	early := usageEvent("same", at, map[string]string{"output_tokens": "0"})
	for _, e := range []*observe.Event{late, early} {
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	if late.ID != early.ID {
		t.Fatal("fanout duplicate grew storage")
	}
	u, err := s.TokenStats("s", at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if u.Accounting.ModelUsage.Observations != 1 {
		t.Fatal("earliest source timestamp lost")
	}
	a := newModelUsage()
	oldCLI := usageEvent("old", at, map[string]string{"input_tokens": "10"})
	oldCLI.Timestamp = at.Add(time.Hour)
	a.observe(oldCLI)
	if got := a.result("s", at.Add(-time.Minute), at.Add(time.Minute)); got.Observations != 1 {
		t.Fatal("old CLI ingestion time used")
	}
	a = newModelUsage()
	a.observe(usageEvent("a", at, map[string]string{"input_tokens": "9007199254740991"}))
	a.observe(usageEvent("b", at, map[string]string{"input_tokens": "1"}))
	if got := a.result("s", time.Time{}, time.Time{}); got.Invalid != 1 || got.Observations != 1 || got.BySource[0].Counters["input_tokens"].Tokens != maxUsageTokens {
		t.Fatalf("unsafe sum %+v", got)
	}
}

func TestModelUsagePartialTotalsMustBeCoherent(t *testing.T) {
	for _, counter := range []string{"input_tokens", "output_tokens", "reasoning_output_tokens", "cache_read_input_tokens", "cache_write_input_tokens", "uncached_input_tokens"} {
		t.Run(counter, func(t *testing.T) {
			a := newModelUsage()
			a.observe(usageEvent("a", time.Now(), map[string]string{counter: "10", "total_tokens": "5"}))
			if got := a.result("", time.Time{}, time.Time{}); got.Invalid != 1 || got.Observations != 0 {
				t.Fatalf("impossible total: %+v", got)
			}
		})
	}
}
func TestModelUsageInvalidSourceTimeCannotMaskValidEvidence(t *testing.T) {
	for _, invalidFirst := range []bool{true, false} {
		s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
		if err != nil {
			t.Fatal(err)
		}
		at := time.Now()
		good := usageEvent("a", at, map[string]string{"input_tokens": "1"})
		bad := usageEvent("a", at, map[string]string{"input_tokens": "1"})
		bad.Attrs["usage_source_time"] = "bad"
		pair := []*observe.Event{good, bad}
		if invalidFirst {
			pair = []*observe.Event{bad, good}
		}
		for _, e := range pair {
			if err := s.AddObserveEvent(e); err != nil {
				t.Fatal(err)
			}
		}
		if pair[0].ID == pair[1].ID {
			t.Fatal("invalid source evidence dropped")
		}
		got, err := s.TokenStats("", time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Accounting.ModelUsage.Conflicts != 1 || got.Accounting.ModelUsage.Observations != 0 {
			t.Fatalf("%+v", got.Accounting.ModelUsage)
		}
		s.Close()
	}
	a := newModelUsage()
	at := time.Now()
	a.observe(usageEvent("a", at.Add(3*time.Hour), map[string]string{"input_tokens": "1"}))
	a.observe(usageEvent("a", at, map[string]string{"input_tokens": "1"}))
	a.observe(usageEvent("a", at.Add(4*time.Hour), map[string]string{"input_tokens": "2"}))
	if got := a.result("", at.Add(-time.Minute), at.Add(time.Minute)); got.Conflicts != 1 {
		t.Fatalf("earliest canonical conflict missed: %+v", got)
	}
}

func TestModelUsageKeepsHostsModelsAndCounterCoverageSeparate(t *testing.T) {
	a := newModelUsage()
	at := time.Now()
	for _, host := range []string{"codex", "claude-code", "opencode"} {
		e := usageEvent("same-id", at, map[string]string{"input_tokens": "0"})
		e.Attrs["host"] = host
		e.Attrs["usage_source"] = map[string]string{"codex": "codex.token_usage_record.v1", "claude-code": "claude.assistant_usage.v1", "opencode": "opencode.step_finish.v1"}[host]
		a.observe(e)
	}
	e := usageEvent("other-model", at, map[string]string{"input_tokens": "10"})
	e.Attrs["model"] = "other"
	a.observe(e)
	got := a.result("", time.Time{}, time.Time{})
	if got.Observations != 4 || len(got.BySource) != 4 {
		t.Fatalf("sources combined %+v", got)
	}
	for _, g := range got.BySource {
		if g.Observations != 1 || g.Counters["input_tokens"].Observations != 1 || g.SourceTimed+g.ObservedTimed != g.Observations {
			t.Fatalf("group %+v", g)
		}
	}
	if got := a.result("another-session", time.Time{}, time.Time{}); got.Observations != 0 || len(got.BySource) != 0 {
		t.Fatal("session selection leaked")
	}
}
