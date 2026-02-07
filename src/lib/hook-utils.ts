/**
 * Shared utilities for Claude Code hooks
 *
 * This module provides common functions used across multiple hooks
 * to reduce code duplication and ensure consistent behavior.
 */

import { execSync, execFileSync } from "child_process";
import { existsSync, realpathSync } from "fs";
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
 * Find the aide binary — canonical implementation.
 *
 * Search order:
 *   1. <CLAUDE_PLUGIN_ROOT>/bin/aide  (CLAUDE_PLUGIN_ROOT = project root)
 *   2. PATH fallback                  (system-wide install)
 *
 * All hooks and utilities should use this single function.
 */
export function findAideBinary(cwd?: string): string | null {
  let pluginRoot = process.env.CLAUDE_PLUGIN_ROOT;
  // Resolve symlinks (e.g., src-office -> dtkr4-cnjjf)
  if (pluginRoot) {
    try {
      pluginRoot = realpathSync(pluginRoot);
    } catch {
      // Keep original if realpath fails
    }
  }
  if (pluginRoot) {
    const pluginBinary = join(pluginRoot, "bin", "aide");
    if (existsSync(pluginBinary)) {
      return pluginBinary;
    }
  }

  // PATH fallback
  try {
    const result = execSync("which aide", { stdio: "pipe", timeout: 2000 })
      .toString()
      .trim();
    if (result) return result;
  } catch {
    // Not in PATH
  }

  return null;
}


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
  const binary = findAideBinary(cwd);
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
  const binary = findAideBinary(cwd);
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
  const binary = findAideBinary(cwd);
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
  const binary = findAideBinary(cwd);
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
  const binary = findAideBinary(cwd);
  if (!binary) return null;

  try {
    return execFileSync(binary, args, { cwd, encoding: "utf-8" });
  } catch {
    return null;
  }
}


