package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRetrievalOutlinePreservesGoSignatures(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		fragments   []string
		declaration string
	}{
		{
			name:        "ordinary function",
			source:      "package demo\nfunc Answer(value int) int {\n return value + 987654\n}\n",
			fragments:   []string{"func Answer(value int) int { ... }"},
			declaration: "func Answer",
		},
		{
			name:        "method with struct parameter and return",
			source:      "package demo\ntype Collector struct{}\nfunc (c *Collector) Add(seen map[string]struct{}) map[string]struct{} {\n seen[\"hidden body sentinel\"] = struct{}{}\n return seen\n}\n",
			fragments:   []string{"func (c *Collector) Add(seen map[string]struct{}) map[string]struct{} { ... }"},
			declaration: "func (c *Collector) Add",
		},
		{
			name:        "anonymous struct result",
			source:      "package demo\nfunc Snapshot() struct{ Count int } {\n panic(\"hidden body sentinel\")\n}\n",
			fragments:   []string{"func Snapshot() struct{ Count int } { ... }"},
			declaration: "func Snapshot",
		},
		{
			name:        "braces in comment and struct tag",
			source:      "package demo\nfunc Tagged(/* { body? } */ value struct{ Count int `json:\"{count}\"` }) int {\n return 987654\n}\n",
			fragments:   []string{"func Tagged(/* { body? } */ value struct{ Count int `json:\"{count}\"` }) int { ... }"},
			declaration: "func Tagged",
		},
		{
			name:        "multiline signature",
			source:      "package demo\nfunc Merge(\n seen map[string]struct{},\n) map[string]struct{} {\n seen[\"hidden body sentinel\"] = struct{}{}\n return seen\n}\n",
			fragments:   []string{"func Merge(", "seen map[string]struct{},", ") map[string]struct{} { ... }"},
			declaration: "func Merge",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, cs, root := retrievalFixture(t)
			indexRetrievalFile(t, s, cs, root, "source.go", test.source)
			result, _, err := s.handleCodeOutline(context.Background(), nil, CodeOutlineInput{File: "source.go"})
			if err != nil || result.IsError {
				t.Fatalf("outline failed: result=%v error=%v", result, err)
			}
			var output strings.Builder
			for _, item := range result.Content {
				if text, ok := item.(*mcp.TextContent); ok {
					output.WriteString(text.Text)
				}
			}
			outline := output.String()
			for _, fragment := range test.fragments {
				if !strings.Contains(outline, fragment) {
					t.Errorf("outline lost signature fragment %q:\n%s", fragment, outline)
				}
			}
			if strings.Count(outline, test.declaration) != 1 {
				t.Errorf("declaration must appear exactly once:\n%s", outline)
			}
			if strings.Contains(outline, "hidden body sentinel") || strings.Contains(outline, "987654") {
				t.Errorf("outline exposed collapsed implementation:\n%s", outline)
			}
		})
	}
}

func TestBuildOutlineSeparateBraceKeepsSignatureOnce(t *testing.T) {
	content := "function answer(): number\n{\n return 42;\n}\n"
	symbols := []*code.Symbol{{
		Name: "answer", Kind: code.KindFunction, Signature: "function answer(): number",
		StartLine: 1, EndLine: 4, BodyStartLine: 2, BodyEndLine: 4,
	}}
	outline := buildOutline([]byte(content), symbols, true)
	want := "// Outline: 1 symbols, 4 lines total\n\n1   : function answer(): number\n2   : { ... }  // lines 2-4\n"
	if outline != want {
		t.Fatalf("separate body brace changed signature or line references:\n%s", outline)
	}
}
