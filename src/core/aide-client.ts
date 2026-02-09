/**
 * Platform-agnostic aide binary discovery and state operations.
 *
 * Extracted from src/lib/hook-utils.ts. Both Claude Code hooks and
 * OpenCode plugin use this module for binary discovery and state management.
 *
 * Key difference from hook-utils.ts: accepts explicit binary path and
 * platform-specific options instead of relying on CLAUDE_PLUGIN_ROOT.
 */

import { execSync, execFileSync } from "child_process";
import { existsSync, realpathSync } from "fs";
import { join } from "path";
import type { FindBinaryOptions } from "./types.js";

/**
 * Find the aide binary — platform-agnostic implementation.
 *
 * Search order:
 *   1. pluginRoot/bin/aide (if pluginRoot provided)
 *   2. cwd/.aide/bin/aide  (project-local install)
 *   3. Each path in additionalPaths
 *   4. PATH fallback (system-wide install)
 */
export function findAideBinary(opts: FindBinaryOptions = {}): string | null {
  const { cwd, additionalPaths = [] } = opts;
  let { pluginRoot } = opts;

  // Resolve symlinks on pluginRoot
  if (pluginRoot) {
    try {
      pluginRoot = realpathSync(pluginRoot);
    } catch {
      // Keep original if realpath fails
    }
  }

  // 1. Plugin root bin/
  if (pluginRoot) {
    const pluginBinary = join(pluginRoot, "bin", "aide");
    if (existsSync(pluginBinary)) {
      return pluginBinary;
    }
  }

  // 2. Project-local .aide/bin/
  if (cwd) {
    const projectBinary = join(cwd, ".aide", "bin", "aide");
    if (existsSync(projectBinary)) {
      return projectBinary;
    }
  }

  // 3. Additional paths
  for (const searchPath of additionalPaths) {
    const binary = join(searchPath, "aide");
    if (existsSync(binary)) {
      return binary;
    }
  }

  // 4. PATH fallback
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
 * Run an aide command and return stdout, or null on failure.
 */
export function runAide(
  binary: string,
  cwd: string,
  args: string[],
  options?: { timeout?: number; env?: Record<string, string | undefined> },
): string | null {
  try {
    const env = options?.env
      ? { ...process.env, ...options.env }
      : process.env;
    return execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      timeout: options?.timeout ?? 10000,
      env,
    });
  } catch {
    return null;
  }
}

/**
 * Set state in aide (global or per-agent)
 */
export function setState(
  binary: string,
  cwd: string,
  key: string,
  value: string,
  agentId?: string,
): boolean {
  try {
    const args = ["state", "set", key, value];
    if (agentId) args.push(`--agent=${agentId}`);
    execFileSync(binary, args, { cwd, stdio: "pipe", timeout: 5000 });
    return true;
  } catch {
    return false;
  }
}

/**
 * Get state from aide
 */
export function getState(
  binary: string,
  cwd: string,
  key: string,
  agentId?: string,
): string | null {
  try {
    const args = ["state", "get", key];
    if (agentId) args.push(`--agent=${agentId}`);
    const output = execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      timeout: 5000,
    });
    const match = output.match(/=\s*(.+)$/m);
    return match ? match[1].trim() : null;
  } catch {
    return null;
  }
}

/**
 * Delete a state key from aide
 */
export function deleteState(
  binary: string,
  cwd: string,
  key: string,
  agentId?: string,
): boolean {
  try {
    const fullKey = agentId ? `agent:${agentId}:${key}` : key;
    execFileSync(binary, ["state", "delete", fullKey], {
      cwd,
      stdio: "pipe",
    });
    return true;
  } catch {
    return false;
  }
}

/**
 * Clear all state for an agent
 */
export function clearAgentState(
  binary: string,
  cwd: string,
  agentId: string,
): boolean {
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
 * Escape a string for safe shell usage (when shell is unavoidable)
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
