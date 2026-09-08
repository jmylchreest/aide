package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func retrievalFixture(t *testing.T) (*MCPServer, *store.CodeStore, string) {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, ".aide", "memory", "memory.db")
	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	s := newMCPServer(nil)
	s.dbPath = dbPath
	s.grammarLoader = grammar.NewCompositeLoader()
	s.setCodeStore(cs)
	return s, cs, root
}

func indexRetrievalFile(t *testing.T, s *MCPServer, cs *store.CodeStore, root, file, content string) {
	t.Helper()
	abs := filepath.Join(root, file)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	parser := code.NewParser(s.grammarLoader)
	defer parser.Close()
	syms, err := parser.ParseContent([]byte(content), "go", file)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.IndexFileBatch(file, syms, nil, stat.ModTime(), stat.Size()); err != nil {
		t.Fatal(err)
	}
}

func retrievalText(t *testing.T, s *MCPServer, input string) (*mcp.CallToolResult, string) {
	t.Helper()
	var in CodeReadSymbolInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		t.Fatal(err)
	}
	r, _, err := s.handleCodeReadSymbol(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, c := range r.Content {
		if c, ok := c.(*mcp.TextContent); ok {
			text.WriteString(c.Text)
		}
	}
	return r, text.String()
}

func TestRetrievalRefreshesSymbolLines(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package demo\nfunc Target() string { return \"old\" }\n")
	// Preserve mtime AND size: timestamp freshness alone cannot establish source identity.
	stat, _ := os.Stat(filepath.Join(root, "source.go"))
	updated := "package demo\n\nfunc Target() string{ return \"new\" }\n"
	if int64(len(updated)) != stat.Size() {
		t.Fatal("fixture must preserve byte size")
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "source.go"), stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	r, text := retrievalText(t, s, `{"symbol":"Target"}`)
	if r.IsError || !strings.Contains(text, `return "new"`) || !strings.Contains(text, "source.go:3-3") {
		t.Fatalf("stale source range: %s", text)
	}
}

func TestRetrievalSameFileAmbiguityAndDeletedSymbol(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	content := "package demo\ntype A struct{}\ntype B struct{}\nfunc (A) Target() string { return \"first\" }\nfunc (B) Target() string { return \"second\" }\n"
	indexRetrievalFile(t, s, cs, root, "source.go", content)
	r, text := retrievalText(t, s, `{"symbol":"Target","file":"source.go"}`)
	if !r.IsError || !strings.Contains(text, "source.go:4") || !strings.Contains(text, "source.go:5") {
		t.Fatalf("ambiguous methods: %s", text)
	}
	r, text = retrievalText(t, s, `{"symbol":"Target","file":"source.go","start_line":5}`)
	if r.IsError || !strings.Contains(text, `return "second"`) {
		t.Fatalf("line selector: %s", text)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package demo\nfunc Replacement() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, text = retrievalText(t, s, `{"symbol":"Target"}`)
	if !r.IsError || strings.Contains(text, "func Replacement()") {
		t.Fatalf("returned replacement as deleted symbol: %s", text)
	}
}

func TestRetrievalBatchRecordsOneVersionedReferencePerFile(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	content := "package demo\nfunc First() {}\nfunc Second() {}\n"
	indexRetrievalFile(t, s, cs, root, "source.go", content)
	st, err := store.NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	observe.SetDefault(store.NewObserveSink(st))
	defer observe.SetDefault(nil)
	ctx, span := observe.StartCtx(context.Background(), "code_read_symbol", observe.KindToolCall)
	r, _, err := s.handleCodeReadSymbol(ctx, nil, CodeReadSymbolInput{Symbols: []string{"First", "Second"}})
	span.End()
	if err != nil || r.IsError {
		t.Fatalf("batch failed: %+v %v", r, err)
	}
	events, err := st.ListObserveEvents(store.ObserveFilter{})
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %+v %v", events, err)
	}
	var refs []struct {
		File   string
		SHA256 string
		Bytes  int
	}
	if err := json.Unmarshal([]byte(events[0].Attrs["source_references"]), &refs); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].File != "source.go" || refs[0].Bytes != len(content) || refs[0].SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(content))) {
		t.Fatalf("invalid or duplicate reference: %+v", refs)
	}
	if events[0].TokensSaved != 0 {
		t.Fatal("conditional reference must not be recorded as observed savings")
	}
	assertRetrievalReceipt(t, r, "code_read_symbol", events[0].Attrs["retrieval_id"], events[0].Attrs["source_references"])
}

