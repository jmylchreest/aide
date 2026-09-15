package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
)

func TestReadCheckMCPDirectParity(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".aide", "memory", "memory.db")
	indexPath, searchPath := getCodeStorePaths(dbPath)
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			cs.Close()
		}
	})
	s := newMCPServer(nil)
	s.dbPath = dbPath
	s.setCodeStore(cs)
	path := filepath.Join(root, "file.go")
	if err := os.WriteFile(path, []byte("αβ🙂"), 0600); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(path)
	if err := cs.SetFileInfo(&code.FileInfo{Path: "file.go", ModTime: stat.ModTime(), Tokens: 9999, SymbolIDs: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	r, _, err := s.handleCodeReadCheck(context.Background(), nil, CodeReadCheckInput{File: "file.go"})
	if err != nil {
		t.Fatal(err)
	}
	var fromMCP ReadCheckResult
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &fromMCP); err != nil {
		t.Fatal(err)
	}
	if fromMCP.TextEstimate == nil || fromMCP.TextEstimate.Bytes != 8 || fromMCP.EstimatedTokens != 3 {
		t.Fatalf("index estimate leaked: %+v", fromMCP)
	}
	cs.Close()
	closed = true
	backend := &Backend{dbPath: s.dbPath}
	direct, err := backend.ReadCheck("file.go")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(direct, &fromMCP) {
		t.Fatalf("direct=%+v MCP=%+v", direct, fromMCP)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	direct, err = backend.ReadCheck("file.go")
	if err != nil {
		t.Fatal(err)
	}
	if direct.TextEstimate != nil || direct.EstimatedTokens != 0 || direct.Fresh {
		t.Fatalf("missing: %+v", direct)
	}
	raw, _ := json.Marshal(direct)
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	if string(fields["text_estimate"]) != "null" {
		t.Fatalf("unknown must be explicit null: %s", raw)
	}
}

type readCheckClient struct {
	grpcapi.CodeServiceClient
	response *grpcapi.CodeReadCheckResponse
}

func (c readCheckClient) ReadCheck(context.Context, *grpcapi.CodeReadCheckRequest, ...grpc.CallOption) (*grpcapi.CodeReadCheckResponse, error) {
	return c.response, nil
}

func TestReadCheckBackendDaemonOptionalEstimate(t *testing.T) {
	for _, estimate := range []*grpcapi.ReadCheckTextEstimate{nil, {Bytes: 0, EstimatedTokens: 0, Estimator: "utf8-bytes/3-v1"}, {Bytes: 9000000000, EstimatedTokens: 3000000000, Estimator: "utf8-bytes/3-v1"}} {
		backend := &Backend{useGRPC: true, grpcClient: &grpcapi.Client{Code: readCheckClient{response: &grpcapi.CodeReadCheckResponse{Indexed: true, TextEstimate: estimate}}}}
		got, err := backend.ReadCheck("file.go")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.TextEstimate, grpcapi.ProtoToReadCheckTextEstimate(estimate)) {
			t.Fatalf("daemon estimate lost: %+v", got)
		}
		if got.EstimatedTokens != 0 {
			t.Fatal("must not invent legacy tokens for old daemon or wide estimate")
		}
	}
}
