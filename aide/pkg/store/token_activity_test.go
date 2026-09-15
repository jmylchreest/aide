package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestTokenActivityUsesAllSelectedEventsAndPreservesUnknown(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		if err := s.AddObserveEvent(&observe.Event{Kind: observe.KindToolCall, Name: "Read", Category: "consume", Subtype: "file", SessionID: "selected", Timestamp: start.Add(time.Duration(i) * time.Hour), Attrs: map[string]string{"accounting_version": "1", "observation_stage": "host_result", "payload_bytes": "6", "invocation_id": fmt.Sprint(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []*observe.Event{
		{Kind: observe.KindToolCall, Name: "Bash", Category: "execute", SessionID: "selected", Timestamp: start, Attrs: map[string]string{"accounting_version": "1", "observation_stage": "host_result", "payload_bytes": "0"}},
		{Kind: observe.KindToolCall, Name: "Read", Category: "consume", SessionID: "selected", Timestamp: start},
		{Kind: observe.KindToolCall, Name: "Read", Category: "consume", SessionID: "other", Timestamp: start, Tokens: 99999},
	} {
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TokenStats("selected", start, start.Add(301*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	a := stats.Accounting.Activity
	if a == nil || len(a.Buckets) > 64 {
		t.Fatalf("unbounded or missing trend: %+v", a)
	}
	var bytes, tokens int64
	var measured, unmeasured int
	for i, b := range a.Buckets {
		if i > 0 && !b.Start.After(a.Buckets[i-1].Start) {
			t.Fatal("trend not sorted")
		}
		if q := b.ByStage["host_result"]; q != nil {
			bytes += q.Bytes
			tokens += q.EstimatedTokens
			measured += q.Events
		}
		unmeasured += b.Unmeasured
	}
	if bytes != 1800 || tokens != 600 || measured != 301 || unmeasured != 1 {
		t.Fatalf("lost/crossed/padded evidence: %d %d %d %d", bytes, tokens, measured, unmeasured)
	}
	filtered, err := s.TokenStats("selected", start.Add(299*time.Hour), start.Add(300*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Accounting.ByStage["host_result"].Events != 1 || len(filtered.Accounting.Activity.Buckets) != 1 {
		t.Fatal("date filter not applied to trend")
	}
}
