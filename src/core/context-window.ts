/** Context continuity is separate from provider prompt-cache lifetime. */
import { createHash, randomUUID } from "crypto";
import { getState, runAide, setState } from "./aide-client.js";

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

const validIdentityPart = (value: unknown): value is string =>
  typeof value === "string" &&
  value.trim() !== "" &&
  Array.from(value).every(
    (char) => char.charCodeAt(0) > 31 && char.charCodeAt(0) !== 127,
  );

function validContextWindow(value: unknown): value is ContextWindow {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const window = value as Record<string, unknown>;
  return (
    window.version === 1 &&
    validIdentityPart(window.id) &&
    (window.status === "active" || window.status === "pending") &&
    (window.continuity === "new" ||
      window.continuity === "reset" ||
      window.continuity === "unknown") &&
    typeof window.reason === "string"
  );
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
    return validContextWindow(window) ? window : null;
  } catch {
    return null;
  }
}

/** An explicit actor start may initialize missing state, but must not replace
 * an existing reset or pending window on replay. The backend owns the atomic
 * create-if-absent operation. Unsupported/failed initialization stays unknown. */
export function ensureContextWindow(
  binary: string,
  cwd: string,
  identity: ContextIdentity,
): ContextWindow | null {
  if (
    ![identity.host, identity.sessionId, identity.actorId].every(
      validIdentityPart,
    )
  )
    return null;
  const scope = contextScope(identity);
  if (!scope) return null;
  const key = `context-window:${scope}`;
  const initial: ContextWindow = {
    version: 1,
    id: randomUUID(),
    status: "active",
    continuity: "new",
    reason: "startup",
  };
  try {
    const result = runAide(
      binary,
      cwd,
      ["state", "init", key, JSON.stringify(initial), "--json"],
      { timeout: 5000 },
    );
    if (!result) return null;
    const state = JSON.parse(result);
    if (
      state?.key !== key ||
      typeof state.value !== "string" ||
      (state.agent !== undefined && state.agent !== "")
    )
      return null;
    const window = JSON.parse(state.value);
    return validContextWindow(window) ? window : null;
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