func assertRetrievalReceipt(t *testing.T, result *mcp.CallToolResult, tool, id, references string) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Meta map[string]struct {
			Version    int             `json:"version"`
			ID         string          `json:"id"`
			Tool       string          `json:"tool"`
			TextSHA256 string          `json:"text_sha256"`
			References json.RawMessage `json:"references"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	r := wire.Meta["aide/retrieval"]
	var text strings.Builder
	for _, content := range result.Content {
		text.WriteString(content.(*mcp.TextContent).Text)
	}
	if r.Version != 1 || r.ID == "" || r.ID != id || r.Tool != tool || r.TextSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(text.String()))) || string(r.References) != references {
		t.Fatalf("receipt does not bind server source evidence to returned text: %s", encoded)
	}
	if strings.Contains(text.String(), "aide/retrieval") || strings.Contains(text.String(), id) {
		t.Fatal("receipt leaked into model text")
	}
}

func TestRetrievalRejectsAmbiguousNames(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "a.go", "package demo\nfunc Target() string { return \"first\" }\n")
	indexRetrievalFile(t, s, cs, root, "b.go", "package demo\nfunc Target() string { return \"second\" }\n")
	r, text := retrievalText(t, s, `{"symbol":"Target"}`)
	if !r.IsError || !strings.Contains(text, "ambiguous") || !strings.Contains(text, "a.go") || !strings.Contains(text, "b.go") {
		t.Fatalf("silently chose a definition: %s", text)
	}
	r, text = retrievalText(t, s, `{"symbol":"Target","file":"b.go"}`)
	if r.IsError || !strings.Contains(text, `return "second"`) || strings.Contains(text, `return "first"`) {
		t.Fatalf("file selector ignored: %s", text)
	}
}

func TestRetrievalExplicitFileWorksWithoutIndex(t *testing.T) {
	s, _, root := retrievalFixture(t)
	s.setCodeStore(nil)
	if err := os.WriteFile(filepath.Join(root, "fresh.go"), []byte("package demo\nfunc Fresh() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, text := retrievalText(t, s, `{"symbol":"Fresh","file":"fresh.go"}`)
	if r.IsError || !strings.Contains(text, "func Fresh()") {
		t.Fatalf("explicit file requires index: %s", text)
	}
}

func TestRetrievalOutlineReceiptSurvivesWireEncoding(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package demo\nfunc Target() {}\n")
	result, _, err := s.handleCodeOutline(context.Background(), nil, CodeOutlineInput{File: "source.go"})
	if err != nil || result.IsError {
		t.Fatalf("outline: %v %v", result, err)
	}
	receipt := result.Meta["aide/retrieval"].(map[string]any)
	refs, _ := json.Marshal(receipt["references"])
	assertRetrievalReceipt(t, result, "code_outline", receipt["id"].(string), string(refs))
	missing, _, _ := s.handleCodeOutline(context.Background(), nil, CodeOutlineInput{File: "missing.go"})
	if !missing.IsError || missing.Meta["aide/retrieval"] != nil {
		t.Fatal("failed outline must not claim a source receipt")
	}
	partial, _ := retrievalText(t, s, `{"symbols":["Target","Missing"],"file":"source.go"}`)
	if !partial.IsError || partial.Meta["aide/retrieval"] == nil {
		t.Fatal("partial batch must keep its failure and successful source evidence")
	}
}

func TestRetrievalFallsBackForDanglingSymbolRecords(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package demo\nfunc Target() {}\n")
	info, err := cs.GetFileInfo("source.go")
	if err != nil {
		t.Fatal(err)
	}
	info.SymbolIDs = []string{"missing-record"}
	if err := cs.SetFileInfo(info); err != nil {
		t.Fatal(err)
	}
	syms, err := s.getFileSymbolsFresh("source.go")
	if err != nil || len(syms) != 1 || syms[0].Name != "Target" {
		t.Fatalf("incomplete index accepted: %+v / %v", syms, err)
	}
}

func TestRetrievalBodyRangesSurviveDaemon(t *testing.T) {
	dbPath := electionRoot(t)
	primary, stopPrimary := mustJoin(t, dbPath)
	defer stopPrimary()
	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	primary.setCodeStore(cs)
	primary.grpcSrv().SetCodeStore(cs)
	root := store.ProjectRootFromDB(dbPath)
	content := "package demo\nfunc Target() string {\n return \"hidden body\"\n}\n"
	indexRetrievalFile(t, primary, cs, root, "source.go", content)
	client, stopClient := mustJoin(t, dbPath)
	defer stopClient()
	direct, err := cs.GetFileSymbols("source.go")
	if err != nil || len(direct) != 1 || direct[0].BodyStartLine == 0 {
		t.Fatalf("fixture body missing: %+v %v", direct, err)
	}
	remote, err := client.getCodeStore().GetFileSymbols("source.go")
	if err != nil || len(remote) != 1 {
		t.Fatalf("remote symbols: %+v %v", remote, err)
	}
	if remote[0].BodyStartLine != direct[0].BodyStartLine || remote[0].BodyEndLine != direct[0].BodyEndLine {
		t.Fatalf("body range lost: %+v", remote[0])
	}
	outline := buildOutline([]byte(content), remote, true)
	if strings.Contains(outline, "hidden body") {
		t.Fatalf("remote outline did not collapse: %s", outline)
	}
}
