package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func setupTestDB(t *testing.T) (*BoltStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-memory-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewBoltStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestBoltStoreImplementsStore(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Verify BoltStore satisfies Store interface at runtime
	var _ Store = store
}

// =============================================================================
// Memory Operations
// =============================================================================

func TestMemoryOperations(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("AddAndGetMemory", func(t *testing.T) {
		m := &memory.Memory{
			ID:        "mem-1",
			Category:  memory.CategoryLearning,
			Content:   "Found auth middleware at src/auth.ts",
			Tags:      []string{"auth", "middleware"},
			Priority:  1.0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := store.AddMemory(m)
		if err != nil {
			t.Fatalf("failed to add memory: %v", err)
		}

		got, err := store.GetMemory("mem-1")
		if err != nil {
			t.Fatalf("failed to get memory: %v", err)
		}

		if got.Content != m.Content {
			t.Errorf("content mismatch: got %q, want %q", got.Content, m.Content)
		}
		if got.Category != m.Category {
			t.Errorf("category mismatch: got %q, want %q", got.Category, m.Category)
		}
		if len(got.Tags) != 2 {
			t.Errorf("tags mismatch: got %d tags, want 2", len(got.Tags))
		}
	})

	t.Run("GetNonexistentMemory", func(t *testing.T) {
		_, err := store.GetMemory("nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("DeleteMemory", func(t *testing.T) {
		m := &memory.Memory{
			ID:        "mem-to-delete",
			Category:  memory.CategoryLearning,
			Content:   "Temporary memory",
			CreatedAt: time.Now(),
		}
		if err := store.AddMemory(m); err != nil {
			t.Fatalf("failed to add memory: %v", err)
		}

		_, err := store.GetMemory("mem-to-delete")
		if err != nil {
			t.Fatalf("memory should exist before delete: %v", err)
		}

		if err := store.DeleteMemory("mem-to-delete"); err != nil {
			t.Fatalf("failed to delete memory: %v", err)
		}

		_, err = store.GetMemory("mem-to-delete")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("ListMemoriesWithFilter", func(t *testing.T) {
		memories := []*memory.Memory{
			{ID: "mem-2", Category: memory.CategoryLearning, Content: "Learning 1", CreatedAt: time.Now()},
			{ID: "mem-3", Category: memory.CategoryDecision, Content: "Decision 1", CreatedAt: time.Now()},
			{ID: "mem-4", Category: memory.CategoryLearning, Content: "Learning 2", Tags: []string{"auth"}, CreatedAt: time.Now()},
		}

		for _, m := range memories {
			if err := store.AddMemory(m); err != nil {
				t.Fatalf("failed to add memory: %v", err)
			}
		}

		learnings, err := store.ListMemories(memory.SearchOptions{Category: memory.CategoryLearning})
		if err != nil {
			t.Fatalf("failed to list memories: %v", err)
		}

		learningCount := 0
		for _, m := range learnings {
			if m.Category == memory.CategoryLearning {
				learningCount++
			}
		}
		if learningCount < 2 {
			t.Errorf("expected at least 2 learnings, got %d", learningCount)
		}

		authMemories, err := store.ListMemories(memory.SearchOptions{Tags: []string{"auth"}})
		if err != nil {
			t.Fatalf("failed to list memories: %v", err)
		}

		if len(authMemories) < 1 {
			t.Errorf("expected at least 1 memory with auth tag, got %d", len(authMemories))
		}
	})

	t.Run("ListMemoriesWithLimit", func(t *testing.T) {
		limited, err := store.ListMemories(memory.SearchOptions{Limit: 2})
		if err != nil {
			t.Fatalf("failed to list memories with limit: %v", err)
		}
		if len(limited) > 2 {
			t.Errorf("expected at most 2 memories, got %d", len(limited))
		}
	})

	t.Run("SearchMemories", func(t *testing.T) {
		results, err := store.SearchMemories("auth", 10)
		if err != nil {
			t.Fatalf("failed to search memories: %v", err)
		}
		if len(results) < 1 {
			t.Errorf("expected at least 1 result for 'auth', got %d", len(results))
		}

		results, err = store.SearchMemories("nonexistent-query-xyz", 10)
		if err != nil {
			t.Fatalf("failed to search memories: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for nonexistent query, got %d", len(results))
		}
	})
}

func TestClearMemories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		m := &memory.Memory{
			ID:        fmt.Sprintf("mem-%d", i),
			Category:  memory.CategoryLearning,
			Content:   fmt.Sprintf("Memory %d", i),
			CreatedAt: time.Now(),
		}
		if err := store.AddMemory(m); err != nil {
			t.Fatalf("failed to add memory: %v", err)
		}
	}

	memories, _ := store.ListMemories(memory.SearchOptions{})
	if len(memories) != 5 {
		t.Fatalf("expected 5 memories, got %d", len(memories))
	}

	count, err := store.ClearMemories()
	if err != nil {
		t.Fatalf("failed to clear memories: %v", err)
	}
	if count != 5 {
		t.Errorf("expected to clear 5 memories, got %d", count)
	}

	memories, _ = store.ListMemories(memory.SearchOptions{})
	if len(memories) != 0 {
		t.Errorf("expected 0 memories after clear, got %d", len(memories))
	}
}

// =============================================================================
// State Operations (split into focused tests for reduced complexity)
// =============================================================================

func TestStateSetAndGet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	st := &memory.State{Key: "mode", Value: "eco"}
	if err := store.SetState(st); err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	got, err := store.GetState("mode")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if got.Value != "eco" {
		t.Errorf("expected value 'eco', got %q", got.Value)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestStateOverwrite(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "mode", Value: "eco"})
	store.SetState(&memory.State{Key: "mode", Value: "ralph"})

	got, err := store.GetState("mode")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if got.Value != "ralph" {
		t.Errorf("expected value 'ralph', got %q", got.Value)
	}
}

func TestStateGetNonexistent(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := store.GetState("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStateAgentSpecific(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "mode", Value: "eco"})
	store.SetState(&memory.State{Key: "agent:abc123:status", Value: "active", Agent: "abc123"})
	store.SetState(&memory.State{Key: "agent:def456:status", Value: "idle", Agent: "def456"})

	all, err := store.ListState("")
	if err != nil {
		t.Fatalf("failed to list state: %v", err)
	}
	if len(all) < 3 {
		t.Errorf("expected at least 3 states, got %d", len(all))
	}

	agentStates, err := store.ListState("abc123")
	if err != nil {
		t.Fatalf("failed to list agent state: %v", err)
	}
	if len(agentStates) != 1 {
		t.Errorf("expected 1 state for abc123, got %d", len(agentStates))
	}
}

func TestStateDelete(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "mode", Value: "eco"})
	if err := store.DeleteState("mode"); err != nil {
		t.Fatalf("failed to delete state: %v", err)
	}

	_, err := store.GetState("mode")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStateClearAgent(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "agent:abc123:status", Value: "active", Agent: "abc123"})
	store.SetState(&memory.State{Key: "agent:def456:status", Value: "idle", Agent: "def456"})

	count, err := store.ClearState("abc123")
	if err != nil {
		t.Fatalf("failed to clear agent state: %v", err)
	}
	if count != 1 {
		t.Errorf("expected to clear 1 state, got %d", count)
	}

	states, err := store.ListState("def456")
	if err != nil {
		t.Fatalf("failed to list state: %v", err)
	}
	if len(states) != 1 {
		t.Errorf("expected 1 state for def456, got %d", len(states))
	}
}

