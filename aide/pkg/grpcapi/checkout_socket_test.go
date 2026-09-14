package grpcapi

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func TestCheckoutSharedSocketAndRestart(t *testing.T) {
	root, wt := checkoutRoots(t)
	for i, r := range []string{root, wt} {
		name := []string{"MainOnly", "OlderOnly"}[i]
		writeGoFile(t, filepath.Join(r, "same.go"), "package p\nfunc "+name+"(){ "+name+"() }\n")
	}
	dbPath := filepath.Join(root, ".aide", "memory", "memory.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		t.Fatal(err)
	}
	c, err := store.CheckoutInfo(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	dir := store.CheckoutDir(dbPath, c)
	var active *Server
	start := func() func() {
		t.Helper()
		st, err := store.NewCombinedStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := store.NewCodeStore(filepath.Join(dir, "code", "index.db"), filepath.Join(dir, "code", "search.bleve"))
		if err != nil {
			t.Fatal(err)
		}
		fs, err := store.NewCheckoutFindingsStore(filepath.Join(dir, "findings"), st, c)
		if err != nil {
			t.Fatal(err)
		}
		ss, err := store.NewSurveyStore(filepath.Join(dir, "survey"))
		if err != nil {
			t.Fatal(err)
		}
		srv := NewServer(st, dbPath, SocketPathFromDB(dbPath), grammar.NewCompositeLoader())
		active = srv
		srv.SetCodeStore(cs)
		srv.SetFindingsStore(fs)
		srv.SetSurveyStore(ss)
		if err := srv.EnableCheckouts(root, nil); err != nil {
			t.Fatal(err)
		}
		ended := make(chan error, 1)
		go func() { ended <- srv.Start() }()
		var once sync.Once
		close := func() {
			once.Do(func() {
				srv.Stop()
				<-ended
				ss.Close()
				fs.Close()
				cs.Close()
				st.Close()
			})
		}
		t.Cleanup(close)
		return close
	}
	connect := func(r string) *Client {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			client, err := NewClientForCheckout(dbPath, r)
			if err == nil {
				t.Cleanup(func() { client.Close() })
				return client
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	stop := start()
	clients := []*Client{connect(root), connect(wt)}
	if clients[0].conn.Target() != clients[1].conn.Target() {
		t.Fatal("clients use different sockets")
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := client.Code.Index(ctx, &CodeIndexRequest{Paths: []string{"."}})
			if err != nil {
				t.Error(err)
				return
			}
			for {
				_, err = stream.Recv()
				if err == io.EOF {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	check := func() {
		t.Helper()
		for i, client := range clients {
			result, err := client.Code.Search(ctx, &CodeSearchRequest{Query: "Only"})
			want := []string{"MainOnly", "OlderOnly"}[i]
			if err != nil || len(result.Symbols) != 1 || result.Symbols[0].Name != want {
				t.Fatalf("checkout %d: %+v %v", i, result, err)
			}
			refs, err := client.Code.SearchReferences(ctx, &CodeSearchReferencesRequest{SymbolName: want})
			if err != nil || len(refs.References) != 1 {
				t.Fatalf("references %d: %+v %v", i, refs, err)
			}
		}
	}
	check()
	added, err := clients[1].Findings.Add(ctx, &FindingAddRequest{Analyzer: "complexity", Title: "Older finding", FilePath: "same.go"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := clients[0].Findings.Get(ctx, &FindingGetRequest{Id: added.Finding.Id})
	if err != nil || other.Found {
		t.Fatalf("finding leaked: %+v %v", other, err)
	}
	survey, err := clients[1].Survey.Add(ctx, &SurveyAddRequest{Analyzer: "topology", Kind: "module", Name: "older", FilePath: "same.go"})
	if err != nil {
		t.Fatal(err)
	}
	otherSurvey, err := clients[0].Survey.Get(ctx, &SurveyGetRequest{Id: survey.Entry.Id})
	if err != nil || otherSurvey.Found {
		t.Fatalf("survey leaked: %+v %v", otherSurvey, err)
	}
	dec, err := clients[1].Decision.Set(ctx, &DecisionSetRequest{Topic: "choice", Decision: "shared decision"})
	if err != nil {
		t.Fatal(err)
	}
	wtInfo, err := store.CheckoutInfo(dbPath, wt)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision.Checkout == nil || dec.Decision.Checkout.Id != wtInfo.ID {
		t.Fatalf("wrong decision origin: %+v", dec.Decision)
	}
	shared, err := clients[0].Decision.Get(ctx, &DecisionGetRequest{Topic: "choice"})
	if err != nil || !shared.Found || shared.Decision.Checkout.Id != wtInfo.ID {
		t.Fatalf("decision not shared: %+v %v", shared, err)
	}
	clients[0].Close()
	clients[1].Close()
	stop()
	stop = start()
	defer stop()
	clients = []*Client{connect(root), connect(wt)}
	check()
	again, err := store.CheckoutInfo(dbPath, wt)
	if err != nil || again.ID != wtInfo.ID {
		t.Fatal("checkout identity changed across restart")
	}
	// A worktree moved by Git keeps the same stores and reopens its owner at
	// the new root without changing the other client's request context.
	moved := wt + "-moved"
	if err := os.Rename(wt, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.WriteFile(filepath.Join(wtInfo.GitDir, "gitdir"), []byte(filepath.Join(moved, ".git")), 0600); err != nil {
		t.Fatal(err)
	}
	clients[1].Close()
	clients[1] = connect(moved)
	check()
	// Removing the checkout and its Git registration starts a grace period.
	// Cache removal never deletes a decision made in that checkout.
	if err := os.RemoveAll(moved); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(wtInfo.GitDir); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if n, err := active.PruneCheckoutCaches(now); err != nil || n != 0 {
		t.Fatalf("start grace: %d %v", n, err)
	}
	if n, err := active.PruneCheckoutCaches(now.Add(8 * 24 * time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	shared, err = clients[0].Decision.Get(ctx, &DecisionGetRequest{Topic: "choice"})
	if err != nil || !shared.Found || shared.Decision.Checkout.Id != wtInfo.ID {
		t.Fatalf("pruning lost decision: %+v %v", shared, err)
	}
}
