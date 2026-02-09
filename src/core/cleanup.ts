/**
 * Cleanup logic — platform-agnostic.
 *
 * Extracted from src/hooks/agent-cleanup.ts and src/hooks/session-end.ts.
 * Handles agent state cleanup and session end operations.
 */

import { clearAgentState, deleteState, setState, runAide } from "./aide-client.js";

/**
 * Clean up agent-specific state when an agent stops
 */
export function cleanupAgent(
  binary: string,
  cwd: string,
  agentId: string,
): boolean {
  return clearAgentState(binary, cwd, agentId);
}

/**
 * Clean up session state and record session end
 */
export function cleanupSession(
  binary: string,
  cwd: string,
  sessionId: string,
  duration?: number,
): void {
  // Record session end
  const durationStr = duration ? ` (${Math.round(duration / 1000)}s)` : "";
  runAide(binary, cwd, [
    "message",
    "send",
    `Session ${sessionId} ended${durationStr}`,
    "--from=system",
    "--type=system",
  ]);

  // Clear transient state for this session/agent
  clearAgentState(binary, cwd, sessionId);

  // Clear global session state
  const globalSessionKeys = [
    "mode",
    "startedAt",
    "modelTier",
    "agentCount",
    "toolCalls",
    "lastToolUse",
    "lastTool",
  ];
  for (const key of globalSessionKeys) {
    deleteState(binary, cwd, key);
  }

  // Record session metrics
  if (duration) {
    setState(binary, cwd, "last_session_duration", String(duration));
  }
  setState(binary, cwd, "last_session_end", new Date().toISOString());
}
