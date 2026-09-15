package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestTokenWorkPreservesOutcomesAndCoverage(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	add := func(name, session, stage, outcome, duration, payload, eventError string, at time.Time) {
		t.Helper()
		attrs := map[string]string{"accounting_version": "1", "observation_stage": stage}
		if outcome != "" {
			attrs["work_version"] = "1"
			attrs["work_outcome"] = outcome
		}
		if duration != "" {
			attrs["work_elapsed_ms"] = duration
		}
		if payload != "" {
			attrs["payload_bytes"] = payload
		}
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindToolCall, Name: name, SessionID: session, Timestamp: at, Error: eventError, DurationMs: 99, Attrs: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	add("code_search", "s", "server_result", "returned", "0", "0", "", now)
	add("code_search", "s", "server_result", "reported_error", "7", "12", "failure", now)
	add("survey_run", "s", "server_result", "", "", "6", "", now)
	add("survey_run", "s", "server_result", "", "", "", "old failure", now)
	add("code_search", "s", "host_result", "returned", "8", "999", "", now)
	add("memory_list", "", "server_result", "returned", "2", "3", "", now)
	add("memory_list", "other", "server_result", "returned", "2", "3", "", now)
	add("memory_list", "s", "server_result", "returned", "2", "3", "", now.Add(-2*time.Hour))
	stats, err := s.TokenStats("s", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	w := stats.Accounting.Work
	if w == nil || w.Version != 1 || w.Calls != 4 || w.Returned != 1 || w.ReportedErrors != 2 || w.UnknownOutcomes != 1 {
		t.Fatalf("outcomes: %+v", w)
	}
	if w.ElapsedMs != 7 || w.MeasuredDurations != 2 || w.MissingDurations != 2 || w.UnassignedSessions != 0 {
		t.Fatalf("duration/identity coverage: %+v", w)
	}
	if w.ReturnedText.Bytes != 18 || w.ReturnedText.Events != 3 || w.ReturnedText.EstimatedTokens != 6 || w.MissingPayload != 1 {
		t.Fatalf("text coverage: %+v", w)
	}
	if len(w.ByTool) != 2 || w.ByTool["code_search"].Calls != 2 || w.ByTool["survey_run"].UnknownOutcomes != 1 {
		t.Fatalf("groups: %+v", w.ByTool)
	}
	all, err := s.TokenStats("", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if all.Accounting.Work.Calls != 6 || all.Accounting.Work.UnassignedSessions != 1 {
		t.Fatalf("all sessions: %+v", all.Accounting.Work)
	}
	empty, err := s.TokenStats("missing", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Accounting.Work == nil || empty.Accounting.Work.Calls != 0 || empty.Accounting.Work.ByTool == nil {
		t.Fatalf("known empty: %+v", empty.Accounting.Work)
	}
}

func TestTokenWorkIncludesTimeBoundsAndPreservesExplicitErrors(t *testing.T) {
	since := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	w := memory.NewTokenWork()
	for _, at := range []time.Time{since.Add(-time.Nanosecond), since, until, until.Add(time.Nanosecond)} {
		addTokenWork(w, &observe.Event{
			Kind: observe.KindToolCall, Name: "code_outline", SessionID: "s", Timestamp: at, Error: "recorded failure",
			Attrs: map[string]string{"accounting_version": "1", "observation_stage": "server_result", "work_version": "1", "work_outcome": "returned", "work_elapsed_ms": "0", "payload_bytes": "0"},
		}, "s", since, until)
	}
	if w.Calls != 2 || w.ReportedErrors != 2 || w.Returned != 0 || w.MeasuredDurations != 2 || w.ElapsedMs != 0 || w.ReturnedText.Events != 2 {
		t.Fatalf("inclusive bounds or error precedence lost: %+v", w)
	}
}

func TestTokenWorkRejectsUnversionedAndMalformedEvidence(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, attrs := range []map[string]string{
		{"accounting_version": "1", "observation_stage": "server_result", "work_version": "1", "work_outcome": "success", "work_elapsed_ms": "-1", "payload_bytes": "-1"},
		{"accounting_version": "1", "observation_stage": "server_result", "work_version": "2", "work_outcome": "returned", "work_elapsed_ms": "5", "payload_bytes": "0"},
		{"accounting_version": "1", "observation_stage": "server_result", "work_version": "1", "work_outcome": "returned", "work_elapsed_ms": "9007199254740992"},
		{"observation_stage": "server_result", "work_version": "1", "work_outcome": "returned", "work_elapsed_ms": "5"},
	} {
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindToolCall, Name: "code_search", Attrs: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindSpan, Name: "background", Attrs: map[string]string{"accounting_version": "1", "observation_stage": "server_result"}}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.TokenStats("", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	w := stats.Accounting.Work
	if w.Calls != 3 || w.Returned != 1 || w.UnknownOutcomes != 2 || w.MeasuredDurations != 0 || w.MissingDurations != 3 || w.MissingPayload != 2 {
		t.Fatalf("invalid evidence: %+v", w)
	}
}
