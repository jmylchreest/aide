/** Context continuity is separate from provider prompt-cache lifetime. */
import { createHash, randomUUID } from "crypto";
import { getState, setState } from "./aide-client.js";

export interface ContextIdentity {
  host?: string;
  sessionId?: string;
  actorId?: string;
}

export interface ContextWindow {
  version: 1;
  id: string;
  status: "active" | "pending";
  continuity: "new" | "reset" | "unknown";
  reason: string;
}

export function contextScope(identity?: ContextIdentity): string | null {
  if (
    !identity?.host ||
    !identity.sessionId ||
    identity.sessionId === "unknown" ||
    !identity.actorId
  )
    return null;
  return createHash("sha256")
    .update(
      JSON.stringify([identity.host, identity.sessionId, identity.actorId]),
    )
    .digest("hex");
}

export function contextWindow(
  binary: string,
  cwd: string,
  identity?: ContextIdentity,
): ContextWindow | null {
  const scope = contextScope(identity);
  if (!scope) return null;
  try {
    const window = JSON.parse(
      getState(binary, cwd, `context-window:${scope}`) ?? "null",
    );
    return window?.version === 1 &&
      typeof window.id === "string" &&
      window.id &&
      ["active", "pending"].includes(window.status)
      ? window
      : null;
  } catch {
    return null;
  }
}

export type ContextTransition =
  | "startup"
  | "compact_pending"
  | "compact_failed"
  | "compact"
  | "clear"
  | "resume"
  | "unknown"
  | "cache_expired";

/** Missing completion leaves a pending window unusable. A process restart
 * alone never calls this function; resume continuity is explicitly unknown. */
export function updateContextWindow(
  binary: string,
  cwd: string,
  identity: ContextIdentity,
  reason: ContextTransition,
): ContextWindow | null {
  const scope = contextScope(identity);
  if (!scope) return null;
  const previous = contextWindow(binary, cwd, identity);
  if (reason === "cache_expired") return previous;
  let next: ContextWindow;
  if (reason === "compact_pending" || reason === "compact_failed") {
    if (!previous) return null;
    next = {
      ...previous,
      status: reason === "compact_pending" ? "pending" : "active",
    };
  } else {
    next = {
      version: 1,
      id: randomUUID(),
      status: "active",
      reason,
      continuity:
        reason === "startup"
          ? "new"
          : reason === "compact" || reason === "clear"
            ? "reset"
            : "unknown",
    };
  }
  return setState(binary, cwd, `context-window:${scope}`, JSON.stringify(next))
    ? next
    : null;
}
