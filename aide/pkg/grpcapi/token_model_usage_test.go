package grpcapi

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestModelUsageRoundTripAndOldServer(t *testing.T) {
	if TokenAccountingFromProto(&TokenAccounting{}).ModelUsage != nil {
		t.Fatal("old server invented usage")
	}
	for _, u := range []*memory.TokenModelUsage{nil, {Version: 1, BySource: []*memory.ModelUsageSource{}}, {Version: 1, Observations: 2, Invalid: 1, Conflicts: 1, BySource: []*memory.ModelUsageSource{{Host: "codex", Source: "codex.token_usage_record.v1", Model: "test", Observations: 2, SourceTimed: 1, ObservedTimed: 1, Counters: map[string]*memory.ModelUsageCounter{"output_tokens": {Tokens: 0, Observations: 1}}}}}} {
		a := memory.NewTokenAccounting()
		a.ModelUsage = u
		b, err := proto.Marshal(TokenAccountingToProto(a))
		if err != nil {
			t.Fatal(err)
		}
		p := &TokenAccounting{}
		if err := proto.Unmarshal(b, p); err != nil {
			t.Fatal(err)
		}
		if got := TokenAccountingFromProto(p).ModelUsage; !reflect.DeepEqual(got, u) {
			t.Fatalf("got %+v want %+v", got, u)
		}
	}
}

func TestObserveRecordPreservesSourceTimestamp(t *testing.T) {
	st, err := store.NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := &observeServiceImpl{store: st}
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := server.RecordEvent(context.Background(), &ObserveRecordRequest{Kind: "session", Name: "source", Timestamp: timestamppb.New(at)}); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].Timestamp.Equal(at) {
		t.Fatal("timestamp dropped by RPC")
	}
	if _, err := server.RecordEvent(context.Background(), &ObserveRecordRequest{Kind: "session", Name: "invalid", Timestamp: &timestamppb.Timestamp{Seconds: 253402300800}}); err == nil {
		t.Fatal("invalid timestamp accepted")
	}
}
