package main

import (
	"context"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMCPAccountingMeasuresRenderedResult(t *testing.T) {
	st, err := store.NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	observe.SetDefault(store.NewObserveSink(st))
	defer observe.SetDefault(nil)
	server := newMCPServer(nil)
	handler := server.toolObserveMiddleware()(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		observe.FromContext(ctx).Tokens(999)
		return textResult("header\né"), nil
	})
	_, err = handler(context.Background(), "tools/call", &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "code_read_symbol"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %v %v", events, err)
	}
	e := events[0]
	if e.Attrs["payload_bytes"] != "9" || e.Attrs["observation_stage"] != "server_result" || e.Tokens != 3 {
		t.Fatalf("rendered result accounting: %+v", e)
	}
	if e.SessionID != "" {
		t.Fatal("must not guess a host session")
	}
}

func TestClientObservationsAndTokenAccountingRoundTrip(t *testing.T) {
	dbPath := electionRoot(t)
	primary, stopPrimary := mustJoin(t, dbPath)
	defer stopPrimary()
	// A real client is a separate process and starts without a local sink.
	observe.SetDefault(nil)
	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()
	defer observe.SetDefault(nil)
	span := observe.Start("code_search", observe.KindToolCall)
	span.Category("navigate").Session("s").Attr("accounting_version", "1").Attr("observation_stage", "server_result").Attr("payload_bytes", "12").Attr("start_line", "4")
	span.End()
	transformation := observe.Start("output-transform", observe.KindHook)
	transformation.Category("transform").Session("s").Attr("accounting_version", "1").Attr("observation_stage", "adapter_change").Attr("host", "opencode").Attr("actor_id", "s").Attr("invocation_id", "call").Attr("context_status", "active").Attr("context_epoch", "epoch").Attr("before_bytes", "30").Attr("after_bytes", "90")
	transformation.End()
	events, err := primary.store().ListObserveEvents(store.ObserveFilter{Name: "code_search"})
	if err != nil || len(events) != 1 {
		t.Fatalf("client observation lost: %v, %v", events, err)
	}
	direct, err := primary.store().TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := client.store().TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if remote.Accounting == nil || !reflect.DeepEqual(direct, remote) {
		t.Fatalf("accounting transport mismatch: %+v / %+v", direct, remote)
	}
	projected, err := client.store().ListTokenEvents("s", 10, time.Time{}, time.Time{})
	if err != nil || len(projected) != 2 || projected[1].Attrs["payload_bytes"] != "12" || projected[1].StartLine != 4 {
		t.Fatalf("event evidence lost: %+v, %v", projected, err)
	}
}

// electionRoot lays out a project the way SocketPathFromDB and
// ProjectRootFromDB expect (<root>/.aide/memory/memory.db) and sandboxes the
// instance registry, which otherwise writes under the real home directory.
func electionRoot(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".aide", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, ".aide", "memory", "memory.db")
}

// join, then register the teardown so a failing test doesn't leave the bolt
// lock held for the rest of the package.
func mustJoin(t *testing.T, dbPath string) (*MCPServer, func()) {
	t.Helper()
	s := newMCPServer(nil)
	s.grammarLoader = newGrammarLoader(dbPath, mcpLog)
	s.dbPath = dbPath

	teardown, err := s.join(dbPath, &mcpConfig{})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	return s, teardown
}

func TestJoinBecomesPrimaryWhenNoneRunning(t *testing.T) {
	dbPath := electionRoot(t)

	s, teardown := mustJoin(t, dbPath)
	defer teardown()

	if s.grpcClient() != nil {
		t.Error("joined as client with no primary running, want primary")
	}
	if s.store() == nil {
		t.Error("primary has no store")
	}
}

func TestJoinAttachesWhenPrimaryHoldsTheStore(t *testing.T) {
	dbPath := electionRoot(t)

	primary, stopPrimary := mustJoin(t, dbPath)
	defer stopPrimary()
	if primary.grpcClient() != nil {
		t.Fatal("first join did not become primary")
	}

	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()

	if client.grpcClient() == nil {
		t.Error("second join became primary too; the bolt lock should have refused it")
	}
	if client.store() == nil {
		t.Error("client has no store adapter")
	}
}

func TestClientTeardownKeepsReplacementConnection(t *testing.T) {
	dbPath := electionRoot(t)
	_, stopPrimary := mustJoin(t, dbPath)
	defer stopPrimary()
	client, stopFirst := mustJoin(t, dbPath)
	stopSecond, err := client.join(dbPath, &mcpConfig{})
	if err != nil {
		stopFirst()
		t.Fatal(err)
	}
	defer stopSecond()
	stopFirst()
	if err := client.grpcClient().Ping(context.Background()); err != nil {
		t.Fatalf("old teardown closed replacement connection: %v", err)
	}
}

// The bolt lock is the election, so concurrency is the case that matters:
// whatever the interleaving, exactly one process may hold the store.
func TestElectionYieldsExactlyOnePrimary(t *testing.T) {
	dbPath := electionRoot(t)

	const contenders = 4
	var (
		wg        sync.WaitGroup
		primaries atomic.Int32
		clients   atomic.Int32
		mu        sync.Mutex
		teardowns []func()
	)

	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := newMCPServer(nil)
			s.grammarLoader = newGrammarLoader(dbPath, mcpLog)
			s.dbPath = dbPath

			teardown, err := s.join(dbPath, &mcpConfig{})
			if err != nil {
				t.Errorf("join: %v", err)
				return
			}
			mu.Lock()
			teardowns = append(teardowns, teardown)
			mu.Unlock()

			if s.grpcClient() == nil {
				primaries.Add(1)
			} else {
				clients.Add(1)
			}
		}()
	}
	wg.Wait()
	defer func() {
		for _, fn := range teardowns {
			fn()
		}
	}()

	if got := primaries.Load(); got != 1 {
		t.Errorf("primaries = %d, want exactly 1", got)
	}
	if got := clients.Load(); got != contenders-1 {
		t.Errorf("clients = %d, want %d", got, contenders-1)
	}
}

// The bug this guards: before failover, a client whose primary exited was
// stranded for the rest of its life.
func TestSupervisorPromotesAfterPrimaryExits(t *testing.T) {
	dbPath := electionRoot(t)

	_, stopPrimary := mustJoin(t, dbPath)
	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()

	if client.grpcClient() == nil {
		t.Fatal("second join did not attach as a client")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var swapped atomic.Value
	swapped.Store(func() {})
	go client.supervisePrimary(ctx, dbPath, &mcpConfig{}, func(next func()) {
		swapped.Store(next)
	})

	stopPrimary()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if client.grpcClient() == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer func() { swapped.Load().(func())() }()

	if client.grpcClient() != nil {
		t.Fatal("client was not promoted after the primary exited")
	}
	if client.store() == nil {
		t.Error("promoted client has no store")
	}
	// Run has long since returned its wiring pass, so the status service is
	// only populated if becomePrimary does it.
	if client.grpcSrv() == nil {
		t.Error("promoted client did not publish a gRPC server")
	}
}
