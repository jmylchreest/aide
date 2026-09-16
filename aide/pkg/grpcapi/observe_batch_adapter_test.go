package grpcapi_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type batchObserveRPCStub struct {
	grpcapi.UnimplementedObserveServiceServer
	mu           sync.Mutex
	batchCalls   int
	singleCalls  int
	requests     []*grpcapi.ObserveRecordRequest
	batchError   codes.Code
	shortResults bool
}

func (s *batchObserveRPCStub) RecordBatch(_ context.Context, req *grpcapi.ObserveBatchRecordRequest) (*grpcapi.ObserveBatchRecordResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchCalls++
	if s.batchError != codes.OK {
		return nil, status.Error(s.batchError, "injected batch failure")
	}
	s.requests = append(s.requests, req.Events...)
	results := make([]*grpcapi.ObserveRecordResponse, len(req.Events))
	for i := range results {
		results[i] = &grpcapi.ObserveRecordResponse{Id: fmt.Sprintf("batch-%d", i), Changed: i%2 == 0}
	}
	if s.shortResults {
		results = results[:len(results)-1]
	}
	return &grpcapi.ObserveBatchRecordResponse{Results: results}, nil
}

func (s *batchObserveRPCStub) RecordEvent(_ context.Context, req *grpcapi.ObserveRecordRequest) (*grpcapi.ObserveRecordResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.singleCalls++
	s.requests = append(s.requests, req)
	// Old daemons acknowledge the ID without a Changed field.
	return &grpcapi.ObserveRecordResponse{Id: fmt.Sprintf("single-%d", s.singleCalls)}, nil
}

