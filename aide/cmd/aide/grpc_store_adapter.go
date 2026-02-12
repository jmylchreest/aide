// Package main provides a gRPC-backed store.Store adapter.
// This allows MCP server instances to share a single BoltDB
// via the gRPC socket when another MCP instance is already the primary.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/oklog/ulid/v2"
)

// grpcStoreAdapter implements store.Store by delegating to a gRPC client.
// It allows secondary MCP instances to operate without opening BoltDB directly.
type grpcStoreAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that grpcStoreAdapter implements store.Store.
var _ store.Store = (*grpcStoreAdapter)(nil)

func newGRPCStoreAdapter(client *grpcapi.Client) *grpcStoreAdapter {
	return &grpcStoreAdapter{client: client}
}

func (g *grpcStoreAdapter) Close() error {
	return g.client.Close()
}

// =============================================================================
// MemoryStore
// =============================================================================

func (g *grpcStoreAdapter) AddMemory(m *memory.Memory) error {
	ctx := context.Background()
	if m.ID == "" {
		m.ID = ulid.Make().String()
	}
	resp, err := g.client.Memory.Add(ctx, &grpcapi.MemoryAddRequest{
		Content:  m.Content,
		Category: string(m.Category),
		Tags:     m.Tags,
	})
	if err != nil {
		return err
	}
	m.ID = resp.Memory.Id
	m.CreatedAt = resp.Memory.CreatedAt.AsTime()
	return nil
}

func (g *grpcStoreAdapter) GetMemory(id string) (*memory.Memory, error) {
	ctx := context.Background()
	resp, err := g.client.Memory.Get(ctx, &grpcapi.MemoryGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return protoToMemory(resp.Memory), nil
}

func (g *grpcStoreAdapter) UpdateMemory(m *memory.Memory) error {
	// gRPC has no native update — delete and re-add.
	ctx := context.Background()
	if _, err := g.client.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: m.ID}); err != nil {
		return err
	}
	_, err := g.client.Memory.Add(ctx, &grpcapi.MemoryAddRequest{
		Content:  m.Content,
		Category: string(m.Category),
		Tags:     m.Tags,
	})
	return err
}

func (g *grpcStoreAdapter) DeleteMemory(id string) error {
	ctx := context.Background()
	_, err := g.client.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: id})
	return err
}

