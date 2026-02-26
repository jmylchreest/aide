package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// Input types for memory, state, decision, message, and usage tools
// ============================================================================

type MemorySearchInput struct {
	Query    string `json:"query" jsonschema:"Search query - uses bleve full-text search with: (1) standard word matching, (2) fuzzy matching for typos (fuzziness=1), (3) edge n-grams for prefix matching (2-15 chars), (4) n-grams for substring matching (3-8 chars). Multi-word queries use OR (any word matches). Use up to 10 distinct keywords like 'colour food preferences'. Fuzzy matching handles spelling variants automatically ('color' matches 'colour'), so synonyms are unnecessary."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results to return (default 10). Increase for broader recall."`
	Category string `json:"category,omitempty" jsonschema:"Filter by category: learning, decision, issue, discovery, blocker"`
}

type MemoryListInput struct {
	Category string `json:"category,omitempty" jsonschema:"Filter by category: learning, decision, issue, discovery, blocker. Leave empty for all."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 50). Increase for comprehensive review."`
}

type StateGetInput struct {
	Key     string `json:"key" jsonschema:"State key: 'mode', 'modelTier', 'activeSkill', or custom keys"`
	AgentID string `json:"agent_id,omitempty" jsonschema:"Agent ID for per-agent state (e.g., 'abc123'). Omit for global state."`
}

type StateListInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Filter to specific agent's state. Omit for all state (global + all agents)."`
}

type DecisionGetInput struct {
	Topic string `json:"topic" jsonschema:"Decision topic key (kebab-case), e.g., 'auth-strategy', 'testing-framework', 'db-schema'. Use decision_list first to discover available topics."`
}

type DecisionHistoryInput struct {
	Topic string `json:"topic" jsonschema:"Decision topic to get full history for (all versions in chronological order). Use decision_list first to discover available topics."`
}

type MessageListInput struct {
	AgentID     string `json:"agent_id" jsonschema:"Your agent ID to receive messages for (required)"`
	IncludeRead bool   `json:"include_read,omitempty" jsonschema:"Include already-acknowledged messages (default false)"`
}

type MessageSendInput struct {
	From       string `json:"from" jsonschema:"Your agent ID (required)"`
	To         string `json:"to,omitempty" jsonschema:"Recipient agent ID. Omit to broadcast to all agents."`
	Content    string `json:"content" jsonschema:"Message content (max 2000 chars)"`
	Type       string `json:"type,omitempty" jsonschema:"Message type: status, request, response, blocker, completion, handoff"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"Time-to-live in seconds (default 3600)"`
}

type MessageAckInput struct {
	MessageID uint64 `json:"message_id" jsonschema:"The numeric ID of the message to acknowledge"`
	AgentID   string `json:"agent_id" jsonschema:"Your agent ID"`
}

type emptyInput struct{}

// ============================================================================
// Memory Tools (read-only)
// ============================================================================

func (s *MCPServer) registerMemoryTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "memory_search",
		Description: `Search project memories — learned facts, discoveries, issues, and blockers accumulated across sessions.

Memories include: user preferences, coding patterns discovered, issues encountered,
architectural decisions (use decision_get for formal decisions), and blockers.

**When to use:** When you need to recall prior context — e.g., "did we decide on
a testing framework?", "what issues did we hit with auth?", "what are the user's preferences?"

**Search capabilities:**
- Fuzzy matching handles typos automatically (1 edit distance)
- Multi-word queries use OR — any word matching is sufficient
- Prefix and substring matching built-in

Results include timestamps — prefer most recent when values conflict.`,
	}, s.handleMemorySearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "memory_list",
		Description: `List all stored memories, optionally filtered by category.

Returns all memories (not just matching ones) with timestamps.
Prefer most recent when answering questions about preferences or decisions.

**When to use:** Use this for broad review or when you need to see everything.
Use memory_search instead when looking for specific topics or keywords.`,
	}, s.handleMemoryList)
}

func (s *MCPServer) handleMemorySearch(_ context.Context, _ *mcp.CallToolRequest, input MemorySearchInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: memory_search query=%q limit=%d", input.Query, input.Limit)

	limit := input.Limit
	if limit == 0 {
		limit = 10
	}

	memories, err := s.store.SearchMemories(input.Query, limit)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d memories", len(memories))
	return textResult(formatMemoriesMarkdown(memories)), nil, nil
}

