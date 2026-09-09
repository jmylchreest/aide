package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCodeReferencesCoverage(t *testing.T) {
	s, cs, _ := retrievalFixture(t)
	for i := 0; i < DefaultCodeRefsLimit+1; i++ {
		if err := cs.AddReference(&code.Reference{
			SymbolName: "Many", Kind: code.RefKindCall, FilePath: "caller.go",
			Line: i + 1, Context: fmt.Sprintf("Many() // %d", i+1),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name      string
		input     CodeReferencesInput
		count     int
		wantLimit bool
		wantEmpty bool
	}{
		{"limited", CodeReferencesInput{SymbolName: "Many", Limit: 2}, 2, true, false},
		{"exact_limit", CodeReferencesInput{SymbolName: "Many", Limit: DefaultCodeRefsLimit + 1}, DefaultCodeRefsLimit + 1, true, false},
		{"below_limit", CodeReferencesInput{SymbolName: "Many", Limit: DefaultCodeRefsLimit + 2}, DefaultCodeRefsLimit + 1, false, false},
		{"default", CodeReferencesInput{SymbolName: "Many"}, DefaultCodeRefsLimit, true, false},
		{"negative_default", CodeReferencesInput{SymbolName: "Many", Limit: -1}, DefaultCodeRefsLimit, true, false},
		{"filtered_empty", CodeReferencesInput{SymbolName: "Many", FilePath: "absent.go", Limit: 2}, 0, false, true},
		{"batch", CodeReferencesInput{SymbolNames: []string{"Many", "Missing"}, Limit: 2}, 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, _, err := s.handleCodeReferences(context.Background(), nil, tc.input)
			if err != nil || result.IsError {
				t.Fatalf("reference search failed: %v, %+v", err, result)
			}
			var sb strings.Builder
			for _, content := range result.Content {
				if text, ok := content.(*mcp.TextContent); ok {
					sb.WriteString(text.Text)
				}
			}
			text := sb.String()
			if got := strings.Count(text, "- **Line "); got != tc.count {
				t.Errorf("returned %d reference rows, want %d", got, tc.count)
			}
			if tc.count > 0 && !strings.Contains(text, fmt.Sprintf("Returned %d indexed reference candidates", tc.count)) {
				t.Errorf("missing qualified count: %s", text)
			}
			if got := strings.Contains(text, "more indexed matches may exist"); got != tc.wantLimit {
				t.Errorf("limit uncertainty = %v, want %v: %s", got, tc.wantLimit, text)
			}
			if tc.wantEmpty && !strings.Contains(text, "No indexed reference candidates") {
				t.Errorf("missing qualified empty result: %s", text)
			}
			if !strings.Contains(text, "not a complete semantic call graph") {
				t.Errorf("missing coverage boundary: %s", text)
			}
		})
	}
}
