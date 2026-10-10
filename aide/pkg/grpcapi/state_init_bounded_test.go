package grpcapi_test

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func boundedStateClient(t *testing.T) (*grpcapi.Client, *store.BoltStore) {
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

func TestStateInitBoundedAdapterRoundTripCapAndReplay(t *testing.T) {
	client, st := boundedStateClient(t)
	remote := adapter.NewStoreAdapter(client)
	proposed := &memory.State{Key: "agent:queue:item", Agent: "queue", Value: "first", UpdatedAt: time.Now().Add(-time.Hour)}
	got, created, err := remote.InitStateBounded(proposed, 1)
	if err != nil || !created || got == nil || got.Key != proposed.Key || got.Agent != proposed.Agent || got.Value != proposed.Value || !got.UpdatedAt.Equal(proposed.UpdatedAt) {
		t.Fatalf("initial: %+v %v %v", got, created, err)
	}
	replay := *proposed
	replay.Value = "replacement"
	got, created, err = remote.InitStateBounded(&replay, 1)
	if err != nil || created || got == nil || got.Value != proposed.Value || !got.UpdatedAt.Equal(proposed.UpdatedAt) {
		t.Fatalf("replay changed state at capacity: %+v %v %v", got, created, err)
	}
	got, created, err = remote.InitStateBounded(&memory.State{Key: "agent:queue:second", Agent: "queue", Value: "overflow"}, 1)
	if status.Code(err) != codes.ResourceExhausted || got != nil || created {
		t.Fatalf("capacity response: %+v %v %v", got, created, err)
	}
	other := &memory.State{Key: "agent:queue:child:item", Agent: "queue:child", Value: "independent"}
	got, created, err = remote.InitStateBounded(other, 1)
	if err != nil || !created || got == nil || got.Key != other.Key || got.Agent != other.Agent {
		t.Fatalf("namespace isolation: %+v %v %v", got, created, err)
	}
	all, err := st.ListState("")
	if err != nil || len(all) != 2 {
		t.Fatalf("mutated or double-prefixed keys: %+v %v", all, err)
	}
}

func TestStateInitBoundedRPCValidationWithoutMutation(t *testing.T) {
	client, st := boundedStateClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, req := range []*grpcapi.StateBoundedInitRequest{
		{}, {State: &grpcapi.StateSetRequest{AgentId: "queue"}, MaxAgentEntries: 1},
		{State: &grpcapi.StateSetRequest{Key: "key"}, MaxAgentEntries: 1},
		{State: &grpcapi.StateSetRequest{Key: "key", AgentId: " "}, MaxAgentEntries: 1},
		{State: &grpcapi.StateSetRequest{Key: "key", AgentId: "queue"}},
		{State: &grpcapi.StateSetRequest{Key: "key", AgentId: "queue"}, MaxAgentEntries: memory.MaxStateAgentEntries + 1},
		{State: &grpcapi.StateSetRequest{Key: "key", AgentId: "queue"}, MaxAgentEntries: ^uint32(0)},
	} {
		got, err := client.State.InitBounded(ctx, req)
		if status.Code(err) != codes.InvalidArgument || got != nil {
			t.Errorf("invalid request accepted: %v => %+v %v", req, got, err)
		}
	}
	remote := adapter.NewStoreAdapter(client)
	for _, proposed := range []*memory.State{nil, {Agent: "queue"}, {Key: "key"}, {Key: "agent:other:key", Agent: "queue"}} {
		if got, fresh, err := remote.InitStateBounded(proposed, 1); err == nil || got != nil || fresh {
			t.Errorf("invalid adapter state accepted: %+v %v %v", got, fresh, err)
		}
	}
	for _, limit := range []int{-1, 0, memory.MaxStateAgentEntries + 1} {
		if got, fresh, err := remote.InitStateBounded(&memory.State{Key: "agent:queue:key", Agent: "queue"}, limit); err == nil || got != nil || fresh {
			t.Errorf("invalid adapter cap accepted: %+v %v %v", got, fresh, err)
		}
	}
	all, err := st.ListState("")
	if err != nil || len(all) != 0 {
		t.Fatalf("invalid calls mutated states: %+v %v", all, err)
	}
}

type boundedLegacyStateServer struct {
	grpcapi.UnimplementedStateServiceServer
	inits atomic.Int32
	sets  atomic.Int32
}

func (s *boundedLegacyStateServer) Init(context.Context, *grpcapi.StateSetRequest) (*grpcapi.StateSetResponse, error) {
	s.inits.Add(1)
	return &grpcapi.StateSetResponse{}, nil
}
func (s *boundedLegacyStateServer) Set(context.Context, *grpcapi.StateSetRequest) (*grpcapi.StateSetResponse, error) {
	s.sets.Add(1)
	return &grpcapi.StateSetResponse{}, nil
}

func TestStateInitBoundedOldDaemonNeverFallsBackToInitOrSet(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	srv := grpc.NewServer()
	legacy := &boundedLegacyStateServer{}
	desc := grpcapi.StateService_ServiceDesc
	desc.Methods = nil
	for _, method := range grpcapi.StateService_ServiceDesc.Methods {
		if method.MethodName != "InitBounded" {
			desc.Methods = append(desc.Methods, method)
		}
	}
	srv.RegisterService(&desc, legacy)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///legacy-bounded", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	remote := adapter.NewStoreAdapter(&grpcapi.Client{State: grpcapi.NewStateServiceClient(conn)})
	got, created, err := remote.InitStateBounded(&memory.State{Key: "agent:queue:item", Agent: "queue", Value: "new"}, 1)
	if status.Code(err) != codes.Unimplemented || got != nil || created {
		t.Fatalf("old daemon: %+v %v %v", got, created, err)
	}
	if legacy.inits.Load() != 0 || legacy.sets.Load() != 0 {
		t.Fatalf("unsafe fallback: Init=%d Set=%d", legacy.inits.Load(), legacy.sets.Load())
	}
}
