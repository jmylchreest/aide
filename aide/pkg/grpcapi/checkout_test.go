package grpcapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"google.golang.org/grpc/metadata"
)

func TestClientRejectsUnavailableCheckout(t *testing.T) {
	if _, err := NewClientForCheckout(filepath.Join(t.TempDir(), "memory.db"), ""); err == nil || !strings.Contains(err.Error(), "caller checkout directory is unavailable") {
		t.Fatalf("missing caller context was not rejected: %v", err)
	}
}

func checkoutRoots(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	admin := filepath.Join(root, ".git", "worktrees", "older")
	if err := os.MkdirAll(admin, 0700); err != nil {
		t.Fatal(err)
	}
	for p, data := range map[string]string{filepath.Join(wt, ".git"): "gitdir: " + admin, filepath.Join(admin, "HEAD"): "ref: refs/heads/older\n", filepath.Join(admin, "commondir"): "../..", filepath.Join(admin, "gitdir"): filepath.Join(wt, ".git")} {
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, wt
}

func TestCheckoutRouting(t *testing.T) {
	root, wt := checkoutRoots(t)
	writeGoFile(t, filepath.Join(root, "same.go"), "package p\nfunc MainOnly() {}\n")
	writeGoFile(t, filepath.Join(wt, "same.go"), "package p\nfunc OlderOnly() {}\n")
	svc := newCodeServiceFixture(t, root)
	if err := svc.server.EnableCheckouts(root, nil); err != nil {
		t.Fatal(err)
	}
	defer svc.server.closeCheckouts()
	contexts := []context.Context{metadata.NewIncomingContext(context.Background(), metadata.Pairs(checkoutRootKey, root)), metadata.NewIncomingContext(context.Background(), metadata.Pairs(checkoutRootKey, wt))}
	var wg sync.WaitGroup
	for _, ctx := range contexts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.Index(&CodeIndexRequest{Paths: []string{"."}}, &fakeIndexStream{ctx: ctx}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for i, ctx := range contexts {
		result, err := svc.Search(ctx, &CodeSearchRequest{Query: "Only", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"MainOnly", "OlderOnly"}[i]
		if len(result.Symbols) != 1 || result.Symbols[0].Name != want {
			t.Fatalf("checkout %d: %+v", i, result.Symbols)
		}
	}
	foreign := metadata.NewIncomingContext(context.Background(), metadata.Pairs(checkoutRootKey, t.TempDir()))
	if _, err := svc.Search(foreign, &CodeSearchRequest{Query: "Only"}); err == nil {
		t.Fatal("foreign root accepted")
	}
	stale := filepath.Join(root, "former-worktree")
	for _, exists := range []bool{false, true} {
		if exists {
			if err := os.Mkdir(stale, 0700); err != nil {
				t.Fatal(err)
			}
		}
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(checkoutRootKey, stale))
		if _, err := svc.Search(ctx, &CodeSearchRequest{Query: "Only"}); err == nil {
			t.Fatalf("stale checkout selected ancestor (exists: %v)", exists)
		}
	}
	// Legacy requests must address the main checkout, independently of daemon cwd.
	scoped, release, err := svc.server.checkoutFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	syms, err := scoped.GetCodeStore().SearchSymbols("MainOnly", code.SearchOptions{})
	if err != nil || len(syms) != 1 {
		t.Fatalf("legacy main routing: %v %v", syms, err)
	}
}
