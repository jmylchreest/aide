package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// RPCTimeout is the default timeout for gRPC calls from adapters.
const RPCTimeout = 10 * time.Second

// Default constants for adapter behavior.
const (
	DefaultMemoryListLimit   = 50
	DefaultMessageTTLSeconds = 3600
)

// StoreAdapter implements store.Store by delegating to a gRPC client.
// It allows secondary processes to operate without opening BoltDB directly.
type StoreAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that StoreAdapter implements store.Store.
var _ store.Store = (*StoreAdapter)(nil)

// NewStoreAdapter creates a new gRPC-backed store adapter.
func NewStoreAdapter(client *grpcapi.Client) *StoreAdapter {
	return &StoreAdapter{client: client}
}

func (g *StoreAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RPCTimeout)
}

func (g *StoreAdapter) Close() error {
	return g.client.Close()
}

// =============================================================================
// MemoryStore
// =============================================================================

func (g *StoreAdapter) AddMemory(m *memory.Memory) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Memory.Add(ctx, &grpcapi.MemoryAddRequest{
		Content:  m.Content,
		Category: string(m.Category),
		Tags:     m.Tags,
	})
	if err != nil {
		return err
	}
	if resp.Memory == nil {
		return fmt.Errorf("server returned nil memory in add response")
	}
	m.ID = resp.Memory.Id
	m.CreatedAt = resp.Memory.CreatedAt.AsTime()
	return nil
}

func (g *StoreAdapter) GetMemory(id string) (*memory.Memory, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Memory.Get(ctx, &grpcapi.MemoryGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return ProtoToMemory(resp.Memory), nil
}

func (g *StoreAdapter) UpdateMemory(m *memory.Memory) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
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

func (g *StoreAdapter) DeleteMemory(id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: id})
	return err
}

func (g *StoreAdapter) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultMemoryListLimit
	}
	resp, err := g.client.Memory.List(ctx, &grpcapi.MemoryListRequest{
		Category: string(opts.Category),
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return ProtoToMemories(resp.Memories), nil
}

func (g *StoreAdapter) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Memory.Search(ctx, &grpcapi.MemorySearchRequest{
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return ProtoToMemories(resp.Memories), nil
}

func (g *StoreAdapter) ClearMemories() (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Memory.Clear(ctx, &grpcapi.MemoryClearRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *StoreAdapter) TouchMemory(ids []string) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Memory.Touch(ctx, &grpcapi.MemoryTouchRequest{Ids: ids})
	if err != nil {
		return 0, err
	}
	return int(resp.Touched), nil
}

// =============================================================================
// StateStore
// =============================================================================

func (g *StoreAdapter) SetState(st *memory.State) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.State.Set(ctx, &grpcapi.StateSetRequest{
		Key:     st.Key,
		Value:   st.Value,
		AgentId: st.Agent,
	})
	return err
}

func (g *StoreAdapter) GetState(key string) (*memory.State, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.State.Get(ctx, &grpcapi.StateGetRequest{Key: key})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return ProtoToState(resp.State), nil
}

func (g *StoreAdapter) DeleteState(key string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.State.Delete(ctx, &grpcapi.StateDeleteRequest{Key: key})
	return err
}

func (g *StoreAdapter) ListState(agentFilter string) ([]*memory.State, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.State.List(ctx, &grpcapi.StateListRequest{AgentId: agentFilter})
	if err != nil {
		return nil, err
	}
	return ProtoToStates(resp.States), nil
}

func (g *StoreAdapter) ClearState(agentID string) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.State.Clear(ctx, &grpcapi.StateClearRequest{AgentId: agentID})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *StoreAdapter) CleanupStaleState(maxAge time.Duration) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.State.Cleanup(ctx, &grpcapi.StateCleanupRequest{MaxAge: maxAge.String()})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// DecisionStore
// =============================================================================

func (g *StoreAdapter) SetDecision(d *memory.Decision) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
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
	if resp.Decision == nil {
		return fmt.Errorf("server returned nil decision in set response")
	}
	d.CreatedAt = resp.Decision.CreatedAt.AsTime()
	return nil
}

