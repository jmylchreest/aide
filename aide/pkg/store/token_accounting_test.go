package store

import (
	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
	"path/filepath"
	"testing"
	"time"
)

func TestObservationIdentityIsolationAndRetention(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, identity := range [][3]string{{"s", "a", "host_result"}, {"s", "b", "host_result"}, {"other", "a", "host_result"}, {"s", "a", "server_result"}} {
		for range 2 {
			e := &observe.Event{Kind: observe.KindToolCall, Name: "Read", Category: "consume", SessionID: identity[0], Timestamp: time.Now().Add(-48 * time.Hour), Attrs: map[string]string{"host": "test", "actor_id": identity[1], "invocation_id": "call", "observation_stage": identity[2]}}
			if err := s.AddObserveEvent(e); err != nil {
				t.Fatal(err)
			}
		}
	}
	events, err := s.ListObserveEvents(ObserveFilter{})
	if err != nil || len(events) != 4 {
		t.Fatalf("isolation: %d %v", len(events), err)
	}
	count, err := s.CleanupObserveEvents(24 * time.Hour)
	if err != nil || count != 4 {
		t.Fatalf("cleanup: %d %v", count, err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket([]byte("observe_origins")); b != nil && b.Stats().KeyN != 0 {
			t.Error("retention left origin index entries")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTokenAccountingEvidenceAndIdentity(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	makeEvent := func(id string) *observe.Event {
		return &observe.Event{Kind: observe.KindToolCall, Name: "Bash", Category: "execute", SessionID: "s", Attrs: map[string]string{"accounting_version": "1", "payload_bytes": "6", "observation_stage": "host_result", "host": "claude-code", "actor_id": "s", "invocation_id": id}}
	}
	first := makeEvent("call1")
	duplicate := makeEvent("call1")
	for _, e := range []*observe.Event{first, duplicate, makeEvent("call2"), {Kind: observe.KindToolCall, Name: "Read", Category: "consume", Subtype: "file", Tokens: 99, TokensSaved: 12}} {
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	if duplicate.ID != first.ID {
		t.Fatal("duplicate origin must retain original event identity")
	}
	stats, err := s.TokenStats("", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventCount != 3 || stats.Sessions != 1 {
		t.Fatalf("counts: %+v", stats)
	}
	a := stats.Accounting
	if a == nil || a.Version != 1 || a.LegacyEvents != 1 || a.MissingIdentity != 1 {
		t.Fatalf("coverage: %+v", a)
	}
	if q := a.ByStage["host_result"]; q.Bytes != 12 || q.EstimatedTokens != 4 || q.Events != 2 {
		t.Fatalf("payload: %+v", q)
	}
	if stats.TotalWritten != 0 {
		t.Fatal("shell result must not be classified as generated arguments")
	}
}

func TestTokenAccountingUnknownEmptyAndStages(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, attrs := range []map[string]string{
		{"accounting_version": "1", "observation_stage": "host_result", "payload_bytes": "0", "argument_bytes": "9"},
		{"accounting_version": "1", "observation_stage": "host_result"},
		{"accounting_version": "1", "observation_stage": "server_result", "payload_bytes": "12"},
		{"accounting_version": "1", "observation_stage": "host_result", "payload_bytes": "-1"},
	} {
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindToolCall, Name: "Write", Category: "modify", Attrs: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TokenStats("", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	a := stats.Accounting
	if a.MissingPayload != 2 || a.ByStage["host_result"].Events != 1 || a.ByStage["server_result"].Bytes != 12 || a.Arguments.Bytes != 9 {
		t.Fatalf("accounting: %+v", a)
	}
	events, err := s.ListTokenEvents("", 1, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Attrs["accounting_version"] != "1" {
		t.Fatal("event evidence must survive projection")
	}
}
