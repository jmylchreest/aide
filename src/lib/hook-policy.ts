/** These handlers report observations and never decide host behavior. */
export const BACKGROUND_HOOKS = new Set([
  "tool-tracker",
  "tool-observe",
  "hud-updater",
  "usage-catchup",
  "session-summary",
  "reflect",
]);

export const BACKGROUND_HOOK_TIMEOUT_SECONDS = 15;
