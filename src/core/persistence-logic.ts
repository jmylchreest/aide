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
 * Build a reinforcement message for the persistence mode
 */
export function buildReinforcement(mode: string, iteration: number): string {
  if (iteration > MAX_PERSISTENCE_ITERATIONS) {
    return `Maximum reinforcements (${MAX_PERSISTENCE_ITERATIONS}) reached. Releasing ${mode} mode.`;
  }

  return `**${mode.toUpperCase()} MODE ACTIVE** (iteration ${iteration}/${MAX_PERSISTENCE_ITERATIONS})

You attempted to stop but work may be incomplete.

Before stopping, verify:
- All tasks in your todo list are marked complete
- All requested functionality is implemented
- Tests pass (if applicable)
- No errors remain unaddressed

If ANY item is incomplete, CONTINUE WORKING.

Use TaskList to check your progress.`;
}

/**
 * Check persistence and return block decision.
 *
 * Returns null if stop is allowed, or { reason } if stop should be blocked.
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

  return { reason: buildReinforcement(mode, iteration) };
}
