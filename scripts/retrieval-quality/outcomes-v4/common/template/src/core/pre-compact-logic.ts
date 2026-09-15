/**
 * Pre-compact logic — platform-agnostic.
 *
 * Extracted from src/hooks/pre-compact.ts.
 * Saves state snapshot before context compaction.
 */

import { runAide, setState } from "./aide-client.js";

/**
 * Save current state snapshot before compaction
 */
export function saveStateSnapshot(
  binary: string,
  cwd: string,
  sessionId: string,
): void {
  // Record compaction event
  runAide(binary, cwd, [
    "message",
    "send",
    `Context compaction initiated for session ${sessionId}`,
    "--from=system",
    "--type=system",
  ]);

  // Get current state and preserve it
  const stateOutput = runAide(binary, cwd, ["state", "list"]);
  if (stateOutput?.trim()) {
    const safeState = stateOutput.replace(/\n/g, " ").slice(0, 1000);
    setState(binary, cwd, "precompact_snapshot", safeState);
  }

  // Record the compaction timestamp
  setState(binary, cwd, "last_compaction", new Date().toISOString());
}
