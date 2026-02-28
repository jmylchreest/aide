/**
 * Persistence logic — platform-agnostic.
 *
 * Extracted from src/hooks/persistence.ts.
 * Determines whether stop should be blocked based on active persistence mode.
 */

import { getState, setState } from "./aide-client.js";
import {
  PERSISTENCE_MODES,
  MAX_PERSISTENCE_ITERATIONS,
  type PersistenceMode,
} from "./types.js";
import { fetchTodosFromAide, checkTodos } from "./todo-checker.js";
import { debug } from "../lib/logger.js";

const SOURCE = "persistence-logic";

/**
 * Check if a persistence mode is active
 */
export function getActiveMode(
  binary: string,
  cwd: string,
): PersistenceMode | null {
  const mode = getState(binary, cwd, "mode");
  if (mode && (PERSISTENCE_MODES as readonly string[]).includes(mode)) {
    return mode as PersistenceMode;
  }
  return null;
}

/**
 * Build a reinforcement message for the persistence mode.
 * If a todo summary is provided, it's appended to give the agent
 * precise visibility into which tasks are still incomplete.
 */
export function buildReinforcement(
  mode: string,
  iteration: number,
  todoSummary?: string,
): string {
  if (iteration > MAX_PERSISTENCE_ITERATIONS) {
    return `Maximum reinforcements (${MAX_PERSISTENCE_ITERATIONS}) reached. Releasing ${mode} mode.`;
  }

  const parts: string[] = [
    `**${mode.toUpperCase()} MODE ACTIVE** (iteration ${iteration}/${MAX_PERSISTENCE_ITERATIONS})`,
    "",
    "You attempted to stop but work may be incomplete.",
  ];

  if (todoSummary) {
    parts.push("");
    parts.push(todoSummary);
  } else {
    parts.push("");
    parts.push("Before stopping, verify:");
    parts.push("- All tasks in your todo list are marked complete");
    parts.push("- All requested functionality is implemented");
    parts.push("- Tests pass (if applicable)");
    parts.push("- No errors remain unaddressed");
  }

  parts.push("");
  parts.push("If ANY item is incomplete, CONTINUE WORKING.");

  return parts.join("\n");
}

/**
 * Check persistence and return block decision.
 *
 * Returns null if stop is allowed, or { reason } if stop should be blocked.
 * When a persistence mode is active and todos exist, the reinforcement
 * message includes the specific incomplete tasks.
 *
 * When agentId is provided, only tasks claimed by that agent are considered
 * for blocking. This prevents subagents from being blocked by tasks that
 * belong to other agents. Global (unclaimed) tasks still count for all agents.
 */
export function checkPersistence(
  binary: string,
  cwd: string,
  agentId?: string,
): { reason: string } | null {
  const mode = getActiveMode(binary, cwd);
  if (!mode) return null;

  // Get and increment iteration counter (guard against NaN from corrupted state)
  const iterStr = getState(binary, cwd, `${mode}_iterations`) || "0";
  const parsed = parseInt(iterStr, 10);
  const iteration = (Number.isNaN(parsed) ? 0 : parsed) + 1;
  setState(binary, cwd, `${mode}_iterations`, String(iteration));

  if (iteration > MAX_PERSISTENCE_ITERATIONS) {
    // Clear the mode after max iterations, allow stop
    setState(binary, cwd, "mode", "");
    setState(binary, cwd, `${mode}_iterations`, "0");
    return null;
  }

  // Fetch todos and build a specific continuation message if incomplete tasks exist.
  // If all tasks are complete (or no tasks exist), auto-release: allow stop.
  let todoSummary: string | undefined;
  let allTasksComplete = false;
  try {
    const todos = fetchTodosFromAide(binary, cwd);
    const todoResult = checkTodos(todos, agentId);
    if (todoResult.hasIncomplete) {
      todoSummary = todoResult.message;
      debug(
        SOURCE,
        `Found ${todoResult.incompleteCount} incomplete todos for persistence reinforcement`,
      );
    } else if (todoResult.totalCount > 0) {
      // All tasks exist and are in terminal states — work is done
      allTasksComplete = true;
      debug(
        SOURCE,
        `All ${todoResult.totalCount} tasks complete — auto-releasing ${mode} mode`,
      );
    }
  } catch (err) {
    debug(SOURCE, `Failed to fetch todos for persistence (non-fatal): ${err}`);
  }

  // Auto-release: if tasks exist and all are complete, allow stop
  if (allTasksComplete) {
    setState(binary, cwd, "mode", "");
    setState(binary, cwd, `${mode}_iterations`, "0");
    return null;
  }

  return { reason: buildReinforcement(mode, iteration, todoSummary) };
}