func (g *StoreAdapter) GetDecision(topic string) (*memory.Decision, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Decision.Get(ctx, &grpcapi.DecisionGetRequest{Topic: topic})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return ProtoToDecision(resp.Decision), nil
}

func (g *StoreAdapter) ListDecisions() ([]*memory.Decision, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Decision.List(ctx, &grpcapi.DecisionListRequest{})
	if err != nil {
		return nil, err
	}
	return ProtoToDecisions(resp.Decisions), nil
}

func (g *StoreAdapter) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Decision.History(ctx, &grpcapi.DecisionHistoryRequest{Topic: topic})
	if err != nil {
		return nil, err
	}
	return ProtoToDecisions(resp.Decisions), nil
}

func (g *StoreAdapter) DeleteDecision(topic string) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Decision.Delete(ctx, &grpcapi.DecisionDeleteRequest{Topic: topic})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *StoreAdapter) ClearDecisions() (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Decision.Clear(ctx, &grpcapi.DecisionClearRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// MessageStore
// =============================================================================

func (g *StoreAdapter) AddMessage(m *memory.Message) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	ttl := int32(DefaultMessageTTLSeconds)
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
	if resp.Message == nil {
		return fmt.Errorf("server returned nil message in send response")
	}
	m.ID = resp.Message.Id
	m.CreatedAt = resp.Message.CreatedAt.AsTime()
	m.ExpiresAt = resp.Message.ExpiresAt.AsTime()
	return nil
}

func (g *StoreAdapter) GetMessages(agentID string) ([]*memory.Message, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Message.List(ctx, &grpcapi.MessageListRequest{AgentId: agentID})
	if err != nil {
		return nil, err
	}
	return ProtoToMessages(resp.Messages), nil
}

func (g *StoreAdapter) AckMessage(messageID uint64, agentID string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.Message.Ack(ctx, &grpcapi.MessageAckRequest{
		MessageId: messageID,
		AgentId:   agentID,
	})
	return err
}

func (g *StoreAdapter) PruneMessages() (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Message.Prune(ctx, &grpcapi.MessagePruneRequest{})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// =============================================================================
// TaskStore
// =============================================================================

func (g *StoreAdapter) CreateTask(t *memory.Task) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Task.Create(ctx, &grpcapi.TaskCreateRequest{
		Title:       t.Title,
		Description: t.Description,
	})
	if err != nil {
		return err
	}
	if resp.Task == nil {
		return fmt.Errorf("server returned nil task in create response")
	}
	t.ID = resp.Task.Id
	t.CreatedAt = resp.Task.CreatedAt.AsTime()
	return nil
}

func (g *StoreAdapter) GetTask(id string) (*memory.Task, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Task.Get(ctx, &grpcapi.TaskGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}
	return ProtoToTask(resp.Task), nil
}

func (g *StoreAdapter) ListTasks(status memory.TaskStatus) ([]*memory.Task, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Task.List(ctx, &grpcapi.TaskListRequest{Status: string(status)})
	if err != nil {
		return nil, err
	}
	return ProtoToTasks(resp.Tasks), nil
}

func (g *StoreAdapter) ClaimTask(taskID, agentID string) (*memory.Task, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
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
	return ProtoToTask(resp.Task), nil
}

func (g *StoreAdapter) CompleteTask(taskID, result string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.Task.Complete(ctx, &grpcapi.TaskCompleteRequest{
		TaskId: taskID,
		Result: result,
	})
	return err
}

func (g *StoreAdapter) UpdateTask(t *memory.Task) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.Task.Update(ctx, &grpcapi.TaskUpdateRequest{
		TaskId: t.ID,
		Status: string(t.Status),
		Result: t.Result,
	})
	return err
}

func (g *StoreAdapter) DeleteTask(id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	_, err := g.client.Task.Delete(ctx, &grpcapi.TaskDeleteRequest{Id: id})
	return err
}

func (g *StoreAdapter) ClearTasks(status memory.TaskStatus) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()
	resp, err := g.client.Task.Clear(ctx, &grpcapi.TaskClearRequest{Status: string(status)})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}
