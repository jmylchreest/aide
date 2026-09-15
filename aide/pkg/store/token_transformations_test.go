package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestTokenTransformationsPreserveBoundaryAndWindow(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	add := func(call, epoch, stage, before, after string) {
		t.Helper()
		e := &observe.Event{Kind: observe.KindHook, Name: "output-transform", Category: "transform", SessionID: "s", Attrs: map[string]string{
			"accounting_version": "1", "observation_stage": stage, "host": "host", "actor_id": "actor", "invocation_id": call,
			"context_status": "active", "context_epoch": epoch, "before_bytes": before, "after_bytes": after,
		}}
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	add("1", "a", "rewrite_candidate", "300", "30")
	add("1", "a", "rewrite_candidate", "300", "30") // replay is idempotent
	add("2", "a", "adapter_change", "300", "60")
	add("3", "b", "adapter_change", "30", "90") // overhead remains negative
	stats, err := s.TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventCount != 3 {
		t.Fatalf("paired observations missing/duplicated: %d", stats.EventCount)
	}
	if stats.TotalRead != 0 || stats.TotalSaved != 0 || len(stats.CallsByTool) != 0 {
		t.Fatalf("transforms counted as more tool calls or consumption: %+v", stats)
	}
	data, _ := json.Marshal(stats.Accounting)
	// Use wire names explicitly for the asserted aggregate.
	var wire map[string]json.RawMessage
	json.Unmarshal(data, &wire)
	if len(wire["transformations"]) == 0 {
		t.Fatal("missing transformation accounting")
	}
	var report struct {
		ByStage map[string]struct {
			Before int64 `json:"before_bytes"`
			After  int64 `json:"after_bytes"`
			Delta  int64 `json:"delta_bytes"`
			Tokens int64 `json:"estimated_token_delta"`
			Events int   `json:"events"`
		} `json:"by_stage"`
		Windows []json.RawMessage `json:"windows"`
	}
	if err := json.Unmarshal(wire["transformations"], &report); err != nil {
		t.Fatal(err)
	}
	if q := report.ByStage["adapter_change"]; q.Before != 330 || q.After != 150 || q.Delta != 180 || q.Tokens != 60 || q.Events != 2 {
		t.Fatalf("adapter pairing: %+v", q)
	}
	if q := report.ByStage["rewrite_candidate"]; q.Delta != 270 || q.Events != 1 {
		t.Fatalf("candidate pairing: %+v", q)
	}
	if len(report.Windows) != 3 {
		t.Fatalf("mixed stages/epochs: %s", wire["transformations"])
	}
}

func TestTokenTransformationWindowsAreBoundedWithoutLosingTotals(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 100; i++ {
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindHook, Name: "output-transform", Category: "transform", SessionID: "s", Attrs: map[string]string{"accounting_version": "1", "observation_stage": "adapter_change", "host": "h", "actor_id": "a", "invocation_id": fmt.Sprint(i), "context_status": "active", "context_epoch": fmt.Sprint(i), "before_bytes": "30", "after_bytes": "3"}}); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(stats.Accounting)
	var a struct {
		Transformations struct {
			Windows []json.RawMessage `json:"windows"`
			Limited bool              `json:"windows_limited"`
			ByStage map[string]struct {
				Events int `json:"events"`
			} `json:"by_stage"`
		} `json:"transformations"`
	}
	json.Unmarshal(data, &a)
	if !a.Transformations.Limited || len(a.Transformations.Windows) != 64 || a.Transformations.ByStage["adapter_change"].Events != 100 {
		t.Fatalf("bounded report lost aggregate: %s", data)
	}
}

func TestTokenTransformationFiltersAndUnknownEvidence(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for i, spec := range []struct {
		session, after, status string
		timestamp              time.Time
	}{{"s", "0", "active", at}, {"s", "-1", "active", at}, {"s", "3", "pending", at}, {"other", "3", "active", at}, {"s", "3", "active", at.Add(-time.Hour)}} {
		err := s.AddObserveEvent(&observe.Event{Kind: observe.KindHook, Name: "output-transform", Category: "transform", SessionID: spec.session, Timestamp: spec.timestamp, Attrs: map[string]string{"accounting_version": "1", "observation_stage": "adapter_change", "host": "h", "actor_id": "a", "invocation_id": fmt.Sprint(i), "context_epoch": "epoch", "context_status": spec.status, "before_bytes": "0", "after_bytes": spec.after}})
		if err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TokenStats("s", at, at)
	if err != nil {
		t.Fatal(err)
	}
	a := stats.Accounting.Transformations
	if stats.EventCount != 3 || a.InvalidEvents != 1 || a.UnwindowedEvents != 1 || len(a.Windows) != 1 {
		t.Fatalf("filters/unknowns: %+v", a)
	}
	if q := a.ByStage["adapter_change"]; q.Events != 2 || q.BeforeBytes != 0 || q.AfterBytes != 3 || q.DeltaBytes != -3 {
		t.Fatalf("known zero/invalid: %+v", q)
	}
	if a.Windows[0].Change.Events != 1 || a.Windows[0].Change.DeltaBytes != 0 {
		t.Fatal("pending pair entered active window")
	}
}
