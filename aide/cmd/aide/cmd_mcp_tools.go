package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// Input types for memory, state, decision, message, and usage tools
// ============================================================================

type MemorySearchInput struct {
	Query    string `json:"query" jsonschema:"Search query - uses bleve full-text search with: (1) standard word matching, (2) fuzzy matching for typos (fuzziness=1), (3) edge n-grams for prefix matching (2-15 chars), (4) n-grams for substring matching (3-8 chars). Use simple keywords like 'colour' or 'favourite food'. For comprehensive recall, perform MULTIPLE searches with different terms (e.g., search 'colour' AND 'color', 'favourite' AND 'favorite')."`
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
	Topic string `json:"topic" jsonschema:"Decision topic key, e.g., 'auth-strategy', 'testing-framework', 'db-schema'"`
}

type DecisionHistoryInput struct {
	Topic string `json:"topic" jsonschema:"Decision topic to get full history for (all versions in chronological order)"`
}

type MessageListInput struct {
	AgentID     string `json:"agent_id" jsonschema:"Your agent ID to receive messages for (required)"`
	IncludeRead bool   `json:"include_read,omitempty" jsonschema:"Include already-acknowledged messages (default false)"`
}

type emptyInput struct{}

type UsageToolInput struct{}

// ============================================================================
// Memory Tools (read-only)
// ============================================================================

func (s *MCPServer) registerMemoryTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "memory_search",
		Description: `Search stored memories using bleve full-text search.

**Search capabilities:**
- Standard word matching (case-insensitive)
- Fuzzy matching for typos (1 edit distance)
- Prefix matching via edge n-grams (2-15 chars)
- Substring matching via n-grams (3-8 chars)

**Best practices:**
- Use simple keywords: "colour", "testing framework", "auth"
- Perform MULTIPLE searches with synonyms: search "colour" AND "color"
- Search tags directly: "preferences", "food"
- Results include timestamps - prefer most recent when values conflict

**Output format:**
Returns memories grouped by category with [date] prefix and tags.`,
	}, s.handleMemorySearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "memory_list",
		Description: "List all stored memories, optionally filtered by category. Returns memories with timestamps - prefer most recent when answering questions about preferences or decisions.",
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
		if err == store.ErrNotFound {
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

Decisions are append-only (latest wins). Use decision_history to see evolution.`,
	}, s.handleDecisionGet)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "decision_history",
		Description: `Get the full history of decisions for a topic.

Returns ALL versions of a decision in chronological order.
Useful when you need to understand why a decision changed or evolved.
Each entry includes: decision, rationale, details, references, and timestamp.`,
	}, s.handleDecisionHistory)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "decision_list",
		Description: `List all recorded decisions (latest for each topic).

Returns a summary of all decision topics with their current (most recent) values.
Use this to discover what decisions exist before querying specific topics.
For detailed info on a specific decision, use decision_get with the topic.`,
	}, s.handleDecisionList)
}

func (s *MCPServer) handleDecisionGet(_ context.Context, _ *mcp.CallToolRequest, input DecisionGetInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: decision_get topic=%s", input.Topic)

	dec, err := s.store.GetDecision(input.Topic)
	if err != nil {
		if err == store.ErrNotFound {
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
// Message Tools (read-only - mutations handled by orchestration)
// ============================================================================

func (s *MCPServer) registerMessageTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "message_list",
		Description: `List messages for an agent (auto-prunes expired).

**What are messages?**
Inter-agent communication in swarm mode. Messages can be:
- Broadcasts (to all agents)
- Directed (to specific agent_id)
- Typed (info, warning, error, etc.)

**Parameters:**
- agent_id (required): Your agent ID to receive messages for
- include_read: Set true to see already-acknowledged messages

Messages are sent/acknowledged by orchestration hooks, not directly via tools.
Expired messages (past TTL) are automatically pruned.`,
	}, s.handleMessageList)
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

// ============================================================================
// Usage Tools (Claude Code token usage statistics)
// ============================================================================

func (s *MCPServer) registerUsageTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "usage",
		Description: `Get Claude Code usage statistics.

Returns real-time token usage from JSONL session files:
- **5-hour rolling window**: Input/output tokens, messages, window reset time
- **Today**: Full daily usage summary
- **Historical**: Weekly and all-time totals from stats cache

Use this to monitor token consumption and rate limit proximity.`,
	}, s.handleUsage)
}

func (s *MCPServer) handleUsage(_ context.Context, _ *mcp.CallToolRequest, _ UsageToolInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: usage")

	home, err := os.UserHomeDir()
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get home directory: %v", err)), nil, nil
	}

	// Scan JSONL files for real-time data
	realtime, err := scanJSONLFiles(home)
	if err != nil {
		return errorResult(fmt.Sprintf("scan failed: %v", err)), nil, nil
	}

	// Read stats cache for historical data
	var stats StatsCache
	cachePath := fmt.Sprintf("%s/.claude/stats-cache.json", home)
	if data, readErr := os.ReadFile(cachePath); readErr == nil {
		json.Unmarshal(data, &stats) // Best effort
	}

	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var weekTokens, totalTokens int64
	for _, day := range stats.DailyModelTokens {
		for _, tokens := range day.TokensByModel {
			totalTokens += int64(tokens)
			if day.Date >= weekAgo {
				weekTokens += int64(tokens)
			}
		}
	}
	weekTokens += realtime.Today.Total

	summary := UsageSummary{
		Realtime: realtime,
		Historical: HistUsage{
			Week:     weekTokens,
			AllTime:  totalTokens + realtime.Today.Total,
			Messages: stats.TotalMessages + realtime.MessagesToday,
			Sessions: stats.TotalSessions,
			Since:    stats.FirstSessionDate,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	output, _ := json.MarshalIndent(summary, "", "  ")
	mcpLog.Printf("  5h=%d today=%d", realtime.Window5h.Total, realtime.Today.Total)
	return textResult(string(output)), nil, nil
}
