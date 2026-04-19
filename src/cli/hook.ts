/**
 * Hook dispatcher for Codex CLI.
 *
 * Codex hooks.json calls `aide-plugin hook <name>` which dispatches to
 * the appropriate hook script in src/hooks/. Input is normalized from
 * stdin and passed through to the hook script.
 *
 * This avoids duplicating hook scripts — the same scripts work for both
 * Claude Code (via plugin.json) and Codex CLI (via hooks.json).
 */

import { execFileSync } from "child_process";
import { existsSync } from "fs";
import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";
import { readStdin, normalizeHookInput } from "../lib/hook-utils.js";

/** Maps hook CLI names to their script files in src/hooks/. */
const HOOK_MAP: Record<string, string> = {
  "session-start": "session-start.ts",
  "skill-injector": "skill-injector.ts",
  "tool-tracker": "tool-tracker.ts",
  "write-guard": "write-guard.ts",
  "pre-tool-enforcer": "pre-tool-enforcer.ts",
  "context-guard": "context-guard.ts",
  "search-enrichment": "search-enrichment.ts",
  "hud-updater": "hud-updater.ts",
  "comment-checker": "comment-checker.ts",
  "context-pruning": "context-pruning.ts",
  "persistence": "persistence.ts",
  "session-summary": "session-summary.ts",
  "agent-cleanup": "agent-cleanup.ts",
  "session-end": "session-end.ts",
};

export function listHooks(): string[] {
  return Object.keys(HOOK_MAP);
}

/**
 * Dispatch a hook by name.
 *
 * Reads stdin, normalizes field names for cross-platform compatibility,
 * then spawns the hook script with the normalized input.
 */
export async function dispatchHook(hookName: string): Promise<void> {
  const scriptFile = HOOK_MAP[hookName];
  if (!scriptFile) {
    console.error(
      `Unknown hook: ${hookName}\nAvailable hooks: ${Object.keys(HOOK_MAP).join(", ")}`,
    );
    process.exit(1);
  }

  const thisDir = dirname(fileURLToPath(import.meta.url));
  const pluginRoot = resolve(thisDir, "..", "..");
  const scriptPath = join(pluginRoot, "src", "hooks", scriptFile);

  if (!existsSync(scriptPath)) {
    console.error(`Hook script not found: ${scriptPath}`);
    process.exit(1);
  }

  const env = {
    ...process.env,
    AIDE_PLUGIN_ROOT: pluginRoot,
    AIDE_PLATFORM: "codex",
  };

  const rawInput = await readStdin();
  const normalizedInput = normalizeHookInput(rawInput);

  try {
    execFileSync(process.execPath, [scriptPath], {
      input: normalizedInput,
      stdio: ["pipe", "inherit", "inherit"],
      env,
      timeout: 120_000,
    });
  } catch (err: unknown) {
    if (err && typeof err === "object" && "status" in err) {
      process.exit((err as { status: number }).status ?? 1);
    }
    throw err;
  }
}
