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

// =============================================================================
// Memory Operations
// =============================================================================

func (b *Backend) AddMemory(content, category string, tags []string) (*memory.Memory, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Add(ctx, &grpcapi.MemoryAddRequest{
			Content:  content,
			Category: category,
			Tags:     tags,
		})
		if err != nil {
			return nil, err
		}
		return protoToMemory(resp.Memory), nil
	}

	mem := &memory.Memory{
		ID:        ulid.Make().String(),
		Content:   content,
		Category:  memory.Category(category),
		Tags:      tags,
		CreatedAt: time.Now(),
	}
	if err := b.store.AddMemory(mem); err != nil {
		return nil, err
	}
	return mem, nil
}

func (b *Backend) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Search(ctx, &grpcapi.MemorySearchRequest{
			Query: query,
			Limit: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		return protoToMemories(resp.Memories), nil
	}

	return b.store.SearchMemories(query, limit)
}

// SearchResult wraps a memory with its search score.
type SearchResult struct {
	Memory *memory.Memory
	Score  float64
}

// SearchMemoriesWithScore returns search results with relevance scores.
func (b *Backend) SearchMemoriesWithScore(query string, limit int, minScore float64) ([]SearchResult, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Search(ctx, &grpcapi.MemorySearchRequest{
			Query: query,
			Limit: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		results := make([]SearchResult, len(resp.Memories))
		for i, m := range resp.Memories {
			results[i] = SearchResult{Memory: protoToMemory(m), Score: 0}
		}
		return results, nil
	}

	combined, err := store.NewCombinedStore(b.dbPath)
	if err != nil {
		return nil, err
	}
	defer combined.Close()

	storeResults, err := combined.SearchMemories(query, limit)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, r := range storeResults {
		if r.Score >= minScore {
			results = append(results, SearchResult{
				Memory: r.Memory,
				Score:  r.Score,
			})
		}
	}
	return results, nil
}

func (b *Backend) ListMemories(category string, limit int) ([]*memory.Memory, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.List(ctx, &grpcapi.MemoryListRequest{
			Category: category,
			Limit:    int32(limit),
		})
		if err != nil {
			return nil, err
		}
		return protoToMemories(resp.Memories), nil
	}

	opts := memory.SearchOptions{
		Category: memory.Category(category),
		Limit:    limit,
	}
	return b.store.ListMemories(opts)
}

func (b *Backend) DeleteMemory(id string) error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: id})
		return err
	}

	return b.store.DeleteMemory(id)
}

func (b *Backend) ClearMemories() (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Clear(ctx, &grpcapi.MemoryClearRequest{})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.ClearMemories()
}

// =============================================================================
// State Operations
// =============================================================================

func (b *Backend) GetState(key, agentID string) (*memory.State, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.State.Get(ctx, &grpcapi.StateGetRequest{
			Key:     key,
			AgentId: agentID,
		})
		if err != nil {
			return nil, err
		}
		if !resp.Found {
			return nil, store.ErrNotFound
		}
		return protoToState(resp.State), nil
	}

	fullKey := key
	if agentID != "" {
		fullKey = fmt.Sprintf("agent:%s:%s", agentID, key)
	}
	return b.store.GetState(fullKey)
}

func (b *Backend) SetState(key, value, agentID string) error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.State.Set(ctx, &grpcapi.StateSetRequest{
			Key:     key,
			Value:   value,
			AgentId: agentID,
		})
		return err
	}

	fullKey := key
	if agentID != "" {
		fullKey = fmt.Sprintf("agent:%s:%s", agentID, key)
	}
	st := &memory.State{
		Key:   fullKey,
		Value: value,
		Agent: agentID,
	}
	return b.store.SetState(st)
}

func (b *Backend) ListState(agentID string) ([]*memory.State, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.State.List(ctx, &grpcapi.StateListRequest{
			AgentId: agentID,
		})
		if err != nil {
			return nil, err
		}
		return protoToStates(resp.States), nil
	}

	return b.store.ListState(agentID)
}

func (b *Backend) DeleteState(key string) error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.State.Delete(ctx, &grpcapi.StateDeleteRequest{Key: key})
		return err
	}

	return b.store.DeleteState(key)
}

func (b *Backend) ClearState(agentID string) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.State.Clear(ctx, &grpcapi.StateClearRequest{AgentId: agentID})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.ClearState(agentID)
}

func (b *Backend) CleanupState(maxAge time.Duration) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.State.Cleanup(ctx, &grpcapi.StateCleanupRequest{
			MaxAge: maxAge.String(),
		})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.CleanupStaleState(maxAge)
}

// =============================================================================
// Decision Operations
// =============================================================================

func (b *Backend) SetDecision(topic, decision, rationale, details, decidedBy string, references []string) (*memory.Decision, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.Set(ctx, &grpcapi.DecisionSetRequest{
			Topic:      topic,
			Decision:   decision,
			Rationale:  rationale,
			Details:    details,
			References: references,
			DecidedBy:  decidedBy,
		})
		if err != nil {
			return nil, err
		}
		return protoToDecision(resp.Decision), nil
	}

	dec := &memory.Decision{
		Topic:      topic,
		Decision:   decision,
		Rationale:  rationale,
		Details:    details,
		References: references,
		DecidedBy:  decidedBy,
		CreatedAt:  time.Now(),
	}
	if err := b.store.SetDecision(dec); err != nil {
		return nil, err
	}
	return dec, nil
}

