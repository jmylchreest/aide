package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func discoveryReads(t *testing.T, result *mcp.CallToolResult) []CodeReadSymbolInput {
	t.Helper()
	var reads []CodeReadSymbolInput
	for _, content := range result.Content {
		block, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		for _, line := range strings.Split(block.Text, "\n") {
			if !strings.HasPrefix(line, "code_read_symbol ") {
				continue
			}
			var in CodeReadSymbolInput
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "code_read_symbol ")), &in); err != nil {
				t.Fatal(err)
			}
			reads = append(reads, in)
		}
	}
	return reads
}

func TestDiscoverySearchToCurrentBody(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package p\nfunc Target() string { return \"before\" }\n")
	result, _, err := s.handleCodeSearch(context.Background(), nil, CodeSearchInput{Query: "Target"})
	if err != nil || result.IsError {
		t.Fatalf("search: %v %+v", err, result)
	}
	reads := discoveryReads(t, result)
	if len(reads) != 1 {
		t.Fatalf("want a direct read selector, got %+v", reads)
	}
	if reads[0].CheckoutRoot != s.sourceRoot() || reads[0].File != "source.go" || reads[0].Symbol != "Target" || reads[0].StartLine != 2 {
		t.Fatalf("selector: %+v", reads[0])
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package p\nfunc Target() string { return \"after\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(reads[0])
	body, text := retrievalText(t, s, string(raw))
	if body.IsError || !strings.Contains(text, "after") || strings.Contains(text, "before") {
		t.Fatalf("stale discovery must read current body: %s", text)
	}
}

func TestDiscoveryReferencesDeduplicateCallerAndSkipUnknown(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "caller.go", "package p\nfunc Caller() {\n Target()\n Other()\n}\n")
	for _, r := range []*code.Reference{
		{SymbolName: "Target", FilePath: "caller.go", Line: 3, Kind: code.RefKindCall},
		{SymbolName: "Other", FilePath: "caller.go", Line: 4, Kind: code.RefKindCall},
		{SymbolName: "Target", FilePath: "missing.go", Line: 9, Kind: code.RefKindCall},
	} {
		if err := cs.AddReference(r); err != nil {
			t.Fatal(err)
		}
	}
	result, _, err := s.handleCodeReferences(context.Background(), nil, CodeReferencesInput{SymbolNames: []string{"Target", "Other"}})
	if err != nil || result.IsError {
		t.Fatalf("references: %v %+v", err, result)
	}
	reads := discoveryReads(t, result)
	if len(reads) != 1 || reads[0].Symbol != "Caller" || reads[0].File != "caller.go" || reads[0].StartLine != 2 {
		t.Fatalf("want one caller read, got %+v", reads)
	}
	raw, _ := json.Marshal(reads[0])
	body, text := retrievalText(t, s, string(raw))
	if body.IsError || !strings.Contains(text, "Target()") || !strings.Contains(text, "Other()") {
		t.Fatalf("caller body: %s", text)
	}
}

func TestDiscoveryReferenceOrderStable(t *testing.T) {
	refs := []*code.Reference{{FilePath: "z.go", Line: 9}, {FilePath: "a.go", Line: 5}, {FilePath: "a.go", Line: 2}}
	text := formatCodeReferences("Target", refs, 50)
	if strings.Index(text, "a.go") > strings.Index(text, "z.go") || strings.Index(text, "Line 2") > strings.Index(text, "Line 5") {
		t.Fatalf("unstable source order: %s", text)
	}
}

func TestDiscoverySearchEmptyDoesNotPrescribeReindex(t *testing.T) {
	text := formatCodeSearchResults(nil)
	if !strings.Contains(text, "do not prove absence") || strings.Contains(text, "Tip: Run") {
		t.Fatalf("empty search should not prescribe redundant indexing: %s", text)
	}
}

type discoveryCountingStore struct {
	store.CodeIndexStore
	lookups int
}

func (s *discoveryCountingStore) GetFileSymbols(file string) ([]*code.Symbol, error) {
	s.lookups++
	return s.CodeIndexStore.GetFileSymbols(file)
}

func TestDiscoveryCallerLookupBudget(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "caller.go", "package p\nfunc Caller() { Target() }\n")
	counted := &discoveryCountingStore{CodeIndexStore: cs}
	suggestions := newCodeReadSuggestions(root)
	refs := make([]*code.Reference, 100)
	for i := range refs {
		refs[i] = &code.Reference{FilePath: "caller.go", Line: 2}
	}
	suggestions.references(counted, refs)
	suggestions.references(counted, refs)
	if counted.lookups != 1 {
		t.Fatalf("same file fetched %d times", counted.lookups)
	}
	for i := 0; i < 50; i++ {
		suggestions.references(counted, []*code.Reference{{FilePath: fmt.Sprintf("missing%d.go", i), Line: 2}})
	}
	if counted.lookups > 10 {
		t.Fatalf("unbounded enrichment lookups: %d", counted.lookups)
	}
	if !strings.Contains(suggestions.text(), "limited") {
		t.Fatal("limited selector coverage must be explicit")
	}
}