func TestStateClearAll(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "mode", Value: "eco"})
	store.SetState(&memory.State{Key: "agent:a:status", Value: "x", Agent: "a"})

	count, err := store.ClearState("")
	if err != nil {
		t.Fatalf("failed to clear all state: %v", err)
	}
	if count < 1 {
		t.Errorf("expected to clear at least 1 state, got %d", count)
	}

	all, err := store.ListState("")
	if err != nil {
		t.Fatalf("failed to list state: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 states after clear all, got %d", len(all))
	}
}

func TestStateCleanupStale(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetState(&memory.State{Key: "agent:fresh:status", Value: "active", Agent: "fresh"})
	store.SetState(&memory.State{Key: "agent:stale:status", Value: "gone", Agent: "stale"})
	store.SetState(&memory.State{Key: "mode", Value: "eco"}) // global, no agent

	time.Sleep(20 * time.Millisecond)
	count, err := store.CleanupStaleState(10 * time.Millisecond)
	if err != nil {
		t.Fatalf("failed to cleanup stale state: %v", err)
	}
	if count != 2 {
		t.Errorf("expected to clean 2 stale states, got %d", count)
	}

	got, err := store.GetState("mode")
	if err != nil {
		t.Fatalf("global state should survive cleanup: %v", err)
	}
	if got.Value != "eco" {
		t.Errorf("global state value changed")
	}
}

