/**
 * Shared utilities for Claude Code hooks
 *
 * This module provides common functions used across multiple hooks
 * to reduce code duplication and ensure consistent behavior.
 */

import { execSync, execFileSync } from "child_process";
import { join } from "path";

/**
 * Read JSON input from stdin (used by all hooks)
 */
export async function readStdin(): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf-8");
}

/**
 * Find the aide binary in common locations
 */
export function findAide(cwd: string): string | null {
  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT;
  const paths = [
    pluginRoot ? join(pluginRoot, "bin", "aide") : null,
    join(cwd, ".aide", "bin", "aide"),
    join(cwd, "bin", "aide"),
    join(process.env.HOME || "", ".aide", "bin", "aide"),
    "aide",
  ].filter((p): p is string => p !== null);

  for (const p of paths) {
    try {
      execSync(`test -x "${p}"`, { stdio: "pipe" });
      return p;
    } catch {
      // Not found at this location
    }
  }

  // Try finding in PATH
  try {
    const result = execSync("which aide", { stdio: "pipe" }).toString().trim();
    if (result) return result;
  } catch {
    // Not in PATH
  }

  return null;
}

/**
 * @deprecated Use findAide instead
 */
export const findAideMemory = findAide;

/**
 * Escape a string for safe shell usage
 */
export function shellEscape(str: string): string {
  return str
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\$/g, "\\$")
    .replace(/`/g, "\\`")
    .replace(/!/g, "\\!")
    .replace(/\n/g, " ")
    .slice(0, 1000);
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
  const binary = findAide(cwd);
  if (!binary) return false;

  try {
    const args = ["state", "set", key, value];
    if (agentId) args.push(`--agent=${agentId}`);
    execFileSync(binary, args, { cwd, stdio: "pipe" });
    return true;
  } catch {
    return false;
  }
}

/**
 * Get state from aide
 */
export function getMemoryState(
  cwd: string,
  key: string,
  agentId?: string,
): string | null {
  const binary = findAide(cwd);
  if (!binary) return null;

  try {
    const args = ["state", "get", key];
    if (agentId) args.push(`--agent=${agentId}`);
    const output = execFileSync(binary, args, { cwd, encoding: "utf-8" });
    // Parse output format: "key = value" or "[agent] key = value"
    const match = output.match(/=\s*(.+)$/m);
    return match ? match[1].trim() : null;
  } catch {
    return null;
  }
}

/**
 * Delete a state key from aide
 */
export function deleteMemoryState(
  cwd: string,
  key: string,
  agentId?: string,
): boolean {
  const binary = findAide(cwd);
  if (!binary) return false;

  try {
    // For agent-specific keys, we need to construct the full key
    const fullKey = agentId ? `agent:${agentId}:${key}` : key;
    execFileSync(binary, ["state", "delete", fullKey], { cwd, stdio: "pipe" });
    return true;
  } catch {
    return false;
  }
}

/**
 * Clear all state for an agent
 */
export function clearAgentState(cwd: string, agentId: string): boolean {
  const binary = findAide(cwd);
  if (!binary) return false;

  try {
    execFileSync(binary, ["state", "clear", `--agent=${agentId}`], {
      cwd,
      stdio: "pipe",
    });
    return true;
  } catch {
    return false;
  }
}

/**
 * Run an aide command with proper escaping
 */
export function runAide(cwd: string, args: string[]): string | null {
  const binary = findAide(cwd);
  if (!binary) return null;

  try {
    return execFileSync(binary, args, { cwd, encoding: "utf-8" });
  } catch {
    return null;
  }
}

/**
 * @deprecated Use runAide instead
 */
export const runAideMemory = runAide;

/**
 * Update session heartbeat (lastSeen timestamp)
 * Called by hooks to indicate the session is still alive
 */
export function updateSessionHeartbeat(
  cwd: string,
  sessionId: string,
): boolean {
  return setMemoryState(
    cwd,
    `session:${sessionId}:lastSeen`,
    Date.now().toString(),
  );
}

/**
 * Get session heartbeat timestamp
 *
 * Note: Currently only used internally by isSessionAlive().
 * Exported for potential future use by cleanup utilities.
 */
export function getSessionHeartbeat(
  cwd: string,
  sessionId: string,
): number | null {
  const value = getMemoryState(cwd, `session:${sessionId}:lastSeen`);
  return value ? parseInt(value, 10) : null;
}

/**
 * Check if a session is considered alive (heartbeat within threshold)
 * Default threshold: 30 minutes
 *
 * Note: Currently not used by hooks. session-start.ts implements
 * similar logic inline. Exported for potential future cleanup utilities.
 */
export function isSessionAlive(
  cwd: string,
  sessionId: string,
  thresholdMs: number = 30 * 60 * 1000,
): boolean {
  const lastSeen = getSessionHeartbeat(cwd, sessionId);
  if (!lastSeen) return false;
  return Date.now() - lastSeen < thresholdMs;
}

/**
 * Get all sessions with their last heartbeat
 */
export function getSessionHeartbeats(cwd: string): Map<string, number> {
  const binary = findAide(cwd);
  if (!binary) return new Map();

  try {
    const output = execFileSync(binary, ["state", "list"], {
      cwd,
      encoding: "utf-8",
    });
    const sessions = new Map<string, number>();

    for (const line of output.split("\n")) {
      // Match: session:<sessionId>:lastSeen = <timestamp>
      const match = line.match(/^session:([^:]+):lastSeen\s*=\s*(\d+)/);
      if (match) {
        sessions.set(match[1], parseInt(match[2], 10));
      }
    }

    return sessions;
  } catch {
    return new Map();
  }
}
