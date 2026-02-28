package main

import (
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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
// When using gRPC, scores are unavailable (returned as 0).
// When using direct DB, it uses CombinedStore's bleve-backed scored search.
// excludeTags filters out memories with any of the given tags (nil = DefaultExcludeTags).
func (b *Backend) SearchMemoriesWithScore(query string, limit int, minScore float64, excludeTags []string) ([]SearchResult, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Search(ctx, &grpcapi.MemorySearchRequest{
			Query: query,
			Limit: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		memories := make([]*memory.Memory, len(resp.Memories))
		for i, m := range resp.Memories {
			memories[i] = protoToMemory(m)
		}
		memories = memory.FilterMemories(memories, excludeTags)
		results := make([]SearchResult, len(memories))
		for i, m := range memories {
			results[i] = SearchResult{Memory: m, Score: 0}
		}
		return results, nil
	}

	// Use the backend's combined store directly (no double-open)
	if b.combined == nil {
		// Fallback: bolt-only store, no scores available
		memories, err := b.store.SearchMemories(query, limit)
		if err != nil {
			return nil, err
		}
		if excludeTags != nil {
			memories = memory.FilterMemories(memories, excludeTags)
		}
		results := make([]SearchResult, len(memories))
		for i, m := range memories {
			results[i] = SearchResult{Memory: m, Score: 0}
		}
		return results, nil
	}

	storeResults, err := b.combined.SearchMemoriesWithScore(query, limit, excludeTags)
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

// ListMemories returns memories matching the given category.
// opts allows optional overrides (ExcludeTags, IncludeAll, etc.). If nil, defaults apply.
func (b *Backend) ListMemories(category string, limit int, opts *memory.SearchOptions) ([]*memory.Memory, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Memory.List(ctx, &grpcapi.MemoryListRequest{
			Category: category,
			Limit:    int32(limit),
		})
		if err != nil {
			return nil, err
		}
		memories := protoToMemories(resp.Memories)
		// Apply exclude-tag filtering for gRPC results
		if opts != nil && opts.IncludeAll {
			return memories, nil
		}
		excludeTags := memory.DefaultExcludeTags
		if opts != nil && opts.ExcludeTags != nil {
			excludeTags = opts.ExcludeTags
		}
		return memory.FilterMemories(memories, excludeTags), nil
	}

	searchOpts := memory.SearchOptions{
		Category: memory.Category(category),
		Limit:    limit,
	}
	if opts != nil {
		searchOpts.ExcludeTags = opts.ExcludeTags
		searchOpts.IncludeAll = opts.IncludeAll
	}
	return b.store.ListMemories(searchOpts)
}

// UpdateMemoryTags adds and/or removes tags from a memory.
// Returns the updated memory.
func (b *Backend) UpdateMemoryTags(id string, addTags, removeTags []string) (*memory.Memory, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	// Get existing memory
	var m *memory.Memory
	if b.useGRPC {
		resp, err := b.grpcClient.Memory.Get(ctx, &grpcapi.MemoryGetRequest{Id: id})
		if err != nil {
			return nil, fmt.Errorf("memory not found: %w", err)
		}
		m = protoToMemory(resp.Memory)
		if m == nil {
			return nil, fmt.Errorf("memory not found: server returned nil for id %s", id)
		}
	} else {
		var err error
		m, err = b.store.GetMemory(id)
		if err != nil {
			return nil, fmt.Errorf("memory not found: %w", err)
		}
	}

	// Build new tag set
	tagSet := make(map[string]bool)
	for _, t := range m.Tags {
		tagSet[t] = true
	}
	for _, t := range removeTags {
		delete(tagSet, t)
	}
	for _, t := range addTags {
		tagSet[t] = true
	}

	// Convert back to slice
	newTags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		newTags = append(newTags, t)
	}
	m.Tags = newTags
	m.UpdatedAt = time.Now()

	// Update via appropriate backend
	if b.useGRPC {
		// gRPC has no native update — delete and re-add (preserves content/category)
		if _, err := b.grpcClient.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: m.ID}); err != nil {
			return nil, fmt.Errorf("failed to delete old memory for tag update: %w", err)
		}
		resp, err := b.grpcClient.Memory.Add(ctx, &grpcapi.MemoryAddRequest{
			Content:  m.Content,
			Category: string(m.Category),
			Tags:     m.Tags,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to re-add memory with updated tags: %w", err)
		}
		return protoToMemory(resp.Memory), nil
	}

	if err := b.store.UpdateMemory(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (b *Backend) GetMemory(id string) (*memory.Memory, error) {
	if b.useGRPC {
		ctx, cancel := b.rpcCtx()
		defer cancel()
		resp, err := b.grpcClient.Memory.Get(ctx, &grpcapi.MemoryGetRequest{Id: id})
		if err != nil {
			return nil, err
		}
		return protoToMemory(resp.Memory), nil
	}
	return b.store.GetMemory(id)
}

func (b *Backend) DeleteMemory(id string) error {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		_, err := b.grpcClient.Memory.Delete(ctx, &grpcapi.MemoryDeleteRequest{Id: id})
		return err
	}

	return b.store.DeleteMemory(id)
}

func (b *Backend) ClearMemories() (int, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		_, err := b.grpcClient.State.Delete(ctx, &grpcapi.StateDeleteRequest{Key: key})
		return err
	}

	return b.store.DeleteState(key)
}

func (b *Backend) ClearState(agentID string) (int, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
		ttlSeconds = DefaultMessageTTLSeconds
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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

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
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		_, err := b.grpcClient.Task.Complete(ctx, &grpcapi.TaskCompleteRequest{
			TaskId: taskID,
			Result: result,
		})
		return err
	}

	return b.store.CompleteTask(taskID, result)
}

func (b *Backend) DeleteTask(taskID string) error {
	return b.store.DeleteTask(taskID)
}

func (b *Backend) ClearTasks(status string) (int, error) {
	return b.store.ClearTasks(memory.TaskStatus(status))
}
