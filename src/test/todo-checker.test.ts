/**
 * Tests for todo-checker core logic
 *
 * Run with: npx vitest run src/test/todo-checker.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  checkTodos,
  parseTodosFromAide,
  type TodoItem,
} from "../core/todo-checker.js";

describe("checkTodos", () => {
  it("should return no issues for empty todo list", () => {
    const result = checkTodos([]);
    expect(result.hasIncomplete).toBe(false);
    expect(result.incompleteCount).toBe(0);
    expect(result.message).toBe("");
  });

  it("should return no issues when all todos are completed", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Task 1", status: "completed" },
      { id: "2", content: "Task 2", status: "completed" },
      { id: "3", content: "Task 3", status: "cancelled" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(false);
    expect(result.incompleteCount).toBe(0);
    expect(result.message).toBe("");
  });

  it("should detect pending todos", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Task 1", status: "completed" },
      { id: "2", content: "Task 2", status: "pending" },
      { id: "3", content: "Task 3", status: "pending" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(2);
    expect(result.totalCount).toBe(3);
    expect(result.incompleteItems).toHaveLength(2);
  });

  it("should detect in_progress todos", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Implementing feature X", status: "in_progress" },
      { id: "2", content: "Task 2", status: "completed" },
    ];
    const result = checkTodos(todos);
    expect(result.hasIncomplete).toBe(true);
    expect(result.incompleteCount).toBe(1);
    expect(result.incompleteItems[0].content).toBe("Implementing feature X");
  });

  it("should format message with task details", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Write tests", status: "completed" },
      { id: "2", content: "Fix bug in parser", status: "in_progress" },
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

  it("should mark in_progress items with > indicator", () => {
    const todos: TodoItem[] = [
      { id: "1", content: "Active task", status: "in_progress" },
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
});

describe("parseTodosFromAide", () => {
  it("should parse aide task list output", () => {
    const output = `[pending] abc123: Implement feature X
[completed] def456: Write unit tests
[in_progress] ghi789: Deploy to staging
[cancelled] jkl012: Remove old code`;

    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(4);
    expect(todos[0]).toEqual({
      status: "pending",
      id: "abc123",
      content: "Implement feature X",
    });
    expect(todos[1]).toEqual({
      status: "completed",
      id: "def456",
      content: "Write unit tests",
    });
    expect(todos[2]).toEqual({
      status: "in_progress",
      id: "ghi789",
      content: "Deploy to staging",
    });
    expect(todos[3]).toEqual({
      status: "cancelled",
      id: "jkl012",
      content: "Remove old code",
    });
  });

  it("should handle empty output", () => {
    expect(parseTodosFromAide("")).toEqual([]);
  });

  it("should skip malformed lines", () => {
    const output = `Some header text
[pending] abc123: Real task
Not a todo item
[completed] def456: Another task`;

    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(2);
  });

  it("should handle content with colons", () => {
    const output = `[pending] abc123: Fix bug: parser fails on colons`;
    const todos = parseTodosFromAide(output);
    expect(todos).toHaveLength(1);
    expect(todos[0].content).toBe("Fix bug: parser fails on colons");
  });
});