// =============================================================================
// Task Operations
// =============================================================================

func TestTaskOperations(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("CreateAndClaimTask", func(t *testing.T) {
		task := &memory.Task{
			ID:          "task-1",
			Title:       "Implement user model",
			Description: "Create the User struct with validation",
			Status:      memory.TaskStatusPending,
			CreatedAt:   time.Now(),
		}

		err := store.CreateTask(task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}

		got, err := store.GetTask("task-1")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if got.Title != "Implement user model" {
			t.Errorf("title mismatch: got %q", got.Title)
		}

		claimed, err := store.ClaimTask("task-1", "executor-1")
		if err != nil {
			t.Fatalf("failed to claim task: %v", err)
		}

		if claimed.Status != memory.TaskStatusClaimed {
			t.Errorf("expected status claimed, got %s", claimed.Status)
		}
		if claimed.ClaimedBy != "executor-1" {
			t.Errorf("expected claimedBy executor-1, got %s", claimed.ClaimedBy)
		}
		if claimed.ClaimedAt.IsZero() {
			t.Error("expected ClaimedAt to be set")
		}
	})

	t.Run("CannotClaimAlreadyClaimedTask", func(t *testing.T) {
		_, err := store.ClaimTask("task-1", "executor-2")
		if err != ErrAlreadyClaimed {
			t.Errorf("expected ErrAlreadyClaimed, got %v", err)
		}
	})

	t.Run("CompleteTask", func(t *testing.T) {
		err := store.CompleteTask("task-1", "User model created at src/models/user.go")
		if err != nil {
			t.Fatalf("failed to complete task: %v", err)
		}

		tasks, err := store.ListTasks(memory.TaskStatusDone)
		if err != nil {
			t.Fatalf("failed to list tasks: %v", err)
		}

		found := false
		for _, task := range tasks {
			if task.ID == "task-1" {
				found = true
				if task.Result != "User model created at src/models/user.go" {
					t.Errorf("unexpected result: %s", task.Result)
				}
				if task.CompletedAt.IsZero() {
					t.Error("expected CompletedAt to be set")
				}
			}
		}
		if !found {
			t.Error("completed task not found")
		}
	})

	t.Run("ClaimNonexistentTask", func(t *testing.T) {
		_, err := store.ClaimTask("nonexistent", "executor-1")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("GetNonexistentTask", func(t *testing.T) {
		_, err := store.GetTask("nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UpdateTask", func(t *testing.T) {
		task := &memory.Task{
			ID:        "task-update",
			Title:     "Original title",
			Status:    memory.TaskStatusPending,
			CreatedAt: time.Now(),
		}
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}

		task.Title = "Updated title"
		task.Status = memory.TaskStatusBlocked
		if err := store.UpdateTask(task); err != nil {
			t.Fatalf("failed to update task: %v", err)
		}

		got, err := store.GetTask("task-update")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if got.Title != "Updated title" {
			t.Errorf("expected updated title, got %q", got.Title)
		}
		if got.Status != memory.TaskStatusBlocked {
			t.Errorf("expected status blocked, got %s", got.Status)
		}
	})

	t.Run("UpdateNonexistentTask", func(t *testing.T) {
		task := &memory.Task{ID: "nonexistent", Title: "ghost"}
		err := store.UpdateTask(task)
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ListTasksAllStatuses", func(t *testing.T) {
		all, err := store.ListTasks("")
		if err != nil {
			t.Fatalf("failed to list all tasks: %v", err)
		}
		if len(all) < 2 {
			t.Errorf("expected at least 2 tasks, got %d", len(all))
		}
	})
}

// =============================================================================
// Decision Operations
// =============================================================================

func TestDecisionOperations(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("SetAndGetDecision", func(t *testing.T) {
		d := &memory.Decision{
			Topic:      "auth-strategy",
			Decision:   "JWT with refresh tokens",
			Rationale:  "Stateless, scalable, works well with microservices",
			Details:    "Use RS256 signing",
			References: []string{"https://jwt.io"},
			DecidedBy:  "architect-1",
			CreatedAt:  time.Now(),
		}

		err := store.SetDecision(d)
		if err != nil {
			t.Fatalf("failed to set decision: %v", err)
		}

		got, err := store.GetDecision("auth-strategy")
		if err != nil {
			t.Fatalf("failed to get decision: %v", err)
		}

		if got.Decision != d.Decision {
			t.Errorf("decision mismatch: got %q, want %q", got.Decision, d.Decision)
		}
		if got.Details != "Use RS256 signing" {
			t.Errorf("details mismatch: got %q", got.Details)
		}
	})

	t.Run("AppendOnlyDecisions", func(t *testing.T) {
		time.Sleep(10 * time.Millisecond)

		d := &memory.Decision{
			Topic:     "auth-strategy",
			Decision:  "Session cookies",
			Rationale: "Simpler for this use case",
			DecidedBy: "architect-2",
			CreatedAt: time.Now(),
		}

		err := store.SetDecision(d)
		if err != nil {
			t.Fatalf("expected append-only, got error: %v", err)
		}

		latest, err := store.GetDecision("auth-strategy")
		if err != nil {
			t.Fatalf("failed to get decision: %v", err)
		}
		if latest.Decision != "Session cookies" {
			t.Errorf("expected latest decision 'Session cookies', got %q", latest.Decision)
		}

		history, err := store.GetDecisionHistory("auth-strategy")
		if err != nil {
			t.Fatalf("failed to get decision history: %v", err)
		}
		if len(history) != 2 {
			t.Errorf("expected 2 decisions in history, got %d", len(history))
		}
	})

	t.Run("GetNonexistentDecision", func(t *testing.T) {
		_, err := store.GetDecision("nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ListDecisions", func(t *testing.T) {
		decisions, err := store.ListDecisions()
		if err != nil {
			t.Fatalf("failed to list decisions: %v", err)
		}
		if len(decisions) < 2 {
			t.Errorf("expected at least 2 decisions, got %d", len(decisions))
		}
	})
}

func TestDeleteDecision(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		d := &memory.Decision{
			Topic:     "test-topic",
			Decision:  fmt.Sprintf("Decision %d", i),
			CreatedAt: time.Now(),
		}
		time.Sleep(10 * time.Millisecond)
		if err := store.SetDecision(d); err != nil {
			t.Fatalf("failed to set decision: %v", err)
		}
	}

	d := &memory.Decision{
		Topic:     "other-topic",
		Decision:  "Other decision",
		CreatedAt: time.Now(),
	}
	if err := store.SetDecision(d); err != nil {
		t.Fatalf("failed to set decision: %v", err)
	}

	count, err := store.DeleteDecision("test-topic")
	if err != nil {
		t.Fatalf("failed to delete decisions: %v", err)
	}
	if count != 3 {
		t.Errorf("expected to delete 3 decisions, got %d", count)
	}

	_, err = store.GetDecision("test-topic")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for deleted topic, got %v", err)
	}

	_, err = store.GetDecision("other-topic")
	if err != nil {
		t.Errorf("expected other-topic to exist, got %v", err)
	}
}

func TestClearDecisions(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	topics := []string{"topic-a", "topic-b", "topic-c"}
	for _, topic := range topics {
		d := &memory.Decision{
			Topic:     topic,
			Decision:  "Decision for " + topic,
			CreatedAt: time.Now(),
		}
		if err := store.SetDecision(d); err != nil {
			t.Fatalf("failed to set decision: %v", err)
		}
	}

	count, err := store.ClearDecisions()
	if err != nil {
		t.Fatalf("failed to clear decisions: %v", err)
	}
	if count != 3 {
		t.Errorf("expected to clear 3 decisions, got %d", count)
	}

	decisions, _ := store.ListDecisions()
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions after clear, got %d", len(decisions))
	}
}

// =============================================================================
// Message Operations
// =============================================================================

func TestMessageOperations(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("SendAndReceiveMessages", func(t *testing.T) {
		broadcast := &memory.Message{
			From:      "executor-1",
			Content:   "User model is ready",
			CreatedAt: time.Now(),
		}
		if err := store.AddMessage(broadcast); err != nil {
			t.Fatalf("failed to add broadcast: %v", err)
		}
		if broadcast.ID == 0 {
			t.Error("expected auto-incremented ID")
		}

		direct := &memory.Message{
			From:      "executor-2",
			To:        "executor-1",
			Content:   "Can you review my changes?",
			CreatedAt: time.Now(),
		}
		if err := store.AddMessage(direct); err != nil {
			t.Fatalf("failed to add direct message: %v", err)
		}

		msgs1, err := store.GetMessages("executor-1")
		if err != nil {
			t.Fatalf("failed to get messages: %v", err)
		}
		if len(msgs1) != 2 {
			t.Errorf("executor-1 expected 2 messages, got %d", len(msgs1))
		}

		msgs2, err := store.GetMessages("executor-2")
		if err != nil {
			t.Fatalf("failed to get messages: %v", err)
		}
		if len(msgs2) != 1 {
			t.Errorf("executor-2 expected 1 message, got %d", len(msgs2))
		}
	})

	t.Run("AckMessage", func(t *testing.T) {
		msg := &memory.Message{
			From:      "sender",
			Content:   "Please ack me",
			CreatedAt: time.Now(),
		}
		if err := store.AddMessage(msg); err != nil {
			t.Fatalf("failed to add message: %v", err)
		}

		if err := store.AckMessage(msg.ID, "reader-1"); err != nil {
			t.Fatalf("failed to ack message: %v", err)
		}

		if err := store.AckMessage(msg.ID, "reader-1"); err != nil {
			t.Fatalf("duplicate ack should not fail: %v", err)
		}

		if err := store.AckMessage(99999, "reader-1"); err != ErrNotFound {
			t.Errorf("expected ErrNotFound for nonexistent message, got %v", err)
		}
	})

	t.Run("PruneExpiredMessages", func(t *testing.T) {
		expired := &memory.Message{
			From:      "sender",
			Content:   "I should be pruned",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		if err := store.AddMessage(expired); err != nil {
			t.Fatalf("failed to add expired message: %v", err)
		}

		fresh := &memory.Message{
			From:      "sender",
			Content:   "I should survive",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		if err := store.AddMessage(fresh); err != nil {
			t.Fatalf("failed to add fresh message: %v", err)
		}

		pruned, err := store.PruneMessages()
		if err != nil {
			t.Fatalf("failed to prune messages: %v", err)
		}
		if pruned < 1 {
			t.Errorf("expected at least 1 pruned message, got %d", pruned)
		}
	})

	t.Run("DefaultTTL", func(t *testing.T) {
		msg := &memory.Message{
			From:    "sender",
			Content: "Check my TTL",
		}
		if err := store.AddMessage(msg); err != nil {
			t.Fatalf("failed to add message: %v", err)
		}
		if msg.ExpiresAt.IsZero() {
			t.Error("expected ExpiresAt to be set with default TTL")
		}
		expected := time.Now().Add(DefaultMessageTTL)
		if msg.ExpiresAt.Before(expected.Add(-5*time.Second)) || msg.ExpiresAt.After(expected.Add(5*time.Second)) {
			t.Errorf("ExpiresAt %v not within expected range around %v", msg.ExpiresAt, expected)
		}
	})
}

// =============================================================================
// Meta Operations
// =============================================================================

func TestMetaSetAndGet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if err := store.SetMeta("test_key", "test_value"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	got, err := store.GetMeta("test_key")
	if err != nil {
		t.Fatalf("GetMeta failed: %v", err)
	}
	if got != "test_value" {
		t.Errorf("expected 'test_value', got %q", got)
	}
}

func TestMetaGetNonexistent(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := store.GetMeta("nonexistent_key")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMetaOverwrite(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.SetMeta("key", "value1")
	store.SetMeta("key", "value2")

	got, err := store.GetMeta("key")
	if err != nil {
		t.Fatalf("GetMeta failed: %v", err)
	}
	if got != "value2" {
		t.Errorf("expected 'value2', got %q", got)
	}
}

func TestNewBoltStoreRunsMigrations(t *testing.T) {
	// NewBoltStore should stamp the schema version on a fresh DB.
	store, cleanup := setupTestDB(t)
	defer cleanup()

	v, err := GetSchemaVersion(store.db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("expected schema version %d after NewBoltStore, got %d", SchemaVersion, v)
	}
}

// =============================================================================
// Concurrent Access
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		task := &memory.Task{
			ID:        fmt.Sprintf("concurrent-task-%d", i),
			Title:     fmt.Sprintf("Task %d", i),
			Status:    memory.TaskStatusPending,
			CreatedAt: time.Now(),
		}
		if err := store.AddTask(task); err != nil {
			t.Fatalf("failed to add task: %v", err)
		}
	}

	claimed := make(chan string, 100)
	errors := make(chan error, 100)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				taskID := fmt.Sprintf("concurrent-task-%d", j)
				task, err := store.ClaimTask(taskID, agentID)
				if err == nil {
					claimed <- fmt.Sprintf("%s claimed by %s", task.ID, agentID)
				} else if err != ErrAlreadyClaimed && err != ErrNotFound {
					errors <- err
				}
			}
		}(fmt.Sprintf("agent-%d", i))
	}

	wg.Wait()
	close(claimed)
	close(errors)

	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}

	claimCount := 0
	for range claimed {
		claimCount++
	}

	if claimCount != 10 {
		t.Errorf("expected 10 claims, got %d", claimCount)
	}
}