func (s *MCPServer) handleMemoryList(_ context.Context, _ *mcp.CallToolRequest, input MemoryListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: memory_list category=%q limit=%d", input.Category, input.Limit)

	limit := input.Limit
	if limit == 0 {
		limit = 50
	}

	opts := memory.SearchOptions{
		Category: memory.Category(input.Category),
		Limit:    limit,
	}

	memories, err := s.store.ListMemories(opts)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("list failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d memories", len(memories))
	return textResult(formatMemoriesMarkdown(memories)), nil, nil
}

// ============================================================================
// State Tools (read-only - mutations handled by hooks/skills)
// ============================================================================

func (s *MCPServer) registerStateReadTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "state_get",
		Description: `Get a state value (global or per-agent).

**Common state keys:**
- "mode" - Current operation mode (normal, eco, ralph, etc.)
- "modelTier" - Current model tier (smart, fast, etc.)
- "activeSkill" - Currently executing skill

**Per-agent state:**
Use agent_id parameter to get agent-specific state like "agent:abc123:status".

State is managed by orchestration hooks, not directly settable via tools.`,
	}, s.handleStateGet)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "state_list",
		Description: `List all state values (global and per-agent).

Returns JSON array of all current state entries including:
- Global state (mode, modelTier, etc.)
- Per-agent state (prefixed with "agent:<id>:")

Use agent_id parameter to filter to a specific agent's state only.`,
	}, s.handleStateList)
}

func (s *MCPServer) handleStateGet(_ context.Context, _ *mcp.CallToolRequest, input StateGetInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: state_get key=%s agent=%s", input.Key, input.AgentID)

	stateKey := input.Key
	if input.AgentID != "" {
		stateKey = fmt.Sprintf("agent:%s:%s", input.AgentID, input.Key)
	}

	st, err := s.store.GetState(stateKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			mcpLog.Printf("  not set")
			return textResult("(not set)"), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("get state failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  value: %s", truncate(st.Value, 50))
	return textResult(st.Value), nil, nil
}

func (s *MCPServer) handleStateList(_ context.Context, _ *mcp.CallToolRequest, input StateListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: state_list agent=%s", input.AgentID)

	states, err := s.store.ListState(input.AgentID)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("list state failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d states", len(states))
	result, _ := json.MarshalIndent(states, "", "  ")
	return textResult(string(result)), nil, nil
}

// ============================================================================
// Decision Tools (read-only)
// ============================================================================

func (s *MCPServer) registerDecisionTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "decision_get",
		Description: `Get the current (latest) decision for a topic.

**What are decisions?**
Decisions are architectural choices with full context - more structured than memories.
They have: topic (key), decision summary, rationale, details (schemas/code), references.

**Example topics:**
- "auth-strategy" - Authentication approach
- "testing-framework" - Which test runner to use
- "db-schema" - Database design choices

**Usage:** Call decision_list first to discover available topics, then use this tool with a specific topic.
Decisions are append-only - the value returned here is the current authoritative decision, superseding all previous versions.
Use decision_history to see how a decision evolved over time.`,
	}, s.handleDecisionGet)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "decision_history",
		Description: `Get the full history of decisions for a topic.

Returns ALL versions of a decision in chronological order.
Useful when you need to understand why a decision changed or evolved.
Each entry includes: decision, rationale, details, references, and timestamp.

**Important:** The most recent entry is the current decision - it supersedes all earlier versions.`,
	}, s.handleDecisionHistory)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "decision_list",
		Description: `List all recorded decisions (latest for each topic).

**Start here** - call this first to discover what decision topics exist.
Returns a summary of all decision topics with their current (most recent) values.
Then use decision_get for full details on a specific topic, or decision_history to see how it evolved.`,
	}, s.handleDecisionList)
}

func (s *MCPServer) handleDecisionGet(_ context.Context, _ *mcp.CallToolRequest, input DecisionGetInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: decision_get topic=%s", input.Topic)

	dec, err := s.store.GetDecision(input.Topic)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			mcpLog.Printf("  not found")
			return textResult(fmt.Sprintf("No decision found for topic: %s", input.Topic)), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("get decision failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %s", truncate(dec.Decision, 50))
	return textResult(formatDecisionMarkdown(dec)), nil, nil
}