func (g *grpcStoreAdapter) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
	ctx := context.Background()
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	resp, err := g.client.Memory.List(ctx, &grpcapi.MemoryListRequest{
		Category: string(opts.Category),
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return protoToMemories(resp.Memories), nil
}

func (g *grpcStoreAdapter) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
	ctx := context.Background()
	resp, err := g.client.Memory.Search(ctx, &grpcapi.MemorySearchRequest{
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return protoToMemories(resp.Memories), nil
}

func (g *grpcStoreAdapter) ClearMemories() (int, error) {
	ctx := context.Background()
	resp, err := g.client.Memory.Clear(ctx, &grpcapi.MemoryClearRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// StateStore
// =============================================================================

func (g *grpcStoreAdapter) SetState(st *memory.State) error {
	ctx := context.Background()
	_, err := g.client.State.Set(ctx, &grpcapi.StateSetRequest{
		Key:     st.Key,
		Value:   st.Value,
		AgentId: st.Agent,
	})
	return err
}

func (g *grpcStoreAdapter) GetState(key string) (*memory.State, error) {
	ctx := context.Background()
	resp, err := g.client.State.Get(ctx, &grpcapi.StateGetRequest{Key: key})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return protoToState(resp.State), nil
}

func (g *grpcStoreAdapter) DeleteState(key string) error {
	ctx := context.Background()
	_, err := g.client.State.Delete(ctx, &grpcapi.StateDeleteRequest{Key: key})
	return err
}

func (g *grpcStoreAdapter) ListState(agentFilter string) ([]*memory.State, error) {
	ctx := context.Background()
	resp, err := g.client.State.List(ctx, &grpcapi.StateListRequest{AgentId: agentFilter})
	if err != nil {
		return nil, err
	}
	return protoToStates(resp.States), nil
}

func (g *grpcStoreAdapter) ClearState(agentID string) (int, error) {
	ctx := context.Background()
	resp, err := g.client.State.Clear(ctx, &grpcapi.StateClearRequest{AgentId: agentID})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *grpcStoreAdapter) CleanupStaleState(maxAge time.Duration) (int, error) {
	ctx := context.Background()
	resp, err := g.client.State.Cleanup(ctx, &grpcapi.StateCleanupRequest{MaxAge: maxAge.String()})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// DecisionStore
// =============================================================================

func (g *grpcStoreAdapter) SetDecision(d *memory.Decision) error {
	ctx := context.Background()
	resp, err := g.client.Decision.Set(ctx, &grpcapi.DecisionSetRequest{
		Topic:      d.Topic,
		Decision:   d.Decision,
		Rationale:  d.Rationale,
		Details:    d.Details,
		References: d.References,
		DecidedBy:  d.DecidedBy,
	})
	if err != nil {
		return err
	}
	d.CreatedAt = resp.Decision.CreatedAt.AsTime()
	return nil
}

func (g *grpcStoreAdapter) GetDecision(topic string) (*memory.Decision, error) {
	ctx := context.Background()
	resp, err := g.client.Decision.Get(ctx, &grpcapi.DecisionGetRequest{Topic: topic})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return protoToDecision(resp.Decision), nil
}

func (g *grpcStoreAdapter) ListDecisions() ([]*memory.Decision, error) {
	ctx := context.Background()
	resp, err := g.client.Decision.List(ctx, &grpcapi.DecisionListRequest{})
	if err != nil {
		return nil, err
	}
	return protoToDecisions(resp.Decisions), nil
}

func (g *grpcStoreAdapter) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	ctx := context.Background()
	resp, err := g.client.Decision.History(ctx, &grpcapi.DecisionHistoryRequest{Topic: topic})
	if err != nil {
		return nil, err
	}
	return protoToDecisions(resp.Decisions), nil
}

func (g *grpcStoreAdapter) DeleteDecision(topic string) (int, error) {
	ctx := context.Background()
	resp, err := g.client.Decision.Delete(ctx, &grpcapi.DecisionDeleteRequest{Topic: topic})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *grpcStoreAdapter) ClearDecisions() (int, error) {
	ctx := context.Background()
	resp, err := g.client.Decision.Clear(ctx, &grpcapi.DecisionClearRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// MessageStore
// =============================================================================

func (g *grpcStoreAdapter) AddMessage(m *memory.Message) error {
	ctx := context.Background()
	ttl := int32(3600)
	if !m.ExpiresAt.IsZero() && !m.CreatedAt.IsZero() {
		ttl = int32(m.ExpiresAt.Sub(m.CreatedAt).Seconds())
	}
	resp, err := g.client.Message.Send(ctx, &grpcapi.MessageSendRequest{
		From:       m.From,
		To:         m.To,
		Content:    m.Content,
		Type:       m.Type,
		TtlSeconds: ttl,
	})
	if err != nil {
		return err
	}
	m.ID = resp.Message.Id
	m.CreatedAt = resp.Message.CreatedAt.AsTime()
	m.ExpiresAt = resp.Message.ExpiresAt.AsTime()
	return nil
}

func (g *grpcStoreAdapter) GetMessages(agentID string) ([]*memory.Message, error) {
	ctx := context.Background()
	resp, err := g.client.Message.List(ctx, &grpcapi.MessageListRequest{AgentId: agentID})
	if err != nil {
		return nil, err
	}
	return protoToMessages(resp.Messages), nil
}

func (g *grpcStoreAdapter) AckMessage(messageID uint64, agentID string) error {
	ctx := context.Background()
	_, err := g.client.Message.Ack(ctx, &grpcapi.MessageAckRequest{
		MessageId: messageID,
		AgentId:   agentID,
	})
	return err
}

func (g *grpcStoreAdapter) PruneMessages() (int, error) {
	ctx := context.Background()
	resp, err := g.client.Message.Prune(ctx, &grpcapi.MessagePruneRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// TaskStore
// =============================================================================

func (g *grpcStoreAdapter) CreateTask(t *memory.Task) error {
	ctx := context.Background()
	resp, err := g.client.Task.Create(ctx, &grpcapi.TaskCreateRequest{
		Title:       t.Title,
		Description: t.Description,
	})
	if err != nil {
		return err
	}
	t.ID = resp.Task.Id
	t.CreatedAt = resp.Task.CreatedAt.AsTime()
	return nil
}

func (g *grpcStoreAdapter) GetTask(id string) (*memory.Task, error) {
	ctx := context.Background()
	resp, err := g.client.Task.Get(ctx, &grpcapi.TaskGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return protoToTask(resp.Task), nil
}

func (g *grpcStoreAdapter) ListTasks(status memory.TaskStatus) ([]*memory.Task, error) {
	ctx := context.Background()
	resp, err := g.client.Task.List(ctx, &grpcapi.TaskListRequest{Status: string(status)})
	if err != nil {
		return nil, err
	}
	return protoToTasks(resp.Tasks), nil
}

func (g *grpcStoreAdapter) ClaimTask(taskID, agentID string) (*memory.Task, error) {
	ctx := context.Background()
	resp, err := g.client.Task.Claim(ctx, &grpcapi.TaskClaimRequest{
		TaskId:  taskID,
		AgentId: agentID,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return protoToTask(resp.Task), nil
}

func (g *grpcStoreAdapter) CompleteTask(taskID, result string) error {
	ctx := context.Background()
	resp, err := g.client.Task.Complete(ctx, &grpcapi.TaskCompleteRequest{
		TaskId: taskID,
		Result: result,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to complete task")
	}
	return nil
}

func (g *grpcStoreAdapter) UpdateTask(t *memory.Task) error {
	ctx := context.Background()
	resp, err := g.client.Task.Update(ctx, &grpcapi.TaskUpdateRequest{
		TaskId: t.ID,
		Status: string(t.Status),
		Result: t.Result,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to update task")
	}
	return nil
}

func (g *grpcStoreAdapter) DeleteTask(id string) error {
	return fmt.Errorf("task delete not supported in gRPC mode")
}

func (g *grpcStoreAdapter) ClearTasks(status memory.TaskStatus) (int, error) {
	return 0, fmt.Errorf("task clear not supported in gRPC mode")
}