// =============================================================================
// Task Delete & Clear
// =============================================================================

func TestDeleteTask(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	task := &memory.Task{
		ID:        "task-del",
		Title:     "Delete me",
		Status:    memory.TaskStatusPending,
		CreatedAt: time.Now(),
	}
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := store.DeleteTask("task-del"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	_, err := store.GetTask("task-del")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestClearTasks(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		store.CreateTask(&memory.Task{
			ID:        fmt.Sprintf("task-c-%d", i),
			Title:     fmt.Sprintf("Task %d", i),
			Status:    memory.TaskStatusPending,
			CreatedAt: time.Now(),
		})
	}
	// Add one done task.
	store.CreateTask(&memory.Task{
		ID:        "task-done",
		Title:     "Done task",
		Status:    memory.TaskStatusDone,
		CreatedAt: time.Now(),
	})

	// Clear only pending.
	count, err := store.ClearTasks(memory.TaskStatusPending)
	if err != nil {
		t.Fatalf("ClearTasks(pending): %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 cleared, got %d", count)
	}

	// Done task should survive.
	got, err := store.GetTask("task-done")
	if err != nil {
		t.Fatalf("done task should survive: %v", err)
	}
	if got.Status != memory.TaskStatusDone {
		t.Errorf("expected done status, got %s", got.Status)
	}

	// Clear all.
	count, err = store.ClearTasks("")
	if err != nil {
		t.Fatalf("ClearTasks(''): %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cleared, got %d", count)
	}
}

// =============================================================================
// Message Clear
// =============================================================================

func TestClearMessages(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	store.AddMessage(&memory.Message{From: "a", To: "b", Content: "hello"})
	store.AddMessage(&memory.Message{From: "b", To: "a", Content: "hi"})
	store.AddMessage(&memory.Message{From: "c", Content: "broadcast"})

	// Clear by agent.
	count, err := store.ClearMessages("a")
	if err != nil {
		t.Fatalf("ClearMessages(a): %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 cleared for agent a (from+to), got %d", count)
	}

	// Broadcast from c should survive.
	msgs, _ := store.GetMessages("c")
	if len(msgs) != 1 {
		t.Errorf("expected 1 message for c, got %d", len(msgs))
	}

	// Clear all.
	count, err = store.ClearMessages("")
	if err != nil {
		t.Fatalf("ClearMessages(''): %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cleared, got %d", count)
	}
}

// =============================================================================
// TouchMemory (Access Tracking)
// =============================================================================

func TestTouchMemory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Seed two memories.
	m1 := &memory.Memory{
		ID:        "touch-1",
		Category:  memory.CategoryLearning,
		Content:   "First memory",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m2 := &memory.Memory{
		ID:        "touch-2",
		Category:  memory.CategoryLearning,
		Content:   "Second memory",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.AddMemory(m1); err != nil {
		t.Fatalf("AddMemory(m1): %v", err)
	}
	if err := store.AddMemory(m2); err != nil {
		t.Fatalf("AddMemory(m2): %v", err)
	}

	t.Run("IncrementFromZero", func(t *testing.T) {
		n, err := store.TouchMemory([]string{"touch-1"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 1 {
			t.Errorf("expected 1 touched, got %d", n)
		}

		got, err := store.GetMemory("touch-1")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if got.AccessCount != 1 {
			t.Errorf("expected AccessCount=1, got %d", got.AccessCount)
		}
		if got.LastAccessed.IsZero() {
			t.Error("expected LastAccessed to be set")
		}
	})

	t.Run("IncrementMultipleTimes", func(t *testing.T) {
		if _, err := store.TouchMemory([]string{"touch-1"}); err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		got, err := store.GetMemory("touch-1")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if got.AccessCount != 2 {
			t.Errorf("expected AccessCount=2, got %d", got.AccessCount)
		}
	})

	t.Run("TouchMultipleIDs", func(t *testing.T) {
		n, err := store.TouchMemory([]string{"touch-1", "touch-2"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 2 {
			t.Errorf("expected 2 touched, got %d", n)
		}

		got2, err := store.GetMemory("touch-2")
		if err != nil {
			t.Fatalf("GetMemory: %v", err)
		}
		if got2.AccessCount != 1 {
			t.Errorf("expected AccessCount=1 for touch-2, got %d", got2.AccessCount)
		}
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		n, err := store.TouchMemory([]string{})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 touched for empty list, got %d", n)
		}
	})

	t.Run("NonexistentIDs", func(t *testing.T) {
		n, err := store.TouchMemory([]string{"does-not-exist"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 touched for nonexistent ID, got %d", n)
		}
	})

	t.Run("MixedExistentAndNonexistent", func(t *testing.T) {
		before, _ := store.GetMemory("touch-1")
		prevCount := before.AccessCount

		n, err := store.TouchMemory([]string{"touch-1", "nonexistent-id"})
		if err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}
		if n != 1 {
			t.Errorf("expected 1 touched, got %d", n)
		}

		after, _ := store.GetMemory("touch-1")
		if after.AccessCount != prevCount+1 {
			t.Errorf("expected AccessCount=%d, got %d", prevCount+1, after.AccessCount)
		}
	})

	t.Run("DoesNotAlterOtherFields", func(t *testing.T) {
		before, _ := store.GetMemory("touch-2")

		if _, err := store.TouchMemory([]string{"touch-2"}); err != nil {
			t.Fatalf("TouchMemory: %v", err)
		}

		after, _ := store.GetMemory("touch-2")

		if after.Content != before.Content {
			t.Errorf("Content changed: %q -> %q", before.Content, after.Content)
		}
		if after.Category != before.Category {
			t.Errorf("Category changed: %q -> %q", before.Category, after.Category)
		}
		if after.Priority != before.Priority {
			t.Errorf("Priority changed: %v -> %v", before.Priority, after.Priority)
		}
	})
}
