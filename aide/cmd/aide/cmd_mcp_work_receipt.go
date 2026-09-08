package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A receipt links a surviving host observation to this exact server operation.
// It is protocol metadata, not model text or proof of final delivery/quality.
func recordWorkReceipt(span *observe.Span, tool string, result *mcp.CallToolResult) {
	digest := sha256.New()
	hasText := len(result.Content) == 0
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok && text != nil {
			hasText = true
			_, _ = digest.Write([]byte(text.Text))
		}
	}
	if !hasText {
		return
	}
	id, hash := rand.Text(), fmt.Sprintf("%x", digest.Sum(nil))
	span.Attr("work_id", id).Attr("work_text_sha256", hash)
	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}
	result.Meta["aide/work"] = map[string]any{"version": 1, "id": id, "tool": tool, "text_sha256": hash}
}