func TestDiscoveryDuplicateDefinitionsHaveExactSelectors(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "same.go", "package p\ntype A struct{}\ntype B struct{}\nfunc (A) Run() { First() }\nfunc (B) Run() { Second() }\n")
	result, _, err := s.handleCodeSearch(context.Background(), nil, CodeSearchInput{Query: "Run"})
	if err != nil {
		t.Fatal(err)
	}
	reads := discoveryReads(t, result)
	if len(reads) != 2 || reads[0].StartLine == reads[1].StartLine {
		t.Fatalf("duplicate selectors: %+v", reads)
	}
	for _, in := range reads {
		raw, _ := json.Marshal(in)
		body, text := retrievalText(t, s, string(raw))
		if body.IsError || strings.Contains(text, "ambiguous") {
			t.Fatalf("selector should resolve directly: %s", text)
		}
	}
}

func TestDiscoveryMCPSelectorsRetainCheckout(t *testing.T) {
	s, root, other := checkoutArgumentFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, ct := mcp.NewInMemoryTransports()
	server, err := s.server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "plain-discovery-client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, tc := range []struct{ root, name string }{{root, "MainOnly"}, {other, "OtherOnly"}, {root, "MainOnly"}} {
		found, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "code_search", Arguments: map[string]any{"checkout_root": tc.root, "query": tc.name}})
		if err != nil || found.IsError {
			t.Fatalf("MCP search: %v %+v", err, found)
		}
		reads := discoveryReads(t, found)
		if len(reads) != 1 || reads[0].CheckoutRoot != tc.root {
			t.Fatalf("wrong checkout selector: %+v", reads)
		}
		body, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "code_read_symbol", Arguments: reads[0]})
		if err != nil || body.IsError {
			t.Fatalf("MCP read: %v %+v", err, body)
		}
		encoded, _ := json.Marshal(body)
		if !strings.Contains(string(encoded), tc.name) {
			t.Fatalf("wrong body: %s", encoded)
		}
	}
}

func TestDiscoveryMovedSelectorFailsWithoutWrongBody(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package p\nfunc Target() {}\n")
	result, _, err := s.handleCodeSearch(context.Background(), nil, CodeSearchInput{Query: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	reads := discoveryReads(t, result)
	if len(reads) != 1 {
		t.Fatalf("selectors: %+v", reads)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package p\nfunc Different() {}\nfunc Target() { Current() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(reads[0])
	_, text := retrievalText(t, s, string(raw))
	if !strings.Contains(text, "not found in current source") || strings.Contains(text, "func Different") {
		t.Fatalf("wrong stale fallback: %s", text)
	}
	reads[0].StartLine = 0
	raw, _ = json.Marshal(reads[0])
	body, text := retrievalText(t, s, string(raw))
	if body.IsError || !strings.Contains(text, "Current()") {
		t.Fatalf("file/name retry: %s", text)
	}
}
