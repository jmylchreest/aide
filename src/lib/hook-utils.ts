/**
 * Shared utilities for Claude Code and Codex CLI hooks.
 *
 * readStdin() is the only unique implementation here. All other functions
 * are convenience wrappers around src/core/aide-client.ts that resolve the
 * binary from AIDE_PLUGIN_ROOT / CLAUDE_PLUGIN_ROOT automatically.
 */

import {
  findAideBinary as clientFindBinary,
  runAide as clientRunAide,
  setState,
  getState,
  deleteState,
  clearAgentState as clientClearAgentState,
  sanitizeForLog,
  shellEscape,
} from "../core/aide-client.js";

export { sanitizeForLog, shellEscape };

/** Maximum stdin payload size: 50 MiB. Prevents unbounded memory allocation. */
const MAX_STDIN_BYTES = 50 * 1024 * 1024;

/**
 * Read JSON input from stdin (used by all hooks).
 * Rejects payloads exceeding MAX_STDIN_BYTES.
 */
export async function readStdin(): Promise<string> {
  const chunks: Buffer[] = [];
  let totalBytes = 0;
  for await (const chunk of process.stdin) {
    totalBytes += chunk.length;
    if (totalBytes > MAX_STDIN_BYTES) {
      throw new Error(
        `stdin payload exceeds ${MAX_STDIN_BYTES} bytes, rejecting`,
      );
    }
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf-8");
}

/**
 * Normalize hook input JSON from different platforms (Claude Code, Codex CLI).
 *
 * Both platforms use command-type hooks with JSON stdin, but field names may
 * differ between versions. This function maps known alternative names to the
 * canonical snake_case format used by aide hook scripts.
 *
 * Returns the normalized JSON string (or the original if no changes needed).
 */
export function normalizeHookInput(raw: string): string {
  try {
    const data = JSON.parse(raw) as Record<string, unknown>;

    // Map known alternative field names → canonical snake_case
    const aliases: Record<string, string> = {
      hookEventName: "hook_event_name",
      sessionId: "session_id",
      toolName: "tool_name",
      agentId: "agent_id",
      agentName: "agent_name",
      toolInput: "tool_input",
      permissionMode: "permission_mode",
    };

    let changed = false;
    for (const [alt, canonical] of Object.entries(aliases)) {
      if (alt in data && !(canonical in data)) {
        data[canonical] = data[alt];
        delete data[alt];
        changed = true;
      }
    }

    return changed ? JSON.stringify(data) : raw;
  } catch {
    return raw;
  }
}

/**
 * Detect which AI assistant harness is running these hooks.
 *
 * - Codex CLI: hook dispatcher sets AIDE_PLATFORM=codex
 * - Claude Code: sets CLAUDE_PLUGIN_ROOT
 * - OpenCode uses a separate code path (src/opencode/hooks.ts), so hooks
 *   in src/hooks/ are only invoked by Claude Code or Codex.
 */
export function detectPlatform(): "claude-code" | "codex" {
  if (process.env.AIDE_PLATFORM === "codex") return "codex";
  return "claude-code";
}

import { existsSync, readFileSync } from "fs";
import { join } from "path";
import { findProjectRoot } from "./project-root.js";

const TRUTHY = new Set(["1", "true", "on", "yes"]);
const FALSY = new Set(["0", "false", "off", "no"]);

/**
 * isTruthy reports whether an env-var value should be treated as "on".
 * Accepts the same set as the Go config helper: 1/true/on/yes
 * (case-insensitive, whitespace-trimmed). Use for opt-in flags.
 */
export function isTruthy(v: string | undefined): boolean {
  if (!v) return false;
  return TRUTHY.has(v.trim().toLowerCase());
}

/**
 * isFalsy reports whether an env-var value was explicitly set to disable.
 * Accepts 0/false/off/no. Unset/empty/unknown values return false so the
 * caller's default-on behaviour wins. Use for opt-out flags.
 */
export function isFalsy(v: string | undefined): boolean {
  if (!v) return false;
  return FALSY.has(v.trim().toLowerCase());
}

/**
 * reflectEnabled mirrors the Go-side config.ResolveReflectEnabled precedence
 * so TS hooks that need to gate on the reflect setting see the same answer
 * as `aide reflect run`. Precedence:
 *
 *   1. AIDE_REFLECT env (recognised truthy/falsy values win)
 *   2. .aide/config/aide.json `reflect.enabled` at the resolved project
 *      root (walks up from cwd via findProjectRoot — does NOT just look
 *      at cwd/.aide/)
 *   3. default false
 *
 * Used by skill-injector.ts and opencode/hooks.ts to gate the user_prompt
 * observe-event emit that convergence detection depends on.
 */
export function reflectEnabled(cwd: string): boolean {
  const env = process.env.AIDE_REFLECT;
  if (env !== undefined && env !== "") {
    const norm = env.trim().toLowerCase();
    if (TRUTHY.has(norm)) return true;
    if (FALSY.has(norm)) return false;
  }
  try {
    const { root } = findProjectRoot(cwd);
    const cfgPath = join(root, ".aide", "config", "aide.json");
    if (existsSync(cfgPath)) {
      const cfg = JSON.parse(readFileSync(cfgPath, "utf-8")) as {
        reflect?: { enabled?: boolean };
      };
      return cfg?.reflect?.enabled === true;
    }
  } catch {
    // Unreadable / malformed config — treat as unset.
  }
  return false;
}

/**
 * Get the plugin root directory from environment variables.
 */
function getPluginRoot(): string | undefined {
  return process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
}

/**
 * Find the aide binary — Claude Code convenience wrapper.
 *
 * Reads AIDE_PLUGIN_ROOT / CLAUDE_PLUGIN_ROOT from the environment
 * and delegates to the platform-agnostic aide-client implementation.
 */
export function findAideBinary(cwd?: string): string | null {
  return clientFindBinary({ cwd, pluginRoot: getPluginRoot() });
}

/**
 * Run an aide command with the auto-discovered binary.
 */
export function runAide(cwd: string, args: string[]): string | null {
  const binary = findAideBinary(cwd);
  if (!binary) return null;
  return clientRunAide(binary, cwd, args);
}

/**
 * Set state in aide (global or per-agent)
 */
export function setMemoryState(
  cwd: string,
  key: string,
  value: string,
  agentId?: string,
): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) return false;
  return setState(binary, cwd, key, value, agentId);
}

/**
 * Get state from aide
 */
export function getMemoryState(
  cwd: string,
  key: string,
  agentId?: string,
): string | null {
  const binary = findAideBinary(cwd);
  if (!binary) return null;
  return getState(binary, cwd, key, agentId);
}

/**
 * Clear all state for an agent
 */
export function clearAgentState(cwd: string, agentId: string): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) return false;
  return clientClearAgentState(binary, cwd, agentId);
}