func (s *MCPServer) handleDecisionHistory(_ context.Context, _ *mcp.CallToolRequest, input DecisionHistoryInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: decision_history topic=%s", input.Topic)

	history, err := s.store.GetDecisionHistory(input.Topic)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("get history failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d entries", len(history))
	return textResult(formatDecisionHistoryMarkdown(input.Topic, history)), nil, nil
}

func (s *MCPServer) handleDecisionList(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: decision_list")

	decisions, err := s.store.ListDecisions()
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("list decisions failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d decisions", len(decisions))
	return textResult(formatDecisionsMarkdown(decisions)), nil, nil
}

// ============================================================================
// Message Tools
// ============================================================================

func (s *MCPServer) registerMessageTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "message_list",
		Description: `For multi-agent (swarm mode) coordination. List messages for an agent (auto-prunes expired).

**What are messages?**
Inter-agent communication in swarm mode. Messages can be:
- Broadcasts (to all agents)
- Directed (to specific agent_id)
- Typed (info, warning, error, etc.)

**Parameters:**
- agent_id (required): Your agent ID to receive messages for
- include_read: Set true to see already-acknowledged messages

Expired messages (past TTL) are automatically pruned.`,
	}, s.handleMessageList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "message_send",
		Description: `For multi-agent (swarm mode) coordination. Send a message to another agent or broadcast to all.

**Message types:**
- "status" — Progress update (e.g., stage transitions)
- "request" — Ask another agent for information or action
- "response" — Reply to a request
- "blocker" — Report something blocking your progress
- "completion" — Signal that your work is done
- "handoff" — Transfer responsibility for a task

**Addressing:**
- Set "to" to a specific agent_id for direct messages
- Omit "to" to broadcast to all agents

**Protocol conventions:**
- Send "status" at each SDLC stage transition
- Send "blocker" when stuck and need help
- Send "completion" when your story/task is done
- Check messages at the start of each stage`,
	}, s.handleMessageSend)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "message_ack",
		Description: `For multi-agent (swarm mode) coordination. Acknowledge a message you've read.

Acknowledging marks the message as read for your agent_id so it won't
appear in future message_list calls (unless include_read is true).
Acknowledge messages after you've processed them to keep your inbox clean.`,
	}, s.handleMessageAck)
}

func (s *MCPServer) handleMessageList(_ context.Context, _ *mcp.CallToolRequest, input MessageListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: message_list agent=%s", input.AgentID)

	messages, err := s.store.GetMessages(input.AgentID)
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("list messages failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d messages", len(messages))
	result, _ := json.MarshalIndent(messages, "", "  ")
	return textResult(string(result)), nil, nil
}

func (s *MCPServer) handleMessageSend(_ context.Context, _ *mcp.CallToolRequest, input MessageSendInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: message_send from=%s to=%s type=%s", input.From, input.To, input.Type)

	if input.From == "" {
		return errorResult("'from' is required"), nil, nil
	}
	if input.Content == "" {
		return errorResult("'content' is required"), nil, nil
	}

	// Cap content length
	content := input.Content
	if len(content) > 2000 {
		content = content[:2000]
	}

	msg := &memory.Message{
		From:    input.From,
		To:      input.To,
		Content: content,
		Type:    input.Type,
	}

	// Apply custom TTL if specified
	if input.TTLSeconds > 0 {
		msg.CreatedAt = time.Now()
		msg.ExpiresAt = msg.CreatedAt.Add(time.Duration(input.TTLSeconds) * time.Second)
	}

	if err := s.store.AddMessage(msg); err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("send message failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  sent: id=%d", msg.ID)
	result, _ := json.Marshal(map[string]any{
		"id":      msg.ID,
		"status":  "sent",
		"to":      input.To,
		"type":    input.Type,
		"expires": msg.ExpiresAt.Format(time.RFC3339),
	})
	return textResult(string(result)), nil, nil
}

func (s *MCPServer) handleMessageAck(_ context.Context, _ *mcp.CallToolRequest, input MessageAckInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: message_ack id=%d agent=%s", input.MessageID, input.AgentID)

	if input.AgentID == "" {
		return errorResult("'agent_id' is required"), nil, nil
	}

	if err := s.store.AckMessage(input.MessageID, input.AgentID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errorResult(fmt.Sprintf("message %d not found", input.MessageID)), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("ack message failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  acked")
	return textResult(fmt.Sprintf("Message %d acknowledged by %s", input.MessageID, input.AgentID)), nil, nil
}

// ============================================================================
// Usage Tools (Claude Code token usage statistics)
// ============================================================================
