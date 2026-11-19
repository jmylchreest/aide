import {
  BACKGROUND_HOOKS,
  BACKGROUND_HOOK_TIMEOUT_SECONDS,
} from "./hook-policy.js";

/** Host limits for frequently invoked hooks. Keep 500 ms for startup and output. */
const HOST_TIMEOUT_MS: Record<string, number> = {
  "observation-worker": 10_500,
  "skill-injector": 5000,
  "tool-tracker": 2000,
  "write-guard": 3000,
  "pre-tool-enforcer": 3000,
  "context-guard": 2000,
  "search-enrichment": 3000,
  "agent-signals": 2000,
  "tool-observe": 3000,
  "hud-updater": 3000,
  "comment-checker": 3000,
  "context-pruning": 3000,
  "usage-catchup": 2000,
};

let deadline: number | undefined;

/** Start once in the dispatcher, then carry the same deadline into its child. */
export function startHookBudget(source: string): number | undefined {
  const limit = BACKGROUND_HOOKS.has(source)
    ? BACKGROUND_HOOK_TIMEOUT_SECONDS * 1000
    : HOST_TIMEOUT_MS[source];
  if (limit === undefined) {
    deadline = undefined;
    return undefined;
  }
  const inherited = Number(process.env.AIDE_HOOK_DEADLINE_MS);
  const own = Date.now() + limit - 500;
  deadline =
    Number.isFinite(inherited) && inherited > 0
      ? Math.min(own, inherited)
      : own;
  return deadline;
}

/** All synchronous CLI calls in a hook share one budget, rather than each
 * consuming a timeout longer than the host allows for the entire hook.
 * OpenCode's in-process plugin and ordinary CLI operations retain their limits.
 */
export function hookCommandTimeout(requestedMs: number): number {
  if (deadline === undefined) return requestedMs;
  const remaining = deadline - Date.now();
  if (remaining <= 0) throw new Error("aide hook command budget exhausted");
  return Math.min(requestedMs, remaining);
}
