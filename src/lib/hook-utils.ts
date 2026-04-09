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
 * Delete a state key from aide
 */
export function deleteMemoryState(
  cwd: string,
  key: string,
  agentId?: string,
): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) return false;
  return deleteState(binary, cwd, key, agentId);
}

/**
 * Clear all state for an agent
 */
export function clearAgentState(cwd: string, agentId: string): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) return false;
  return clientClearAgentState(binary, cwd, agentId);
}
