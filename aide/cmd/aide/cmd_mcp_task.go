package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// Input types for task tools
// ============================================================================

type TaskCreateInput struct {
	Title       string `json:"title" jsonschema:"Short title for the task (required)"`
	Description string `json:"description,omitempty" jsonschema:"Detailed description of what the task involves"`
}

type TaskGetInput struct {
	ID string `json:"id" jsonschema:"The task ID to retrieve"`
}

type TaskListInput struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by status: pending, claimed, done, blocked. Omit for all tasks."`
}

type TaskClaimInput struct {
	TaskID  string `json:"task_id" jsonschema:"The task ID to claim"`
	AgentID string `json:"agent_id" jsonschema:"Your agent ID (required)"`
}

type TaskCompleteInput struct {
	TaskID string `json:"task_id" jsonschema:"The task ID to mark as complete"`
	Result string `json:"result,omitempty" jsonschema:"Completion result or summary"`
}

type TaskDeleteInput struct {
	ID string `json:"id" jsonschema:"The task ID to delete"`
}

// ============================================================================
// Task Tools
// ============================================================================

func (s *MCPServer) registerTaskTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_create",
		Description: `Create a new swarm task.

Tasks are units of work that can be claimed by agents in swarm mode.
New tasks start with status "pending".

Returns the created task with its generated ID.`,
	}, s.handleTaskCreate)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_get",
		Description: `Get a task by its ID.

Returns full task details including status, claimed_by, result, and timestamps.`,
	}, s.handleTaskGet)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_list",
		Description: `List tasks, optionally filtered by status.

**Task statuses:**
- "pending" - Available to be claimed
- "claimed" - Being worked on by an agent
- "done" - Completed with result
- "blocked" - Waiting on something

Omit status to see all tasks.`,
	}, s.handleTaskList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_claim",
		Description: `Claim a pending task for your agent.

Atomically transitions a task from "pending" to "claimed" and assigns it
to the specified agent. Fails if the task is already claimed or not pending.`,
	}, s.handleTaskClaim)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_complete",
		Description: `Mark a task as done with an optional result summary.

Transitions the task to "done" status and records the completion result.`,
	}, s.handleTaskComplete)

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "task_delete",
		Description: `Delete a task by its ID.

Permanently removes the task. Use this for cleanup of obsolete or
mistakenly created tasks.`,
	}, s.handleTaskDelete)
}

// ============================================================================
// Task Handlers
// ============================================================================

func (s *MCPServer) handleTaskCreate(_ context.Context, _ *mcp.CallToolRequest, input TaskCreateInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_create title=%q", input.Title)

	if input.Title == "" {
		return errorResult("'title' is required"), nil, nil
	}

	task := &memory.Task{
		Title:       input.Title,
		Description: input.Description,
		Status:      memory.TaskStatusPending,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateTask(task); err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("create task failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  created: id=%s", task.ID)
	return textResult(formatTaskJSON(task)), nil, nil
}

func (s *MCPServer) handleTaskGet(_ context.Context, _ *mcp.CallToolRequest, input TaskGetInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_get id=%s", input.ID)

	if input.ID == "" {
		return errorResult("'id' is required"), nil, nil
	}

	task, err := s.store.GetTask(input.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return textResult(fmt.Sprintf("Task not found: %s", input.ID)), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("get task failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %s status=%s", task.Title, task.Status)
	return textResult(formatTaskJSON(task)), nil, nil
}

func (s *MCPServer) handleTaskList(_ context.Context, _ *mcp.CallToolRequest, input TaskListInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_list status=%s", input.Status)

	tasks, err := s.store.ListTasks(memory.TaskStatus(input.Status))
	if err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("list tasks failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  found: %d tasks", len(tasks))

	if len(tasks) == 0 {
		if input.Status != "" {
			return textResult(fmt.Sprintf("No tasks with status: %s", input.Status)), nil, nil
		}
		return textResult("No tasks found."), nil, nil
	}

	return textResult(formatTasksMarkdown(tasks)), nil, nil
}

func (s *MCPServer) handleTaskClaim(_ context.Context, _ *mcp.CallToolRequest, input TaskClaimInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_claim task=%s agent=%s", input.TaskID, input.AgentID)

	if input.TaskID == "" {
		return errorResult("'task_id' is required"), nil, nil
	}
	if input.AgentID == "" {
		return errorResult("'agent_id' is required"), nil, nil
	}

	task, err := s.store.ClaimTask(input.TaskID, input.AgentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errorResult(fmt.Sprintf("task not found: %s", input.TaskID)), nil, nil
		}
		if errors.Is(err, store.ErrAlreadyClaimed) {
			return errorResult(fmt.Sprintf("task already claimed: %s", input.TaskID)), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("claim task failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  claimed: %s by %s", task.ID, input.AgentID)
	return textResult(formatTaskJSON(task)), nil, nil
}

func (s *MCPServer) handleTaskComplete(_ context.Context, _ *mcp.CallToolRequest, input TaskCompleteInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_complete task=%s", input.TaskID)

	if input.TaskID == "" {
		return errorResult("'task_id' is required"), nil, nil
	}

	if err := s.store.CompleteTask(input.TaskID, input.Result); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errorResult(fmt.Sprintf("task not found: %s", input.TaskID)), nil, nil
		}
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("complete task failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  completed")
	return textResult(fmt.Sprintf("Task %s marked as done.", input.TaskID)), nil, nil
}

func (s *MCPServer) handleTaskDelete(_ context.Context, _ *mcp.CallToolRequest, input TaskDeleteInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: task_delete id=%s", input.ID)

	if input.ID == "" {
		return errorResult("'id' is required"), nil, nil
	}

	if err := s.store.DeleteTask(input.ID); err != nil {
		mcpLog.Printf("  error: %v", err)
		return errorResult(fmt.Sprintf("delete task failed: %v", err)), nil, nil
	}

	mcpLog.Printf("  deleted")
	return textResult(fmt.Sprintf("Task %s deleted.", input.ID)), nil, nil
}

// ============================================================================
// Task Formatting
// ============================================================================

func formatTaskJSON(task *memory.Task) string {
	data, _ := json.MarshalIndent(task, "", "  ")
	return string(data)
}

func formatTasksMarkdown(tasks []*memory.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Tasks (%d)\n\n", len(tasks))

	for _, t := range tasks {
		fmt.Fprintf(&sb, "- **[%s]** `%s`: %s", t.Status, truncateID(t.ID), t.Title)
		if t.ClaimedBy != "" {
			fmt.Fprintf(&sb, " (claimed by %s)", t.ClaimedBy)
		}
		if t.Result != "" {
			fmt.Fprintf(&sb, " — %s", t.Result)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncateID shortens a ULID-style ID for display (first 8 chars).
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
