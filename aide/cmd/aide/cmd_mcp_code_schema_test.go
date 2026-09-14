package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPCodeSymbolSelection(t *testing.T) {
	s, cs, root := retrievalFixture(t)
	indexRetrievalFile(t, s, cs, root, "source.go", "package demo\nfunc First() {}\nfunc Second() { First() }\n")
	implementation := &mcp.Implementation{Name: "aide-schema-test", Version: "1"}
	s.server = mcp.NewServer(implementation, nil)
	s.registerCodeTools()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(implementation, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, toolName := range []string{"code_read_symbol", "code_references"} {
		t.Run(toolName, func(t *testing.T) {
			t.Run("schema", func(t *testing.T) {
				for _, tool := range listed.Tools {
					if tool.Name != toolName {
						continue
					}
					encoded, err := json.Marshal(tool.InputSchema)
					if err != nil {
						t.Fatal(err)
					}
					var schema struct {
						Required   []string
						Properties map[string]any
					}
					if err := json.Unmarshal(encoded, &schema); err != nil {
						t.Fatal(err)
					}
					for _, required := range schema.Required {
						if required == "symbol" || required == "symbols" {
							t.Errorf("alternative selector %q is unconditionally required: %s", required, encoded)
						}
					}
					if schema.Properties["symbol"] == nil || schema.Properties["symbols"] == nil {
						t.Errorf("missing singular or batch selector: %s", encoded)
					}
					if schema.Properties["checkout_root"] == nil {
						t.Errorf("missing portable checkout selector: %s", encoded)
					}
					return
				}
				t.Fatal("registered tool missing from tools/list")
			})

			for _, tc := range []struct {
				name      string
				selection map[string]any
				want      []string
				wantError bool
			}{
				{"singular", map[string]any{"symbol": "First"}, []string{"First"}, false},
				{"batch_without_singular", map[string]any{"symbols": []string{"First", "Second"}}, []string{"First", "Second"}, false},
				{"neither", map[string]any{}, []string{"symbol name is required"}, true},
				{"empty_batch", map[string]any{"symbols": []string{}}, []string{"symbol name is required"}, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					args := tc.selection
					args["file"] = "source.go"
					result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args, Meta: mcp.Meta{"aide/checkout_root": s.sourceRoot()}})
					if err != nil {
						t.Fatal(err)
					}
					var text strings.Builder
					for _, content := range result.Content {
						if content, ok := content.(*mcp.TextContent); ok {
							text.WriteString(content.Text)
						}
					}
					if result.IsError != tc.wantError {
						t.Errorf("IsError = %v, want %v: %s", result.IsError, tc.wantError, text.String())
					}
					for _, want := range tc.want {
						if !strings.Contains(text.String(), want) {
							t.Errorf("result missing %q: %s", want, text.String())
						}
					}
				})
			}
		})
	}
}
