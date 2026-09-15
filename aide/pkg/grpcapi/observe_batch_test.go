package grpcapi

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/eventbus"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func batchUsageRequest(id string, at time.Time, input string) *ObserveRecordRequest {
	return &ObserveRecordRequest{Kind: "session", Name: "model_usage", SessionId: "batch-session", Timestamp: timestamppb.New(at), Attrs: map[string]string{
		"model_usage_version": "1", "host": "codex", "usage_source": "codex.token_usage_record.v1", "usage_id": id,
		"usage_time_basis": "source", "usage_source_time": at.Format(time.RFC3339Nano), "input_tokens": input,
	}}
}

func batchObserveService(t *testing.T) (*observeServiceImpl, *store.BoltStore, <-chan *observe.Event) {
	t.Helper()
	st, err := store.NewBoltStore(filepath.Join(t.TempDir(), "observe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bus := eventbus.New[*observe.Event](512)
	events, unsubscribe := bus.Subscribe(context.Background(), nil)
	t.Cleanup(unsubscribe)
	return &observeServiceImpl{store: st, bus: bus}, st, events
}

func requireObserveNotifications(t *testing.T, events <-chan *observe.Event, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-events:
		default:
			t.Fatalf("missing notification %d of %d", i+1, count)
		}
	}
	select {
	case e := <-events:
		t.Fatalf("unexpected replay notification: %+v", e)
	default:
	}
}

func TestObserveBatchBroadcastsChangesAndAcknowledgesReplay(t *testing.T) {
	svc, st, events := batchObserveService(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	first, err := svc.RecordBatch(ctx, &ObserveBatchRecordRequest{Events: []*ObserveRecordRequest{batchUsageRequest("a", at, "10"), batchUsageRequest("b", at, "20")}})
	if err != nil || first == nil || len(first.Results) != 2 {
		t.Fatalf("initial batch: %+v %v", first, err)
	}
	for _, result := range first.Results {
		if !result.Changed || result.Id == "" {
			t.Fatalf("new event not acknowledged: %+v", result)
		}
	}
	requireObserveNotifications(t, events, 2)

	requests := make([]*ObserveRecordRequest, 256)
	for i := range requests {
		requests[i] = batchUsageRequest("a", at, "10")
	}
	replay, err := svc.RecordBatch(ctx, &ObserveBatchRecordRequest{Events: requests})
	if err != nil || replay == nil || len(replay.Results) != 256 {
		t.Fatalf("replay batch: %+v %v", replay, err)
	}
	for _, result := range replay.Results {
		if result.Changed || result.Id != first.Results[0].Id {
			t.Fatalf("duplicate mutated: %+v", result)
		}
	}
	requireObserveNotifications(t, events, 0)

	changed, err := svc.RecordBatch(ctx, &ObserveBatchRecordRequest{Events: []*ObserveRecordRequest{
		batchUsageRequest("a", at.Add(-time.Hour), "10"), batchUsageRequest("b", at, "21"), batchUsageRequest("a", at, "10"),
	}})
	if err != nil || changed == nil || len(changed.Results) != 3 {
		t.Fatalf("changed batch: %+v %v", changed, err)
	}
	if !changed.Results[0].Changed || changed.Results[0].Id != first.Results[0].Id || !changed.Results[1].Changed || changed.Results[1].Id == first.Results[1].Id || changed.Results[2].Changed {
		t.Fatalf("earlier evidence/conflict/replay outcomes: %+v", changed.Results)
	}
	requireObserveNotifications(t, events, 2)
	stored, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil || len(stored) != 3 {
		t.Fatalf("stored evidence: %d %v", len(stored), err)
	}
	stats, err := st.TokenStats("batch-session", time.Time{}, time.Time{})
	if err != nil || stats.Accounting.ModelUsage.Observations != 1 || stats.Accounting.ModelUsage.Conflicts != 1 {
		t.Fatalf("conflicting evidence lost: %+v %v", stats, err)
	}
	if stats.TotalSaved != 0 {
		t.Fatalf("accounting batching invented savings: %d", stats.TotalSaved)
	}
}

func TestObserveSingleBroadcastsOnlyChanges(t *testing.T) {
	svc, _, events := batchObserveService(t)
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	first, err := svc.RecordEvent(context.Background(), batchUsageRequest("single", at, "10"))
	if err != nil || first == nil || !first.Changed {
		t.Fatalf("initial single: %+v %v", first, err)
	}
	requireObserveNotifications(t, events, 1)
	replay, err := svc.RecordEvent(context.Background(), batchUsageRequest("single", at, "10"))
	if err != nil || replay == nil || replay.Changed || replay.Id != first.Id {
		t.Fatalf("single replay: %+v %v", replay, err)
	}
	requireObserveNotifications(t, events, 0)
}

func TestObserveConcurrentBatchReplaysPublishOneChange(t *testing.T) {
	svc, st, events := batchObserveService(t)
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	const writers = 24
	results := make(chan *ObserveRecordResponse, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := svc.RecordBatch(context.Background(), &ObserveBatchRecordRequest{Events: []*ObserveRecordRequest{batchUsageRequest("concurrent", at, "10")}})
			if err != nil || got == nil || len(got.Results) != 1 {
				t.Errorf("concurrent batch: %+v %v", got, err)
				return
			}
			results <- got.Results[0]
		}()
	}
	wg.Wait()
	close(results)
	var id string
	changed, received := 0, 0
	for result := range results {
		received++
		if id == "" {
			id = result.Id
		}
		if result.Id != id {
			t.Errorf("concurrent replay changed ID: %s != %s", result.Id, id)
		}
		if result.Changed {
			changed++
		}
	}
	if changed != 1 || received != writers {
		t.Fatalf("concurrent acknowledgements: changed=%d received=%d", changed, received)
	}
	requireObserveNotifications(t, events, 1)
	stored, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil || len(stored) != 1 {
		t.Fatalf("concurrent stored evidence: %d %v", len(stored), err)
	}
}

func TestObserveBatchRejectsWholeInvalidBatchBeforeWrites(t *testing.T) {
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	oversized := make([]*ObserveRecordRequest, 257)
	for i := range oversized {
		oversized[i] = batchUsageRequest(fmt.Sprint(i), at, "10")
	}
	for name, request := range map[string]*ObserveBatchRecordRequest{
		"nil batch":         nil,
		"nil event":         {Events: []*ObserveRecordRequest{batchUsageRequest("valid", at, "10"), nil}},
		"invalid timestamp": {Events: []*ObserveRecordRequest{batchUsageRequest("valid", at, "10"), {Kind: "session", Timestamp: &timestamppb.Timestamp{Seconds: 253402300800}}}},
		"zero timestamp":    {Events: []*ObserveRecordRequest{batchUsageRequest("valid", at, "10"), {Kind: "session", Timestamp: timestamppb.New(time.Time{})}}},
		"oversized":         {Events: oversized},
	} {
		t.Run(name, func(t *testing.T) {
			svc, st, events := batchObserveService(t)
			got, err := svc.RecordBatch(context.Background(), request)
			if status.Code(err) != codes.InvalidArgument || got != nil {
				t.Fatalf("invalid batch accepted: %+v %v", got, err)
			}
			stored, err := st.ListObserveEvents(store.ObserveFilter{})
			if err != nil || len(stored) != 0 {
				t.Fatalf("partial invalid batch persisted: %d %v", len(stored), err)
			}
			requireObserveNotifications(t, events, 0)
		})
	}
}
