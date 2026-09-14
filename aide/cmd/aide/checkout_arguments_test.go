package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func checkoutArgumentFixture(t *testing.T) (*MCPServer, string, string) {
	t.Helper()
	s, cs, root := retrievalFixture(t)
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	admin := filepath.Join(root, ".git", "worktrees", "other")
	if err := os.MkdirAll(admin, 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{filepath.Join(wt, ".git"): "gitdir: " + admin, filepath.Join(admin, "commondir"): "../..", filepath.Join(admin, "HEAD"): "ref: refs/heads/other\n", filepath.Join(admin, "gitdir"): filepath.Join(wt, ".git")} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	indexRetrievalFile(t, s, cs, root, "same.go", "package p\nfunc MainOnly(){}\n")
	if err := os.WriteFile(filepath.Join(wt, "same.go"), []byte("package p\nfunc OtherOnly(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	shared, err := store.NewCombinedStore(s.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })
	if err := shared.AddMemory(&memory.Memory{Content: "RepositorySharedMemory", Category: "learning"}); err != nil {
		t.Fatal(err)
	}
	primary := grpcapi.NewServer(shared, s.dbPath, "", s.grammarLoader)
	primary.SetCodeStore(cs)
	if err := primary.EnableCheckouts(root, func(scoped *grpcapi.Server, c checkout.Info) (func(), error) {
		idx := NewIndexerFromStore(scoped.GetCodeStore(), s.grammarLoader, c.Root)
		defer idx.Close()
		if _, err := idx.Reconcile(); err != nil {
			return nil, err
		}
		if err := scoped.GetFindingsStore().AddFinding(&findings.Finding{Analyzer: "test", Title: "OtherFinding", FilePath: "same.go"}); err != nil {
			return nil, err
		}
		if err := scoped.GetSurveyStore().AddEntry(&survey.Entry{Analyzer: "test", Kind: "module", Name: "OtherModule", Title: "OtherModule", FilePath: "same.go"}); err != nil {
			return nil, err
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primary.Stop)
	s.setBackend(&mcpBackend{store: shared, codeStore: cs})
	s.grpcServer.Store(primary)
	s.checkoutRoot = root
	s.server = mcp.NewServer(&mcp.Implementation{Name: "checkout-arguments", Version: "1"}, nil)
	s.registerCodeTools()
	s.registerFindingsTools()
	s.registerSurveyTools()
	s.registerMemoryTools()
	s.registerInstanceInfoTools()
	return s, root, wt
}

func TestCheckoutArgumentsWithoutClientRoots(t *testing.T) {
	s, root, wt := checkoutArgumentFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "plain-mcp-client", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		data, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Properties map[string]any
			Required   []string
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		analysis := strings.HasPrefix(tool.Name, "code_") || strings.HasPrefix(tool.Name, "findings_") || strings.HasPrefix(tool.Name, "survey_") || tool.Name == "instance_info"
		if (schema.Properties["checkout_root"] != nil) != analysis {
			t.Errorf("wrong checkout scope for %s: %s", tool.Name, data)
		}
		for _, required := range schema.Required {
			if required == "checkout_root" {
				t.Errorf("checkout selector required on %s", tool.Name)
			}
		}
	}
	cases := []struct {
		name, tool string
		args       map[string]any
		meta       mcp.Meta
		want, via  string
		bad        bool
	}{
		{"launch", "code_search", map[string]any{"query": "Only"}, nil, "MainOnly", "launch", false},
		{"argument", "code_search", map[string]any{"query": "Only", "checkout_root": wt}, nil, "OtherOnly", "argument", false},
		{"absolute file", "code_symbols", map[string]any{"file": filepath.Join(wt, "same.go")}, nil, "OtherOnly", "file", false},
		{"absolute filter", "code_search", map[string]any{"query": "Only", "file": filepath.Join(wt, "same.go")}, nil, "OtherOnly", "file", false},
		{"relative file", "code_symbols", map[string]any{"file": "same.go", "checkout_root": wt}, nil, "OtherOnly", "argument", false},
		{"metadata", "code_search", map[string]any{"query": "Only"}, mcp.Meta{"aide/checkout_root": wt}, "OtherOnly", "metadata", false},
		{"agreeing hints", "code_symbols", map[string]any{"file": filepath.Join(wt, "same.go"), "checkout_root": wt}, mcp.Meta{"aide/checkout_root": wt}, "OtherOnly", "argument", false},
		{"invalid metadata", "code_search", map[string]any{"query": "Only"}, mcp.Meta{"aide/checkout_root": 42}, "must be a string", "", true},
		{"file conflict", "code_symbols", map[string]any{"file": filepath.Join(wt, "same.go"), "checkout_root": root}, nil, "conflicting checkout hints", "", true},
		{"metadata conflict", "code_search", map[string]any{"query": "Only", "checkout_root": wt}, mcp.Meta{"aide/checkout_root": root}, "conflicting checkout hints", "", true},
		{"relative root", "code_search", map[string]any{"query": "Only", "checkout_root": "../other"}, nil, "absolute checkout path", "", true},
		{"findings filter", "findings_list", map[string]any{"file": filepath.Join(wt, "same.go")}, nil, "OtherFinding", "file", false},
		{"survey filter", "survey_list", map[string]any{"file": filepath.Join(wt, "same.go")}, nil, "OtherModule", "file", false},
		{"stats", "code_stats", map[string]any{"checkout_root": wt}, nil, "Files indexed: 1", "argument", false},
	}
	alias := filepath.Join(t.TempDir(), "worktree-alias")
	if err := os.Symlink(wt, alias); err == nil {
		cases = append(cases, struct {
			name, tool string
			args       map[string]any
			meta       mcp.Meta
			want, via  string
			bad        bool
		}{"alias missing file", "code_read_check", map[string]any{"file": filepath.Join(alias, "missing.go")}, nil, "\"indexed\":false", "file", false})
		cases = append(cases, struct {
			name, tool string
			args       map[string]any
			meta       mcp.Meta
			want, via  string
			bad        bool
		}{"alias existing file", "code_symbols", map[string]any{"file": filepath.Join(alias, "same.go")}, nil, "OtherOnly", "file", false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args, Meta: tc.meta})
			if err != nil {
				t.Fatal(err)
			}
			var body strings.Builder
			for _, item := range result.Content {
				if text, ok := item.(*mcp.TextContent); ok {
					body.WriteString(text.Text)
				}
			}
			if result.IsError != tc.bad || !strings.Contains(body.String(), tc.want) {
				t.Fatalf("result: %s error=%v", body.String(), result.IsError)
			}
			if !tc.bad {
				data, _ := json.Marshal(result.Meta["aide/checkout"])
				var route struct{ ID, Root, Source string }
				if err := json.Unmarshal(data, &route); err != nil {
					t.Fatal(err)
				}
				if route.ID == "" || route.Source != tc.via {
					t.Fatalf("routing metadata: %s", data)
				}
			}
		})
	}
	// Same connection, simultaneous calls: request arguments never become defaults.
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, want, other := root, "MainOnly", "OtherOnly"
			if i%2 == 1 {
				r, want, other = wt, "OtherOnly", "MainOnly"
			}
			result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "code_search", Arguments: map[string]any{"query": "Only", "checkout_root": r}})
			if err != nil {
				t.Error(err)
				return
			}
			data, _ := json.Marshal(result.Content)
			if result.IsError || !strings.Contains(string(data), want) || strings.Contains(string(data), other) {
				t.Errorf("concurrent result: %s", data)
			}
		}()
	}
	wg.Wait()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "code_search", Arguments: map[string]any{"query": "Only"}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result.Content)
	if result.IsError || !strings.Contains(string(data), "MainOnly") || strings.Contains(string(data), "OtherOnly") {
		t.Fatalf("override changed launch default: %s", data)
	}
	for _, r := range []string{root, wt} {
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "memory_list", Arguments: map[string]any{"limit": 1}, Meta: mcp.Meta{"aide/checkout_root": r}})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(result.Content)
		if result.IsError || !strings.Contains(string(data), "RepositorySharedMemory") || result.Meta["aide/checkout"] != nil {
			t.Fatalf("memory scoped by checkout: %s", data)
		}
	}
}

