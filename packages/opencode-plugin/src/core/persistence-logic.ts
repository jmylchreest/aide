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
 */
export function checkPersistence(
  binary: string,
  cwd: string,
): { reason: string } | null {
  const mode = getActiveMode(binary, cwd);
  if (!mode) return null;

  // Get and increment iteration counter
  const iterStr = getState(binary, cwd, `${mode}_iterations`) || "0";
  const iteration = parseInt(iterStr, 10) + 1;
  setState(binary, cwd, `${mode}_iterations`, String(iteration));

  if (iteration > MAX_PERSISTENCE_ITERATIONS) {
    // Clear the mode after max iterations, allow stop
    setState(binary, cwd, "mode", "");
    setState(binary, cwd, `${mode}_iterations`, "0");
    return null;
  }

  // Fetch todos and build a specific continuation message if incomplete tasks exist
  let todoSummary: string | undefined;
  try {
    const todos = fetchTodosFromAide(binary, cwd);
    const todoResult = checkTodos(todos);
    if (todoResult.hasIncomplete) {
      todoSummary = todoResult.message;
      debug(
        SOURCE,
        `Found ${todoResult.incompleteCount} incomplete todos for persistence reinforcement`,
      );
    }
  } catch (err) {
    debug(SOURCE, `Failed to fetch todos for persistence (non-fatal): ${err}`);
  }

  return { reason: buildReinforcement(mode, iteration, todoSummary) };
}
