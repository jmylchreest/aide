/**
 * Todo continuation checker — platform-agnostic.
 *
 * Reads the agent's todo list and checks for incomplete items.
 * Used to enhance persistence-logic.ts with precise todo-aware blocking:
 * instead of a generic "verify your work is complete", we list the
 * specific incomplete todos.
 *
 * For Claude Code: reads todos from the transcript (TodoWrite tool outputs)
 *   or from aide task list.
 * For OpenCode: reads todos via client.session.todo() API.
 *
 * This module provides the platform-agnostic core. Platform hooks
 * call it with however they obtained the todo list.
 */

import { runAide } from "./aide-client.js";
import { debug } from "../lib/logger.js";

const SOURCE = "todo-checker";

export interface TodoItem {
  id: string;
  content: string;
  status: "pending" | "in_progress" | "completed" | "cancelled";
  priority?: string;
}

export interface TodoCheckResult {
  /** Whether there are incomplete todos */
  hasIncomplete: boolean;
  /** Number of incomplete items */
  incompleteCount: number;
  /** Total items */
  totalCount: number;
  /** The incomplete items */
  incompleteItems: TodoItem[];
  /** Formatted message for injection */
  message: string;
}

/**
 * Check a list of todos for incomplete items and build a continuation message.
 */
export function checkTodos(todos: TodoItem[]): TodoCheckResult {
  if (!todos || todos.length === 0) {
    return {
      hasIncomplete: false,
      incompleteCount: 0,
      totalCount: 0,
      incompleteItems: [],
      message: "",
    };
  }

  const incomplete = todos.filter(
    (t) => t.status !== "completed" && t.status !== "cancelled",
  );

  if (incomplete.length === 0) {
    return {
      hasIncomplete: false,
      incompleteCount: 0,
      totalCount: todos.length,
      incompleteItems: [],
      message: "",
    };
  }

  const completedCount = todos.length - incomplete.length;
  const lines: string[] = [
    `**TODO CONTINUATION** — ${incomplete.length} of ${todos.length} tasks incomplete (${completedCount} done)`,
    "",
    "Remaining tasks:",
  ];

  for (const item of incomplete) {
    const statusIcon = item.status === "in_progress" ? ">" : " ";
    lines.push(`  [${statusIcon}] ${item.content}`);
  }

  lines.push("");
  lines.push(
    "You stopped but have unfinished tasks. Continue working on the next incomplete item.",
  );

  return {
    hasIncomplete: true,
    incompleteCount: incomplete.length,
    totalCount: todos.length,
    incompleteItems: incomplete,
    message: lines.join("\n"),
  };
}

/**
 * Parse todo items from aide task list output.
 *
 * Output format from `aide task list`:
 *   [status] id: content
 *   e.g.: [pending] abc123: Implement feature X
 */
export function parseTodosFromAide(output: string): TodoItem[] {
  const todos: TodoItem[] = [];
  const lines = output.split("\n");

  for (const line of lines) {
    const match = line.match(
      /\[(pending|in_progress|completed|cancelled)\]\s+(\S+):\s+(.+)/,
    );
    if (match) {
      todos.push({
        status: match[1] as TodoItem["status"],
        id: match[2],
        content: match[3].trim(),
      });
    }
  }

  return todos;
}

/**
 * Fetch todos from aide binary task list.
 * Returns empty array if binary unavailable or no tasks.
 */
export function fetchTodosFromAide(binary: string, cwd: string): TodoItem[] {
  try {
    const output = runAide(binary, cwd, ["task", "list"], { timeout: 5000 });
    if (!output) return [];
    return parseTodosFromAide(output);
  } catch (err) {
    debug(SOURCE, `Failed to fetch todos from aide: ${err}`);
    return [];
  }
}

debug(SOURCE, "Todo checker core loaded");
