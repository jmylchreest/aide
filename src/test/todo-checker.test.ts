/**
 * Tests for todo-checker core logic
 *
 * Run with: npx vitest run src/test/todo-checker.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  checkTodos,
  parseTodosFromAide,
  TERMINAL_STATUSES,
  type TodoItem,
} from "../core/todo-checker.js";

describe("TERMINAL_STATUSES", () => {
  it("should include done, completed, and cancelled", () => {
    expect(TERMINAL_STATUSES.has("done")).toBe(true);
    expect(TERMINAL_STATUSES.has("completed")).toBe(true);
    expect(TERMINAL_STATUSES.has("cancelled")).toBe(true);
  });

  it("should not include active statuses", () => {
    expect(TERMINAL_STATUSES.has("pending")).toBe(false);
    expect(TERMINAL_STATUSES.has("claimed")).toBe(false);
    expect(TERMINAL_STATUSES.has("blocked")).toBe(false);
    expect(TERMINAL_STATUSES.has("in_progress")).toBe(false);
  });
});

describe("checkTodos", () => {
  it("should return no issues for empty todo list", () => {
    const result = checkTodos([]);
    expect(result.hasIncomplete).toBe(false);
    expect(result.incompleteCount).toBe(0);
    expect(result.message).toBe("");
  });

  it("should return no issues when all todos are done (aide status)", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Task 1", status: "done" },
      { id: "2", content: "Task 2", status: "done" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(false);
    expect(result.incompleteCount).toBe(0);
    expect(result.message).toBe("");
  });

  it("should treat completed and cancelled as terminal (legacy aliases)", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Task 1", status: "completed" },
      { id: "2", content: "Task 2", status: "cancelled" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(false);
    expect(result.incompleteCount).toBe(0);
    expect(result.message).toBe("");
  });

  it("should detect pending todos", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Task 1", status: "done" },
      { id: "2", content: "Task 2", status: "pending" },
      { id: "3", content: "Task 3", status: "pending" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(2);
    expect(result.totalCount).toBe(3);
    expect(result.incompleteItems).toHaveLength(2);
  });

  it("should detect claimed todos as incomplete", () => {
    const todos: TodoItem[] = [
      {
        id: "1",
        content: "Implementing feature X",
        status: "claimed",
        claimedBy: "agent-1",
      },
      { id: "2", content: "Task 2", status: "done" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(1);
    expect(result.incompleteItems[0].content).toBe("Implementing feature X");
  });

  it("should detect blocked todos as incomplete", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Blocked task", status: "blocked" },
      { id: "2", content: "Done task", status: "done" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(1);
    expect(result.incompleteItems[0].content).toBe("Blocked task");
  });

  it("should treat unknown statuses as incomplete (resilient default)", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Future status task", status: "waiting" },
      { id: "2", content: "Done task", status: "done" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(1);
    expect(result.incompleteItems[0].content).toBe("Future status task");
  });

  it("should format message with task details", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Write tests", status: "done" },
      {
        id: "2",
        content: "Fix bug in parser",
        status: "claimed",
        claimedBy: "agent-1",
      },
      { id: "3", content: "Update documentation", status: "pending" },
    ];
    const result = checkTodos(todos);
    expect(result.message).toContain("TODO CONTINUATION");
    expect(result.message).toContain("2 of 3 tasks incomplete");
    expect(result.message).toContain("1 done");
    expect(result.message).toContain("Fix bug in parser");
    expect(result.message).toContain("Update documentation");
    expect(result.message).toContain("Continue working");
  });

  it("should mark claimed items with > indicator", () => {
    const todos: TodoItem[] = [
      {
        id: "1",
        content: "Active task",
        status: "claimed",
        claimedBy: "agent-1",
      },
      { id: "2", content: "Pending task", status: "pending" },
    ];
    const result = checkTodos(todos);
    expect(result.message).toContain("[>] Active task");
    expect(result.message).toContain("[ ] Pending task");
  });

  it("should handle null/undefined input", () => {
    const result = checkTodos(null as unknown as TodoItem[]);
    expect(result.hasIncomplete).toBe(false);
    expect(result.message).toBe("");
  });

  // Agent-scoped filtering tests
  describe("agent scoping", () => {
    it("should show all tasks when no agentId provided", () => {
      const todos: TodoItem[] = [
        {
          id: "1",
          content: "Agent-1 task",
          status: "claimed",
          claimedBy: "agent-1",
        },
        {
          id: "2",
          content: "Agent-2 task",
          status: "claimed",
          claimedBy: "agent-2",
        },
        { id: "3", content: "Unclaimed task", status: "pending" },
      ];
      const result = checkTodos(todos);
      expect(result.hasIncomplete).toBe(true);
      expect(result.incompleteCount).toBe(3);
    });

    it("should only show agent's own tasks and unclaimed tasks", () => {
      const todos: TodoItem[] = [
        {
          id: "1",
          content: "My task",
          status: "claimed",
          claimedBy: "agent-1",
        },
        {
          id: "2",
          content: "Other agent task",
          status: "claimed",
          claimedBy: "agent-2",
        },
        { id: "3", content: "Unclaimed task", status: "pending" },
      ];
      const result = checkTodos(todos, "agent-1");
      expect(result.hasIncomplete).toBe(true);
      expect(result.incompleteCount).toBe(2);
      expect(result.incompleteItems.map((t) => t.content)).toEqual([
        "My task",
        "Unclaimed task",
      ]);
    });

    it("should allow stop when agent has no relevant tasks", () => {
      const todos: TodoItem[] = [
        {
          id: "1",
          content: "Other agent task",
          status: "claimed",
          claimedBy: "agent-2",
        },
      ];
      const result = checkTodos(todos, "agent-1");
      expect(result.hasIncomplete).toBe(false);
      expect(result.totalCount).toBe(0);
    });

    it("should report complete when agent's tasks are all done", () => {
      const todos: TodoItem[] = [
        {
          id: "1",
          content: "My done task",
          status: "done",
          claimedBy: "agent-1",
        },
        {
          id: "2",
          content: "Other agent task",
          status: "pending",
          claimedBy: "agent-2",
        },
      ];
      const result = checkTodos(todos, "agent-1");
      expect(result.hasIncomplete).toBe(false);
      expect(result.totalCount).toBe(1);
    });

    it("should include unclaimed incomplete tasks for any agent", () => {
      const todos: TodoItem[] = [
        {
          id: "1",
          content: "My done task",
          status: "done",
          claimedBy: "agent-1",
        },
        { id: "2", content: "Unclaimed pending", status: "pending" },
      ];
      const result = checkTodos(todos, "agent-1");
      expect(result.hasIncomplete).toBe(true);
      expect(result.incompleteCount).toBe(1);
      expect(result.incompleteItems[0].content).toBe("Unclaimed pending");
    });
  });
});

describe("parseTodosFromAide", () => {
  it("should parse aide task list output with real statuses", () => {
    const output = `[pending] abc12345: Implement feature X
[done] def45678: Write unit tests
[claimed] ghi78901: Deploy to staging
[blocked] jkl01234: Remove old code`;

    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(4);
    expect(todos[0]).toEqual({
      status: "pending",
      id: "abc12345",
      content: "Implement feature X",
      claimedBy: undefined,
    });
    expect(todos[1]).toEqual({
      status: "done",
      id: "def45678",
      content: "Write unit tests",
      claimedBy: undefined,
    });
    expect(todos[2]).toEqual({
      status: "claimed",
      id: "ghi78901",
      content: "Deploy to staging",
      claimedBy: undefined,
    });
    expect(todos[3]).toEqual({
      status: "blocked",
      id: "jkl01234",
      content: "Remove old code",
      claimedBy: undefined,
    });
  });

  it("should parse unknown/future statuses", () => {
    const output = `[waiting] abc12345: Future status task`;
    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(1);
    expect(todos[0].status).toBe("waiting");
  });

  it("should parse agent claim annotations", () => {
    const output = `[claimed] abc12345: Deploy service (agent:executor-1)`;
    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(1);
    expect(todos[0]).toEqual({
      status: "claimed",
      id: "abc12345",
      content: "Deploy service",
      claimedBy: "executor-1",
    });
  });

  it("should handle lines without agent annotation", () => {
    const output = `[pending] abc12345: Unclaimed task`;
    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(1);
    expect(todos[0].claimedBy).toBeUndefined();
  });

  it("should handle empty output", () => {
    expect(parseTodosFromAide("")).toEqual([]);
  });

  it("should skip malformed lines", () => {
    const output = `Some header text
[pending] abc12345: Real task
Not a todo item
[done] def45678: Another task`;

    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(2);
  });

  it("should handle content with colons", () => {
    const output = `[pending] abc12345: Fix bug: parser fails on colons`;
    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(1);
    expect(todos[0].content).toBe("Fix bug: parser fails on colons");
  });
});
