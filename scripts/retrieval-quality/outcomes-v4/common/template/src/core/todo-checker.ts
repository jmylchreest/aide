/**
 * Todo continuation checker — platform-agnostic.
 *
 * Reads aide tasks (`aide task list`) and checks for incomplete items.
 * Used to enhance persistence-logic.ts with precise todo-aware blocking:
 * instead of a generic "verify your work is complete", we list the
 * specific incomplete tasks.
 *
 * Only checks aide tasks (persistent, cross-session). Native todos
 * (Claude Code TodoWrite, OpenCode todowrite) are session-scoped
 * personal tracking and are intentionally not checked here — neither
 * platform exposes an API for reading them from hooks.
 *
 * This module provides the platform-agnostic core. Platform hooks
 * call it via persistence-logic.ts.
 */

import { runAide } from "./aide-client.js";
import { debug } from "../lib/logger.js";

const SOURCE = "todo-checker";

/**
 * Known terminal statuses — tasks in these states are considered "done".
 * Any status NOT in this set is treated as incomplete (including unknown
 * statuses from future aide versions), which is the safe default for
 * persistence enforcement.
 *
 * Covers both aide backend statuses (done) and any legacy/alias statuses
 * (completed, cancelled) for forward/backward compatibility.
 */
export const TERMINAL_STATUSES = new Set(["done", "completed", "cancelled"]);

export interface TodoItem {
  id: string;
  content: string;
  /** Raw status string from aide — may be any value, not just known ones. */
  status: string;
  /** Agent that claimed this task, if any. */
  claimedBy?: string;
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
 *
 * When agentId is provided, only tasks relevant to this agent are considered:
 * - Unclaimed tasks (pending, blocked) — everyone's responsibility
 * - Tasks claimed by this agent — this agent's responsibility
 * Tasks claimed by other agents are filtered out.
 */
export function checkTodos(
  todos: TodoItem[],
  agentId?: string,
): TodoCheckResult {
  if (!todos || todos.length === 0) {
    return {
      hasIncomplete: false,
      incompleteCount: 0,
      totalCount: 0,
      incompleteItems: [],
      message: "",
    };
  }

  // When scoped to a specific agent, filter out tasks claimed by other agents.
  // Unclaimed tasks (no claimedBy) are considered everyone's responsibility.
  const relevant = agentId
    ? todos.filter((t) => !t.claimedBy || t.claimedBy === agentId)
    : todos;

  const incomplete = relevant.filter((t) => !TERMINAL_STATUSES.has(t.status));

  if (incomplete.length === 0) {
    return {
      hasIncomplete: false,
      incompleteCount: 0,
      totalCount: relevant.length,
      incompleteItems: [],
      message: "",
    };
  }

  const completedCount = relevant.length - incomplete.length;
  const lines: string[] = [
    `**TODO CONTINUATION** — ${incomplete.length} of ${relevant.length} tasks incomplete (${completedCount} done)`,
    "",
    "Remaining tasks:",
  ];

  for (const item of incomplete) {
    const statusIcon =
      item.status === "in_progress" || item.status === "claimed" ? ">" : " ";
    lines.push(`  [${statusIcon}] ${item.content}`);
  }

  lines.push("");
  lines.push(
    "You stopped but have unfinished tasks. Continue working on the next incomplete item.",
  );

  return {
    hasIncomplete: true,
    incompleteCount: incomplete.length,
    totalCount: relevant.length,
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
 *         [claimed] task-def: Deploy service
 *
 * The regex accepts any alphanumeric/underscore status to handle
 * current statuses (pending, claimed, done, blocked) and any future
 * additions without code changes. Unknown statuses are treated as
 * incomplete by checkTodos().
 */
export function parseTodosFromAide(output: string): TodoItem[] {
  const todos: TodoItem[] = [];
  const lines = output.split("\n");

  for (const line of lines) {
    const match = line.match(
      /\[(\w+)\]\s+(\S+):\s+(.+?)(?:\s+\(agent:(\S+)\))?$/,
    );
    if (match) {
      todos.push({
        status: match[1],
        id: match[2],
        content: match[3].trim(),
        claimedBy: match[4] || undefined,
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
