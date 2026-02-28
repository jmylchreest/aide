/**
 * Shared utilities for Claude Code hooks.
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