func (b *Backend) GetDecision(topic string) (*memory.Decision, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.Get(ctx, &grpcapi.DecisionGetRequest{Topic: topic})
		if err != nil {
			return nil, err
		}
		if !resp.Found {
			return nil, store.ErrNotFound
		}
		return protoToDecision(resp.Decision), nil
	}

	return b.store.GetDecision(topic)
}

func (b *Backend) ListDecisions() ([]*memory.Decision, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.List(ctx, &grpcapi.DecisionListRequest{})
		if err != nil {
			return nil, err
		}
		return protoToDecisions(resp.Decisions), nil
	}

	return b.store.ListDecisions()
}

func (b *Backend) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.History(ctx, &grpcapi.DecisionHistoryRequest{Topic: topic})
		if err != nil {
			return nil, err
		}
		return protoToDecisions(resp.Decisions), nil
	}

	return b.store.GetDecisionHistory(topic)
}

func (b *Backend) DeleteDecision(topic string) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.Delete(ctx, &grpcapi.DecisionDeleteRequest{Topic: topic})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.DeleteDecision(topic)
}

func (b *Backend) ClearDecisions() (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Decision.Clear(ctx, &grpcapi.DecisionClearRequest{})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.ClearDecisions()
}

// =============================================================================
// Message Operations
// =============================================================================

func (b *Backend) SendMessage(from, to, content, msgType string, ttlSeconds int) (*memory.Message, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Message.Send(ctx, &grpcapi.MessageSendRequest{
			From:       from,
			To:         to,
			Content:    content,
			Type:       msgType,
			TtlSeconds: int32(ttlSeconds),
		})
		if err != nil {
			return nil, err
		}
		return protoToMessage(resp.Message), nil
	}

	if ttlSeconds == 0 {
		ttlSeconds = 3600
	}
	msg := &memory.Message{
		From:      from,
		To:        to,
		Content:   content,
		Type:      msgType,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
	if err := b.store.AddMessage(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (b *Backend) ListMessages(agentID string) ([]*memory.Message, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Message.List(ctx, &grpcapi.MessageListRequest{AgentId: agentID})
		if err != nil {
			return nil, err
		}
		return protoToMessages(resp.Messages), nil
	}

	return b.store.GetMessages(agentID)
}

func (b *Backend) AckMessage(messageID uint64, agentID string) error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.Message.Ack(ctx, &grpcapi.MessageAckRequest{
			MessageId: messageID,
			AgentId:   agentID,
		})
		return err
	}

	return b.store.AckMessage(messageID, agentID)
}

func (b *Backend) PruneMessages() (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Message.Prune(ctx, &grpcapi.MessagePruneRequest{})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	return b.store.PruneMessages()
}

// =============================================================================
// Task Operations
// =============================================================================

func (b *Backend) CreateTask(title, description string) (*memory.Task, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Task.Create(ctx, &grpcapi.TaskCreateRequest{
			Title:       title,
			Description: description,
		})
		if err != nil {
			return nil, err
		}
		return protoToTask(resp.Task), nil
	}

	task := &memory.Task{
		Title:       title,
		Description: description,
		Status:      memory.TaskStatusPending,
		CreatedAt:   time.Now(),
	}
	if err := b.store.CreateTask(task); err != nil {
		return nil, err
	}
	return task, nil
}

func (b *Backend) GetTask(id string) (*memory.Task, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Task.Get(ctx, &grpcapi.TaskGetRequest{Id: id})
		if err != nil {
			return nil, err
		}
		if !resp.Found {
			return nil, store.ErrNotFound
		}
		return protoToTask(resp.Task), nil
	}

	return b.store.GetTask(id)
}

func (b *Backend) ListTasks(status string) ([]*memory.Task, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Task.List(ctx, &grpcapi.TaskListRequest{Status: status})
		if err != nil {
			return nil, err
		}
		return protoToTasks(resp.Tasks), nil
	}

	return b.store.ListTasks(memory.TaskStatus(status))
}

func (b *Backend) ClaimTask(taskID, agentID string) (*memory.Task, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Task.Claim(ctx, &grpcapi.TaskClaimRequest{
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

	return b.store.ClaimTask(taskID, agentID)
}

func (b *Backend) CompleteTask(taskID, result string) error {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Task.Complete(ctx, &grpcapi.TaskCompleteRequest{
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

	return b.store.CompleteTask(taskID, result)
}

func (b *Backend) DeleteTask(taskID string) error {
	// gRPC not implemented for delete - local only
	if b.useGRPC {
		return fmt.Errorf("task delete not supported in gRPC mode")
	}

	return b.store.DeleteTask(taskID)
}

func (b *Backend) ClearTasks(status string) (int, error) {
	// gRPC not implemented for clear - local only
	if b.useGRPC {
		return 0, fmt.Errorf("task clear not supported in gRPC mode")
	}

	return b.store.ClearTasks(memory.TaskStatus(status))
}
