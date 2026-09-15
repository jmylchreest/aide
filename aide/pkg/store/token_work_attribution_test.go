package store

import (
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func workReceiptEvent(stage, session string, at time.Time) *observe.Event {
	e := &observe.Event{Kind: observe.KindToolCall, Name: "code_search", Category: "consume", SessionID: session, Timestamp: at, Attrs: map[string]string{
		"accounting_version": "1", "observation_stage": stage, "work_id": "receipt", "work_text_sha256": strings.Repeat("a", 64), "payload_bytes": "0",
	}}
	if stage == "server_result" {
		e.Attrs["work_version"], e.Attrs["work_outcome"], e.Attrs["work_elapsed_ms"] = "1", "returned", "0"
	} else {
		e.Attrs["work_receipt_version"], e.Attrs["host"], e.Attrs["actor_id"], e.Attrs["invocation_id"] = "1", "codex", "actor", "call"
	}
	return e
}

func TestWorkReceiptJoinsAcrossOrderAndDateBounds(t *testing.T) {
	for _, host := range []string{"codex", "claude-code", "opencode"} {
		for _, hostFirst := range []bool{false, true} {
			t.Run(host+map[bool]string{true: "/host-first", false: "/server-first"}[hostFirst], func(t *testing.T) {
				s := retrievalStore(t)
				at := time.Now().UTC()
				server := workReceiptEvent("server_result", "", at)
				receipt := workReceiptEvent("host_result", "s", at.Add(time.Hour))
				receipt.Attrs["host"] = host
				if hostFirst {
					addRetrieval(t, s, receipt)
					addRetrieval(t, s, server)
				} else {
					addRetrieval(t, s, server)
					addRetrieval(t, s, receipt)
				}
				stats, err := s.TokenStats("s", at, at)
				if err != nil {
					t.Fatal(err)
				}
				w := stats.Accounting.Work
				if w.Calls != 1 || w.UnassignedSessions != 0 || w.ReturnedText.Events != 1 {
					t.Fatalf("joined work: %+v", w)
				}
				if stats.EventCount != 1 {
					t.Fatalf("joined server missing from selection: %d", stats.EventCount)
				}
				if q := stats.Accounting.ByStage["server_result"]; q == nil || q.Events != 1 || stats.Accounting.MissingIdentity != 0 {
					t.Fatalf("joined stage/identity: %+v", stats.Accounting)
				}
				if len(stats.Accounting.Activity.Buckets) != 1 || stats.Accounting.Activity.Buckets[0].ByStage["server_result"].Events != 1 {
					t.Fatal("joined activity missing")
				}
				listed, err := s.ListTokenEvents("s", 10, at, at)
				if err != nil || len(listed) != 1 || listed[0].Attrs["work_attribution"] != "receipt" || listed[0].Attrs["host"] != host {
					t.Fatalf("joined details: %+v %v", listed, err)
				}
				raw, err := s.ListObserveEvents(ObserveFilter{})
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range raw {
					if e.Attrs["observation_stage"] == "server_result" && (e.SessionID != "" || e.Attrs["host"] != "") {
						t.Fatal("stored evidence mutated")
					}
				}
				outside, _ := s.TokenStats("s", at.Add(time.Minute), time.Time{})
				if outside.Accounting.Work.Calls != 0 {
					t.Fatal("used receipt date instead of operation date")
				}
			})
		}
	}
}

func TestWorkReceiptAmbiguityAndMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		alter    func(*observe.Event, *observe.Event) []*observe.Event
		assigned bool
	}{
		{"identical-host-replay", func(s, h *observe.Event) []*observe.Event {
			eventCopy := *h
			eventCopy.ID = ""
			return []*observe.Event{&eventCopy}
		}, true},
		{"wrong-id", func(s, h *observe.Event) []*observe.Event { h.Attrs["work_id"] = "other"; return nil }, false},
		{"wrong-hash", func(s, h *observe.Event) []*observe.Event {
			h.Attrs["work_text_sha256"] = strings.Repeat("b", 64)
			return nil
		}, false},
		{"wrong-tool", func(s, h *observe.Event) []*observe.Event { h.Name = "memory_list"; return nil }, false},
		{"missing-actor", func(s, h *observe.Event) []*observe.Event { delete(h.Attrs, "actor_id"); return nil }, false},
		{"missing-invocation", func(s, h *observe.Event) []*observe.Event { delete(h.Attrs, "invocation_id"); return nil }, false},
		{"missing-host", func(s, h *observe.Event) []*observe.Event { delete(h.Attrs, "host"); return nil }, false},
		{"missing-session", func(s, h *observe.Event) []*observe.Event { h.SessionID = ""; return nil }, false},
		{"old-host", func(s, h *observe.Event) []*observe.Event { delete(h.Attrs, "work_receipt_version"); return nil }, false},
		{"old-server", func(s, h *observe.Event) []*observe.Event { delete(s.Attrs, "work_id"); return nil }, false},
		{"malformed-hash", func(s, h *observe.Event) []*observe.Event {
			s.Attrs["work_text_sha256"] = "bad"
			h.Attrs["work_text_sha256"] = "bad"
			return nil
		}, false},
		{"oversized-id", func(s, h *observe.Event) []*observe.Event {
			s.Attrs["work_id"] = strings.Repeat("x", 129)
			h.Attrs["work_id"] = s.Attrs["work_id"]
			return nil
		}, false},
		{"duplicate-server-id", func(s, h *observe.Event) []*observe.Event {
			eventCopy := *s
			eventCopy.ID = ""
			return []*observe.Event{&eventCopy}
		}, false},
		{"conflicting-direct-session", func(s, h *observe.Event) []*observe.Event { s.SessionID = "other"; return nil }, false},
		{"conflicting-session", func(s, h *observe.Event) []*observe.Event {
			other := workReceiptEvent("host_result", "other", h.Timestamp)
			return []*observe.Event{other}
		}, false},
		{"conflicting-invocation", func(s, h *observe.Event) []*observe.Event {
			other := workReceiptEvent("host_result", "s", h.Timestamp)
			other.Attrs["invocation_id"] = "different"
			return []*observe.Event{other}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := retrievalStore(t)
			at := time.Now().UTC()
			server := workReceiptEvent("server_result", "", at)
			host := workReceiptEvent("host_result", "s", at)
			extra := tc.alter(server, host)
			addRetrieval(t, s, server)
			addRetrieval(t, s, host)
			for _, e := range extra {
				addRetrieval(t, s, e)
			}
			stats, err := s.TokenStats("s", time.Time{}, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.assigned {
				want = 1
			}
			if stats.Accounting.Work.Calls != want {
				t.Fatalf("work: %+v", stats.Accounting.Work)
			}
			all, _ := s.TokenStats("", time.Time{}, time.Time{})
			if !tc.assigned && all.Accounting.Work.UnassignedSessions != all.Accounting.Work.Calls {
				t.Fatalf("ambiguous assignment: %+v", all.Accounting.Work)
			}
		})
	}
}

