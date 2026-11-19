/**
 * Persistence logic — platform-agnostic.
 *
 * Extracted from src/hooks/persistence.ts.
 * Determines whether stop should be blocked based on active persistence mode.
 */

import { getState, setState, incrementState } from "./aide-client.js";
import {
  PERSISTENCE_MODES,
  MAX_PERSISTENCE_ITERATIONS,
  type PersistenceMode,
} from "./types.js";
import { fetchTodosFromAide, checkTodos } from "./todo-checker.js";
import { debug } from "../lib/logger.js";

const SOURCE = "persistence-logic";

/**
 * Check if a persistence mode is active.
 *
 * Mode is global by design (see the note in aide-client.ts): the autopilot
 * skill and its documented off-switch (`aide state set mode ""`) both write
 * the global key. Iteration counters remain session-scoped.
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
  sessionId?: string,
): { reason: string } | null {
  const mode = getActiveMode(binary, cwd);
  if (!mode) return null;

  const iteration = incrementState(
    binary,
    cwd,
    `${mode}_iterations`,
    sessionId,
  );
  if (iteration === null) {
    // An older daemon or failed counter must not trap the user in a stop loop.
    debug(SOURCE, "Atomic persistence counter unavailable; allowing stop");
    return null;
  }

  if (iteration > MAX_PERSISTENCE_ITERATIONS) {
    // Clear the (global) mode after max iterations, allow stop
    setState(binary, cwd, "mode", "");
    setState(binary, cwd, `${mode}_iterations`, "0", sessionId);
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
    setState(binary, cwd, `${mode}_iterations`, "0", sessionId);
    return null;
  }

  return { reason: buildReinforcement(mode, iteration, todoSummary) };
}