func observeBatchStubAdapter(t *testing.T, stub *batchObserveRPCStub, oldDaemon bool) *adapter.StoreAdapter {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	srv := grpc.NewServer()
	desc := grpcapi.ObserveService_ServiceDesc
	if oldDaemon {
		desc.Methods = nil
		for _, method := range grpcapi.ObserveService_ServiceDesc.Methods {
			if method.MethodName != "RecordBatch" {
				desc.Methods = append(desc.Methods, method)
			}
		}
	}
	srv.RegisterService(&desc, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///observe-batch", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return adapter.NewStoreAdapter(&grpcapi.Client{Observe: grpcapi.NewObserveServiceClient(conn)})
}

func adapterUsageEvents() []*observe.Event {
	at := time.Date(2026, 9, 12, 12, 0, 0, 123, time.UTC)
	return []*observe.Event{
		{Kind: observe.KindSession, Name: "model_usage", SessionID: "parent", Timestamp: at, Parent: "child", Attrs: map[string]string{"usage_id": "response-a", "input_tokens": "12"}},
		{Kind: observe.KindSession, Name: "model_usage", SessionID: "parent", Timestamp: at, Attrs: map[string]string{"usage_id": "response-b", "cache_read_input_tokens": "8"}},
	}
}

func TestObserveAdapterUsesOneBatchAndPreservesResponseEvidence(t *testing.T) {
	stub := &batchObserveRPCStub{}
	remote := observeBatchStubAdapter(t, stub, false)
	events := adapterUsageEvents()
	changed, err := remote.AddObserveEvents(events)
	if err != nil || len(changed) != 2 || !changed[0] || changed[1] {
		t.Fatalf("batch results: %v %v", changed, err)
	}
	if events[0].ID != "batch-0" || events[1].ID != "batch-1" {
		t.Fatalf("response IDs not returned: %+v", events)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.batchCalls != 1 || stub.singleCalls != 0 || len(stub.requests) != 2 {
		t.Fatalf("RPC fanout: batch=%d single=%d records=%d", stub.batchCalls, stub.singleCalls, len(stub.requests))
	}
	for i, req := range stub.requests {
		if req.SessionId != events[i].SessionID || req.Parent != events[i].Parent || req.Name != "model_usage" || !req.Timestamp.AsTime().Equal(events[i].Timestamp) || req.Attrs["usage_id"] != events[i].Attrs["usage_id"] {
			t.Fatalf("lost response attribution: %+v", req)
		}
	}
}

func observeBatchClient(t *testing.T) (*grpcapi.Client, *store.BoltStore) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	socket := filepath.Join(dir, "aide.sock")
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := grpcapi.NewServer(st, dbPath, socket, grammar.NewCompositeLoader())
	go func() { _ = srv.Start() }()
	t.Cleanup(srv.Stop)
	client := waitForClient(t, socket)
	t.Cleanup(func() { client.Close() })
	return client, st
}

func TestObserveBatchAdapterRealDaemonRoundTrip(t *testing.T) {
	client, st := observeBatchClient(t)
	remote := adapter.NewStoreAdapter(client)
	events := adapterUsageEvents()
	for _, e := range events {
		e.Attrs["model_usage_version"] = "1"
		e.Attrs["host"] = "codex"
		e.Attrs["usage_source"] = "codex.token_usage_record.v1"
		e.Attrs["usage_time_basis"] = "source"
		e.Attrs["usage_source_time"] = e.Timestamp.Format(time.RFC3339Nano)
	}
	for pass := 0; pass < 2; pass++ {
		changed, err := remote.AddObserveEvents(events)
		if err != nil || len(changed) != 2 {
			t.Fatalf("pass %d: %v %v", pass, changed, err)
		}
		for i, flag := range changed {
			if flag != (pass == 0) || events[i].ID == "" {
				t.Fatalf("pass %d acknowledgement: changed=%v event=%+v", pass, flag, events[i])
			}
		}
	}
	stored, err := st.ListObserveEvents(store.ObserveFilter{SessionID: "parent"})
	if err != nil || len(stored) != 2 {
		t.Fatalf("roundtrip duplicated evidence: %d %v", len(stored), err)
	}
}

func TestObserveAdapterOldDaemonFallsBackToAcknowledgedSingles(t *testing.T) {
	stub := &batchObserveRPCStub{}
	remote := observeBatchStubAdapter(t, stub, true)
	events := adapterUsageEvents()
	changed, err := remote.AddObserveEvents(events)
	if err != nil || len(changed) != len(events) {
		t.Fatalf("old daemon fallback failed: %v %v", changed, err)
	}
	if events[0].ID != "single-1" || events[1].ID != "single-2" {
		t.Fatalf("fallback lost acknowledgements: %+v", events)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.batchCalls != 0 || stub.singleCalls != 2 {
		t.Fatalf("fallback calls: batch=%d single=%d", stub.batchCalls, stub.singleCalls)
	}
}

func TestObserveAdapterDoesNotRetryAmbiguousBatchFailuresAsSingles(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.InvalidArgument} {
		t.Run(code.String(), func(t *testing.T) {
			stub := &batchObserveRPCStub{batchError: code}
			remote := observeBatchStubAdapter(t, stub, false)
			events := adapterUsageEvents()
			_, err := remote.AddObserveEvents(events)
			if status.Code(err) != code {
				t.Fatalf("error changed: %v", err)
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if stub.batchCalls != 1 || stub.singleCalls != 0 {
				t.Fatalf("ambiguous failure retried: batch=%d single=%d", stub.batchCalls, stub.singleCalls)
			}
			for _, e := range events {
				if e.ID != "" {
					t.Fatalf("failure acknowledged unconfirmed evidence: %+v", e)
				}
			}
		})
	}
}

func TestObserveAdapterRejectsIncompleteBatchAcknowledgement(t *testing.T) {
	stub := &batchObserveRPCStub{shortResults: true}
	remote := observeBatchStubAdapter(t, stub, false)
	_, err := remote.AddObserveEvents(adapterUsageEvents())
	if err == nil {
		t.Fatal("incomplete response acknowledged as complete")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.singleCalls != 0 {
		t.Fatalf("malformed acknowledgement retried %d singles", stub.singleCalls)
	}
}

func TestObserveAdapterRejectsInvalidBatchBeforeAnyRPC(t *testing.T) {
	for name, events := range map[string][]*observe.Event{
		"nil entry":         {adapterUsageEvents()[0], nil},
		"invalid timestamp": {adapterUsageEvents()[0], {Kind: observe.KindSession, Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &batchObserveRPCStub{}
			remote := observeBatchStubAdapter(t, stub, true)
			if _, err := remote.AddObserveEvents(events); err == nil {
				t.Fatal("invalid local batch accepted")
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if stub.batchCalls != 0 || stub.singleCalls != 0 {
				t.Fatalf("invalid batch partially submitted: batch=%d single=%d", stub.batchCalls, stub.singleCalls)
			}
		})
	}
}