func TestWorkReceiptInvocationCannotClaimDifferentOperations(t *testing.T) {
	for _, ids := range [][]string{{"first", "second"}, {"second", "first"}, {"first", "second", "third"}} {
		t.Run(strings.Join(ids, "/"), func(t *testing.T) {
			s := retrievalStore(t)
			at := time.Now().UTC()
			for _, id := range ids {
				server := workReceiptEvent("server_result", "", at)
				host := workReceiptEvent("host_result", "s", at.Add(time.Hour))
				server.Attrs["work_id"], host.Attrs["work_id"] = id, id
				addRetrieval(t, s, server)
				addRetrieval(t, s, host)
				// Exact replays must not add another contradictory observation.
				replay := workReceiptEvent("host_result", "s", host.Timestamp)
				replay.Attrs["work_id"] = id
				addRetrieval(t, s, replay)
			}
			stats, err := s.TokenStats("s", at, at)
			if err != nil {
				t.Fatal(err)
			}
			if stats.Accounting.Work.Calls != 0 {
				t.Fatal("one invocation claimed multiple operations")
			}
			all, _ := s.TokenStats("", at, at)
			if all.Accounting.Work.Calls != len(ids) || all.Accounting.Work.UnassignedSessions != len(ids) {
				t.Fatalf("ambiguous work lost: %+v", all.Accounting.Work)
			}
			raw, err := s.ListObserveEvents(ObserveFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) != 2*len(ids) {
				t.Fatalf("receipt replays not deduplicated: %d", len(raw))
			}
		})
	}
}
