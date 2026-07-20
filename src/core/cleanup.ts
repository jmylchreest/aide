/**
 * Cleanup logic — platform-agnostic.
 *
 * Extracted from src/hooks/agent-cleanup.ts and src/hooks/session-end.ts.
 * Handles agent state cleanup and session end operations.
 */

import { clearAgentState } from "./aide-client.js";

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

// Session teardown lives in Go as `aide session end` (single invocation:
// end message, agent state clear, session keys, metrics). All platforms —
// Claude Code SessionEnd hook, OpenCode session.deleted — spawn it rather
// than duplicating the sequence here.
