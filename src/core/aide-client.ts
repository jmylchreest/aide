/**
 * Platform-agnostic aide binary discovery and state operations.
 *
 * Extracted from src/lib/hook-utils.ts. Both Claude Code hooks and
 * OpenCode plugin use this module for binary discovery and state management.
 *
 * Key difference from hook-utils.ts: accepts explicit binary path and
 * platform-specific options instead of relying on CLAUDE_PLUGIN_ROOT.
 */

import { execFileSync } from "child_process";
import { existsSync, realpathSync } from "fs";
import { dirname, join } from "path";
import which from "which";
import type { FindBinaryOptions } from "./types.js";
import { debug } from "../lib/logger.js";
import { findProjectRoot } from "../lib/project-root.js";

const SOURCE = "aide-client";
const IS_WINDOWS = process.platform === "win32";
const BINARY_NAME = IS_WINDOWS ? "aide.exe" : "aide";

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
    } catch (err) {
      debug(SOURCE, `realpath failed for pluginRoot ${pluginRoot}: ${err}`);
    }
  }

  // 1. Plugin root bin/
  if (pluginRoot) {
    const pluginBinary = join(pluginRoot, "bin", BINARY_NAME);
    if (existsSync(pluginBinary)) {
      ensureBinInPath(dirname(pluginBinary));
      return pluginBinary;
    }
  }

  // 2. Project-local .aide/bin/ — at cwd, then at the resolved project
  // root (ensureBinSymlink plants the symlink at the ROOT, so a cwd deep
  // inside the repo — or inside a worktree/submodule whose store lives
  // elsewhere — must still find it).
  if (cwd) {
    const projectBinary = join(cwd, ".aide", "bin", BINARY_NAME);
    if (existsSync(projectBinary)) {
      ensureBinInPath(dirname(projectBinary));
      return projectBinary;
    }
    try {
      const { root } = findProjectRoot(cwd);
      if (root && root !== cwd) {
        const rootBinary = join(root, ".aide", "bin", BINARY_NAME);
        if (existsSync(rootBinary)) {
          ensureBinInPath(dirname(rootBinary));
          return rootBinary;
        }
      }
    } catch {
      /* resolution is best-effort here */
    }
  }

  // 3. Additional paths
  for (const searchPath of additionalPaths) {
    const binary = join(searchPath, BINARY_NAME);
    if (existsSync(binary)) {
      ensureBinInPath(dirname(binary));
      return binary;
    }
  }

  // 4. PATH fallback (already on PATH, no injection needed)
  const fromPath = which.sync("aide", { nothrow: true });
  if (fromPath) return fromPath;

  debug(SOURCE, `aide binary not found (checked pluginRoot=${pluginRoot || "none"}, cwd=${cwd || "none"}, PATH)`);
  return null;
}

/**
 * Ensure a directory is on PATH so child processes can find the aide binary.
 * Only prepends if not already present.
 */
function ensureBinInPath(binDir: string): void {
  const currentPath = process.env.PATH || "";
  const sep = process.platform === "win32" ? ";" : ":";
  if (!currentPath.split(sep).includes(binDir)) {
    process.env.PATH = `${binDir}${sep}${currentPath}`;
  }
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
    const env = options?.env ? { ...process.env, ...options.env } : process.env;
    return execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      timeout: options?.timeout ?? 10000,
      env,
    });
  } catch (err) {
    debug(SOURCE, `runAide failed: ${args.join(" ")}: ${err}`);
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
  } catch (err) {
    debug(SOURCE, `setState failed for key=${key}: ${err}`);
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
  } catch (err) {
    debug(SOURCE, `getState failed for key=${key}: ${err}`);
    return null;
  }
}

/**
 * Read session-scoped state (agent:<sessionId>:<key>), falling back to the
 * bare global key. Session-descriptive keys (mode, startedAt, toolCalls…)
 * are written session-scoped by hooks so concurrent sessions sharing one
 * store cannot clobber each other; the bare global spelling remains for
 * sessionless writers (skills, manual CLI use).
 */
export function getScopedState(
  binary: string,
  cwd: string,
  sessionId: string | undefined,
  key: string,
): string | null {
  if (sessionId) {
    const scoped = getState(binary, cwd, key, sessionId);
    if (scoped) return scoped;
  }
  return getState(binary, cwd, key);
}

// NOTE on `mode`: unlike the per-session counters above, mode is deliberately
// GLOBAL state. It is written by sessionless actors (`aide state set mode
// autopilot` from the autopilot skill, swarm orchestration) and its documented
// off-switch is `aide state set mode ""` — both only reachable as a global
// key. An earlier promote-to-session-scope design broke the off-switch,
// could lose the mode entirely on a failed scoped write, and starved shared
// swarm mode; per-session mode isolation needs real session identity (anchor
// work) rather than first-reader-wins claiming. Per-session iteration
// counters (`<mode>_iterations`) ARE session-scoped.

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
  } catch (err) {
    debug(SOURCE, `deleteState failed for key=${key}: ${err}`);
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
  } catch (err) {
    debug(SOURCE, `clearAgentState failed for agent=${agentId}: ${err}`);
    return false;
  }
}

/**
 * Sanitize a string for safe inclusion in log messages and CLI arguments.
 *
 * Strips control characters and limits length. This is NOT shell escaping —
 * use execFileSync (which avoids shells entirely) for subprocess execution.
 */
export function sanitizeForLog(str: string): string {
  // eslint-disable-next-line no-control-regex
  return str.replace(/[\x00-\x1f\x7f]/g, " ").slice(0, 1000);
}

/** @deprecated Use sanitizeForLog instead */
export const shellEscape = sanitizeForLog;
