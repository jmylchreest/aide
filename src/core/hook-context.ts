import {
  updateContextWindow,
  type ContextTransition,
} from "./context-window.js";

/** Shared lifecycle hooks must use the same actor scope as tool observations. */
export function updateHookContextWindow(
  binary: string,
  cwd: string,
  host: string,
  input: { session_id?: string; agent_id?: string },
  reason: ContextTransition,
) {
  return updateContextWindow(
    binary,
    cwd,
    {
      host,
      sessionId: input.session_id,
      actorId: input.agent_id || input.session_id,
    },
    reason,
  );
}
