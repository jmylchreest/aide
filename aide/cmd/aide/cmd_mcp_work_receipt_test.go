package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPWorkReceipt(t *testing.T) {
	ids := map[string]bool{}
	for _, tc := range []struct {
		name    string
		result  *mcp.CallToolResult
		text    string
		receipt bool
	}{
		{"text", textResult("é"), "é", true},
		{"empty", &mcp.CallToolResult{}, "", true},
		{"error", errorResult("failure"), "Error: failure", true},
		{"multiple", &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "one"}, &mcp.TextContent{Text: "two"}}}, "onetwo", true},
		{"image", &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: []byte{1}, MIMEType: "image/png"}}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.result.Meta = mcp.Meta{"aide/retrieval": "preserved", "other": "preserved"}
			h := newMCPServer(nil).toolObserveMiddleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { return tc.result, nil })
			_, err := h(context.Background(), "tools/call", &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "memory_list"}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.result.Meta["aide/retrieval"] != "preserved" || tc.result.Meta["other"] != "preserved" {
				t.Fatal("lost metadata")
			}
			r, ok := tc.result.Meta["aide/work"].(map[string]any)
			if ok != tc.receipt {
				t.Fatalf("receipt: %+v", tc.result.Meta)
			}
			if !ok {
				return
			}
			id, _ := r["id"].(string)
			if id == "" || len(id) > 128 || ids[id] {
				t.Fatalf("invalid/nonunique id %q", id)
			}
			ids[id] = true
			if r["version"] != 1 || r["tool"] != "memory_list" || r["text_sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(tc.text))) {
				t.Fatalf("receipt: %+v", r)
			}
		})
	}
}