func TestCheckoutHintValidation(t *testing.T) {
	s, root, wt := checkoutArgumentFixture(t)
	ctx := context.Background()
	if _, _, err := s.resolveToolCheckout(ctx, nil, "", []string{filepath.Join(root, "same.go"), filepath.Join(wt, "same.go")}); err == nil {
		t.Fatal("mixed checkout batch accepted")
	}
	foreign := t.TempDir()
	if _, err := git.PlainInit(foreign, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.resolveToolCheckout(ctx, nil, foreign, nil); err == nil {
		t.Fatal("foreign root accepted")
	}
	if _, _, err := s.resolveToolCheckout(ctx, nil, "", []string{filepath.Join(foreign, "missing.go")}); err == nil {
		t.Fatal("foreign file accepted")
	}
	for _, path := range []string{filepath.Join(wt, "*.go"), filepath.Join(wt, "missing.go"), "same.go", wt} {
		if concreteFile(path) {
			t.Errorf("non-concrete filter used as root: %s", path)
		}
	}
	link := filepath.Join(root, "other.go")
	if err := os.Symlink(filepath.Join(wt, "same.go"), link); err == nil {
		if _, _, err := s.resolveToolCheckout(ctx, nil, root, []string{link}); err == nil {
			t.Fatal("cross-checkout symlink conflict accepted")
		}
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(wt, "absent"), dangling); err == nil {
		if _, err := canonicalCheckoutFile(filepath.Join(dangling, "file.go")); err == nil {
			t.Fatal("dangling parent symlink accepted")
		}
	}
}
