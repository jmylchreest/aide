package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func instinctJSONResult(v any) *mcp.CallToolResult {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf("marshal: %v", err))
	}
	return textResult(string(out))
}

type InstinctProposalsListInput struct {
	Status    string `json:"status,omitempty" jsonschema:"Filter by status (open|accepted|rejected|expired). Default: open."`
	Shape     string `json:"shape,omitempty" jsonschema:"Filter by detector shape (repetition|convergence)."`
	SessionID string `json:"session_id,omitempty" jsonschema:"Filter by originating session id."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results. Default 50."`
}

type InstinctInspectInput struct {
	ID string `json:"id" jsonschema:"Proposal ULID."`
}

func (s *MCPServer) registerInstinctTools() {
	mcpLog.Printf("instinct tools: registered (read-only)")

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "instinct_proposals_list",
		Description: `List instinct proposals — patterns the reflect detectors think might
be worth promoting to durable memories. Proposals are NOT memories yet —
they sit in a holding bucket awaiting explicit user approval.

Defaults to status=open. Each proposal carries a shape (repetition,
convergence), a summary, an evidence snapshot, and a proposed_instinct
(the memory content that WOULD be created on accept). Use instinct_inspect
for the full record including the source observe events.

Your role: surface proposals to the user with your judgement, then wait
for them to decide. Do NOT chain list → accept in the same turn. The
user owns the final say on every memory write.

Recommended pattern:
  1. List + summarise proposals for the user.
  2. For each one you'd act on, do the analysis the detectors can't:
       - Is the canned content right, or should it be rewritten?
       - Does this supersede any existing memories? Use memory_search
         to find candidates; judge whether they genuinely conflict
         (structural instinct_key:* dedup is automatic; manual or
         topical conflicts you identify go via --supersedes).
  3. Tell the user what you'd run and WHY, then wait.
  4. Only after explicit user approval, invoke the CLI write surface:
       ./.aide/bin/aide reflect accept <id> [--content=…] [--supersedes=ID1,ID2]
       ./.aide/bin/aide reflect reject <id> [--reason=…]

MCP tools never mutate per aide convention. See the reflect skill for
the full workflow.`,
	}, s.handleInstinctProposalsList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "instinct_inspect",
		Description: `Return the full proposal record including the snapshotted observe
events that triggered it. Use to audit why a proposal exists — what edits,
prompts, or commands the detector saw — before deciding whether to accept
(via the CLI: aide reflect accept <id>).`,
	}, s.handleInstinctInspect)
}

func (s *MCPServer) handleInstinctProposalsList(_ context.Context, _ *mcp.CallToolRequest, input InstinctProposalsListInput) (*mcp.CallToolResult, any, error) {
	if s.instinctStore == nil {
		return errorResult("instinct store not available"), nil, nil
	}
	statusVal := instinct.Status(input.Status)
	if statusVal == "" {
		statusVal = instinct.StatusOpen
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	props, err := s.instinctStore.ListInstinctProposals(store.InstinctFilter{
		Status:    statusVal,
		Shape:     input.Shape,
		SessionID: input.SessionID,
		Limit:     limit,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if props == nil {
		props = []*instinct.Proposal{}
	}
	payload := map[string]any{"proposals": props, "count": len(props)}
	return instinctJSONResult(payload), payload, nil
}

func (s *MCPServer) handleInstinctInspect(_ context.Context, _ *mcp.CallToolRequest, input InstinctInspectInput) (*mcp.CallToolResult, any, error) {
	if s.instinctStore == nil {
		return errorResult("instinct store not available"), nil, nil
	}
	if input.ID == "" {
		return errorResult("id required"), nil, nil
	}
	p, err := s.instinctStore.GetInstinctProposal(input.ID)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if p == nil {
		return errorResult(fmt.Sprintf("proposal %s not found", input.ID)), nil, nil
	}
	payload := map[string]any{"proposal": p}
	return instinctJSONResult(payload), payload, nil
}

// Write operations on proposals are CLI-only:
//   aide reflect accept <id> [--content=OVERRIDE]
//   aide reflect reject <id> [--reason=TEXT]
// Matches aide's read-only-from-the-model convention for MCP tools.
