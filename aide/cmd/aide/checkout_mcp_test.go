package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//nolint:staticcheck // Exercise compatibility with MCP clients that advertise roots.
func TestMCPSessionsRouteDifferentCheckouts(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	admin := filepath.Join(root, ".git", "worktrees", "other")
	os.MkdirAll(admin, 0700)
	for path, data := range map[string]string{filepath.Join(wt, ".git"): "gitdir: " + admin, filepath.Join(admin, "commondir"): "../..", filepath.Join(admin, "HEAD"): "ref: refs/heads/other\n", filepath.Join(admin, "gitdir"): filepath.Join(wt, ".git")} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	indexRetrievalFile(t, s, cs, root, "same.go", "package p\nfunc MainOnly(){}\n")
	if err := os.WriteFile(filepath.Join(wt, "same.go"), []byte("package p\nfunc OtherOnly(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	primary := grpcapi.NewServer(nil, s.dbPath, "", s.grammarLoader)
	primary.SetCodeStore(cs)
	if err := primary.EnableCheckouts(root, func(scoped *grpcapi.Server, c checkout.Info) (func(), error) {
		idx := NewIndexerFromStore(scoped.GetCodeStore(), s.grammarLoader, c.Root)
		_, err := idx.Reconcile()
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()
	s.grpcServer.Store(primary)
	s.checkoutRoot = root
	implementation := &mcp.Implementation{Name: "checkout-test", Version: "1"}
	s.server = mcp.NewServer(implementation, nil)
	s.registerCodeTools()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessions := make([]*mcp.ClientSession, 0, 2)
	for _, r := range []string{root, wt} {
		st, ct := mcp.NewInMemoryTransports()
		ss, err := s.server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		client := mcp.NewClient(implementation, nil)
		client.AddRoots(&mcp.Root{URI: (&url.URL{Scheme: "file", Path: filepath.ToSlash(r)}).String()})
		session, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		sessions = append(sessions, session)
	}
	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			params := &mcp.CallToolParams{Name: "code_search", Arguments: map[string]any{"query": "Only"}}
			result, err := session.CallTool(ctx, params)
			if err != nil {
				t.Error(err)
				return
			}
			// The SDK answers the server's roots input request using this session's roots.

			var text string
			for _, c := range result.Content {
				if c, ok := c.(*mcp.TextContent); ok {
					text += c.Text
				}
			}
			want, other := []string{"MainOnly", "OtherOnly"}[i], []string{"OtherOnly", "MainOnly"}[i]
			if result.IsError || !strings.Contains(text, want) || strings.Contains(text, other) {
				t.Errorf("checkout %d: %s", i, text)
			}
			// An explicit per-request root can override the connection's normal checkout.
			params.Meta = mcp.Meta{"aide/checkout_root": root}
			params.InputResponses = nil
			result, err = session.CallTool(ctx, params)
			if err != nil || result.IsError {
				t.Errorf("explicit checkout: %+v %v", result, err)
				return
			}
			text = ""
			for _, content := range result.Content {
				if c, ok := content.(*mcp.TextContent); ok {
					text += c.Text
				}
			}
			if !strings.Contains(text, "MainOnly") || strings.Contains(text, "OtherOnly") {
				t.Errorf("explicit root did not override session: %s", text)
			}
		}()
	}
	wg.Wait()
	foreign := t.TempDir()
	if _, err := git.PlainInit(foreign, false); err != nil {
		t.Fatal(err)
	}
	result, err := sessions[0].CallTool(ctx, &mcp.CallToolParams{Name: "code_search", Arguments: map[string]any{"query": "Only"}, Meta: mcp.Meta{"aide/checkout_root": foreign}})
	if err == nil && !result.IsError {
		t.Fatal("accepted unrelated repository over shared MCP server")
	}
}
