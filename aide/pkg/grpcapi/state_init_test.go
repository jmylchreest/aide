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

func TestStateInitAdapterRoundTrip(t *testing.T) {
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
	remote := adapter.NewStoreAdapter(client)
	proposed := &memory.State{Key: "agent:worker:context", Agent: "worker", Value: "initial", UpdatedAt: time.Now().Add(-time.Hour)}
	got, created, err := remote.InitState(proposed)
	if err != nil || !created || got == nil || got.Key != proposed.Key || got.Agent != proposed.Agent || got.Value != proposed.Value || !got.UpdatedAt.Equal(proposed.UpdatedAt) {
		t.Fatalf("initial state %+v created=%v err=%v", got, created, err)
	}
	if err := st.SetState(&memory.State{Key: proposed.Key, Agent: proposed.Agent, Value: "reset"}); err != nil {
		t.Fatal(err)
	}
	got, created, err = remote.InitState(proposed)
	if err != nil || created || got == nil || got.Value != "reset" {
		t.Fatalf("replay %+v created=%v err=%v", got, created, err)
	}
	all, err := st.ListState("")
	if err != nil || len(all) != 1 {
		t.Fatalf("agent key duplicated: %+v %v", all, err)
	}
}

type legacyStateServer struct {
	grpcapi.UnimplementedStateServiceServer
	sets atomic.Int32
}

func (s *legacyStateServer) Set(context.Context, *grpcapi.StateSetRequest) (*grpcapi.StateSetResponse, error) {
	s.sets.Add(1)
	return &grpcapi.StateSetResponse{}, nil
}

func TestStateInitOldDaemonFailsWithoutSetFallback(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	srv := grpc.NewServer()
	legacy := &legacyStateServer{}
	// Mimic the old service descriptor, where Init is unknown to the daemon.
	desc := grpcapi.StateService_ServiceDesc
	desc.Methods = nil
	for _, method := range grpcapi.StateService_ServiceDesc.Methods {
		if method.MethodName != "Init" {
			desc.Methods = append(desc.Methods, method)
		}
	}
	srv.RegisterService(&desc, legacy)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///legacy", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	remote := adapter.NewStoreAdapter(&grpcapi.Client{State: grpcapi.NewStateServiceClient(conn)})
	got, created, err := remote.InitState(&memory.State{Key: "context", Value: "startup"})
	if status.Code(err) != codes.Unimplemented || got != nil || created {
		t.Fatalf("got %+v created=%v err=%v", got, created, err)
	}
	if legacy.sets.Load() != 0 {
		t.Fatal("init fell back to unsafe Set")
	}
}
