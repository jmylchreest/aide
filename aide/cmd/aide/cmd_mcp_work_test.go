package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPWorkRecordsReportedOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name             string
		result           *mcp.CallToolResult
		err              error
		outcome, payload string
	}{
		{"returned", textResult("é"), nil, "returned", "2"},
		{"empty", textResult(""), nil, "returned", "0"},
		{"reported_error", errorResult("failed"), nil, "reported_error", "13"},
		{"go_error", textResult("partial"), errors.New("failed"), "reported_error", "7"},
		{"unknown", nil, nil, "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			observe.SetDefault(store.NewObserveSink(st))
			defer observe.SetDefault(nil)
			handler := newMCPServer(nil).toolObserveMiddleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { return tc.result, tc.err })
			_, _ = handler(context.Background(), "tools/call", &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "code_search"}})
			events, err := st.ListObserveEvents(store.ObserveFilter{})
			if err != nil || len(events) != 1 {
				t.Fatalf("events: %v %v", events, err)
			}
			e := events[0]
			if e.Attrs["work_version"] != "1" || e.Attrs["work_outcome"] != tc.outcome || e.Attrs["payload_bytes"] != tc.payload {
				t.Fatalf("work evidence: %+v", e)
			}
			elapsed, err := strconv.ParseInt(e.Attrs["work_elapsed_ms"], 10, 64)
			if err != nil || elapsed < 0 {
				t.Fatalf("duration unknown: %+v", e)
			}
			if (e.Error != "") != (tc.outcome == "reported_error") {
				t.Fatalf("error evidence: %+v", e)
			}
		})
	}
}
