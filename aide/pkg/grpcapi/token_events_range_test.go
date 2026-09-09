package grpcapi_test

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
	"strings"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTokenEventsTimeRangeBeforeLimitAndTransport(t *testing.T) {
	dir := t.TempDir()
	dbPath, socket := filepath.Join(dir, "memory.db"), filepath.Join(dir, "aide.sock")
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	at := time.Date(2026, 9, 1, 0, 0, 0, 123456789, time.UTC)
	add := func(e *observe.Event) {
		t.Helper()
		if err := st.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	// More than the normal 4 MiB gRPC receive limit, inserted out of source-time
	// order. A bounded page stays below that limit without raising the ceiling.
	for i := 899; i >= 0; i-- {
		add(&observe.Event{Kind: observe.KindToolCall, Name: "Read", Category: "consume", Subtype: "file", SessionID: "s", Timestamp: at.Add(time.Duration(i) * time.Second), Attrs: map[string]string{"fixture": strings.Repeat("x", 8192)}})
	}
	add(&observe.Event{Kind: observe.KindToolCall, Name: "Read", Category: "consume", Subtype: "file", SessionID: "other", Timestamp: at.Add(150 * time.Second)})
	// A later host receipt must still attribute an earlier server result even
	// though the receipt itself lies outside the requested date window.
	server := &observe.Event{Kind: observe.KindToolCall, Name: "code_search", Category: "consume", Timestamp: at.Add(150 * time.Second), Attrs: map[string]string{"accounting_version": "1", "observation_stage": "server_result", "work_id": "receipt", "work_text_sha256": strings.Repeat("a", 64), "payload_bytes": "0", "work_version": "1", "work_outcome": "returned", "work_elapsed_ms": "0"}}
	add(server)
	add(&observe.Event{Kind: observe.KindToolCall, Name: "code_search", Category: "consume", SessionID: "s", Timestamp: at.Add(time.Hour), Attrs: map[string]string{"accounting_version": "1", "observation_stage": "host_result", "work_id": "receipt", "work_text_sha256": strings.Repeat("a", 64), "payload_bytes": "0", "work_receipt_version": "1", "host": "codex", "actor_id": "actor", "invocation_id": "call"}})
	rawBefore, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil {
		t.Fatal(err)
	}
	srv := grpcapi.NewServer(st, dbPath, socket, grammar.NewCompositeLoader())
	go func() { _ = srv.Start() }()
	t.Cleanup(srv.Stop)
	client := waitForClient(t, socket)
	t.Cleanup(func() { client.Close() })
	remote := adapter.NewStoreAdapter(client)
	if _, err := client.Token.ListTokenEvents(context.Background(), &grpcapi.TokenEventListRequest{SessionId: "s"}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("fixture must exceed unbounded transport limit: %v", err)
	}
	for _, tc := range []struct {
		name, session string
		limit         int
		since, until  time.Time
	}{
		{"both_bounds", "s", 200, at.Add(100 * time.Second), at.Add(399 * time.Second)},
		{"since_only", "s", 5, at.Add(700 * time.Second), time.Time{}},
		{"until_only", "s", 5, time.Time{}, at.Add(300 * time.Second)},
		{"inclusive_equal_and_receipt_join", "s", 5, at.Add(150 * time.Second), at.Add(150 * time.Second)},
		{"all_sessions", "", 5, at.Add(150 * time.Second), at.Add(150 * time.Second)},
		{"empty", "s", 5, at.Add(2 * time.Hour), time.Time{}},
		{"no_time_filter", "s", 5, time.Time{}, time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := st.ListTokenEvents(tc.session, tc.limit, tc.since, tc.until)
			if err != nil {
				t.Fatal(err)
			}
			got, err := remote.ListTokenEvents(tc.session, tc.limit, tc.since, tc.until)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("got %d events want %d", len(got), len(want))
			}
			for i := range want {
				if !got[i].Timestamp.Equal(want[i].Timestamp) {
					t.Fatal("timestamp precision lost")
				}
				got[i].Timestamp = want[i].Timestamp
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("event %d differs: %+v != %+v", i, got[i], want[i])
				}
			}
			if tc.name == "both_bounds" && len(got) != 200 {
				t.Fatal("fixture did not exercise requested 200-event page")
			}
			if tc.name == "inclusive_equal_and_receipt_join" && (len(got) != 2 || (got[0].Attrs["work_attribution"] != "receipt" && got[1].Attrs["work_attribution"] != "receipt")) {
				t.Fatal("out-of-range receipt attribution lost")
			}
		})
	}
	for _, req := range []*grpcapi.TokenEventListRequest{
		{Limit: 100001},
		{Since: &timestamppb.Timestamp{Seconds: 253402300800}},
		{Since: timestamppb.New(at.Add(time.Second)), Until: timestamppb.New(at)},
	} {
		if _, err := client.Token.ListTokenEvents(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid request accepted: %+v %v", req, err)
		}
	}
	rawAfter, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil || !reflect.DeepEqual(rawBefore, rawAfter) {
		t.Fatal("query mutated observation history")
	}
}

type oldTokenEventServer struct {
	grpcapi.UnimplementedTokenServiceServer
}

func (*oldTokenEventServer) ListTokenEvents(context.Context, *grpcapi.TokenEventListRequest) (*grpcapi.TokenEventListResponse, error) {
	return &grpcapi.TokenEventListResponse{}, nil
}

func TestTokenEventsOldDaemonCannotSilentlyIgnoreTimeBounds(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	srv := grpc.NewServer()
	grpcapi.RegisterTokenServiceServer(srv, &oldTokenEventServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///legacy", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	remote := adapter.NewStoreAdapter(&grpcapi.Client{Token: grpcapi.NewTokenServiceClient(conn)})
	for _, limit := range []int{grpcapi.MaxTokenEventListLimit + 1, int(^uint(0) >> 1)} {
		if _, err := remote.ListTokenEvents("s", limit, time.Time{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "limit must not exceed") {
			t.Fatalf("positive limit wrapped or reached old daemon: %d %v", limit, err)
		}
	}
	if _, err := remote.ListTokenEvents("s", 200, time.Now(), time.Time{}); err == nil || !strings.Contains(err.Error(), "rebuild and restart") {
		t.Fatalf("old daemon silently ignored time bounds: %v", err)
	}
	if _, err := remote.ListTokenEvents("s", 200, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("unbounded legacy query unnecessarily refused: %v", err)
	}
}
