package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestFormatTokenWorkKeepsUnknownDistinctFromZero(t *testing.T) {
	if got := formatTokenWork(nil); !strings.Contains(got, "unavailable") {
		t.Fatalf("old server: %s", got)
	}
	if got := formatTokenWork(&memory.TokenWork{Version: 2}); !strings.Contains(got, "unavailable") {
		t.Fatalf("unsupported version: %s", got)
	}
	if got := formatTokenWork(memory.NewTokenWork()); !strings.Contains(got, "No eligible service calls") {
		t.Fatalf("empty selection: %s", got)
	}
	w := memory.NewTokenWork()
	w.TokenWorkCounters = memory.TokenWorkCounters{Calls: 2, Returned: 1, UnknownOutcomes: 1, MeasuredDurations: 1, MissingDurations: 1, ReturnedText: memory.TokenQuantity{Events: 1}, MissingPayload: 1, UnassignedSessions: 1}
	w.ByTool["z_unknown"] = &memory.TokenWorkCounters{Calls: 1, UnknownOutcomes: 1, MissingDurations: 1, MissingPayload: 1, UnassignedSessions: 1}
	w.ByTool["a_zero"] = &memory.TokenWorkCounters{Calls: 1, Returned: 1, MeasuredDurations: 1, ReturnedText: memory.TokenQuantity{Events: 1}}
	got := formatTokenWork(w)
	for _, want := range []string{"0 ms [1/1]", "0 B [1/1]", "unknown [0/1]", "Unassigned sessions: 1", "not task success", "can overlap", "not CPU", "not additional text"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Index(got, "a_zero") > strings.Index(got, "z_unknown") {
		t.Fatalf("groups not sorted: %s", got)
	}
}

func TestTokenWorkCLIIsDetailsOnlyAndAvailableInJSON(t *testing.T) {
	dbPath, _ := newShareProject(t)
	withBackend(t, dbPath, func(b *Backend) {
		if err := b.Store().AddObserveEvent(&observe.Event{
			Kind: observe.KindToolCall, Name: "code_outline",
			Attrs: map[string]string{"accounting_version": "1", "observation_stage": "server_result", "work_version": "1", "work_outcome": "returned", "work_elapsed_ms": "0", "payload_bytes": "0"},
		}); err != nil {
			t.Fatal(err)
		}
	})
	run := func(args ...string) string {
		t.Helper()
		return captureStdout(t, func() {
			if err := cmdTokenStats(dbPath, args); err != nil {
				t.Fatal(err)
			}
		})
	}
	if out := run(); strings.Contains(out, "Aide work (recorded MCP") {
		t.Fatalf("work details leaked into overview: %s", out)
	}
	if out := run("--details"); !strings.Contains(out, "Aide work (recorded MCP") || !strings.Contains(out, "0 ms [1/1]") {
		t.Fatalf("missing work details: %s", out)
	}
	var stats memory.TokenStats
	if err := json.Unmarshal([]byte(run("--json")), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Accounting == nil || stats.Accounting.Work == nil || stats.Accounting.Work.Calls != 1 {
		t.Fatalf("missing structured work: %+v", stats.Accounting)
	}
}
