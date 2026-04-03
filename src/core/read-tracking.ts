/**
 * Read tracking — platform-agnostic core logic.
 *
 * Tracks file reads per session and checks file freshness against
 * the aide code index. Used by both Claude Code hooks and OpenCode plugin
 * to provide smart read hints (suggest code_outline/code_symbols over
 * redundant file re-reads).
 *
 * Gated behind AIDE_CODE_WATCH=1 (file watcher must be enabled).
 */

import { execFileSync } from "child_process";
import { isAbsolute, relative, resolve } from "path";
import { setState, getState } from "./aide-client.js";
import { debug } from "../lib/logger.js";

const SOURCE = "read-tracking";

/** Prefix for state keys tracking file reads */
const STATE_KEY_PREFIX = "file-read:";

/**
 * Result from checking file freshness against the code index.
 */
export interface ReadCheckResult {
  indexed: boolean;
  fresh: boolean;
  symbols: number;
  outline_available: boolean;
  estimated_tokens: number;
}

/**
 * Normalize a file path to a relative path from cwd.
 * Ensures consistent state keys regardless of absolute/relative input.
 */
function toRelativePath(cwd: string, filePath: string): string {
  const abs = isAbsolute(filePath) ? filePath : resolve(cwd, filePath);
  return relative(cwd, abs);
}

/**
 * Record that a file was read in this session.
 * Sets a state key so subsequent reads can be detected.
 *
 * No-op if AIDE_CODE_WATCH is not enabled.
 */
export function recordFileRead(
  binary: string,
  cwd: string,
  filePath: string,
): void {
  if (process.env.AIDE_CODE_WATCH !== "1") return;

  try {
    const relPath = toRelativePath(cwd, filePath);
    const key = STATE_KEY_PREFIX + relPath;
    setState(binary, cwd, key, new Date().toISOString());
    debug(SOURCE, `Recorded read: ${relPath}`);
  } catch (err) {
    debug(SOURCE, `Failed to record read: ${err}`);
  }
}

/**
 * Check if a file was previously read in this session.
 * Returns the ISO timestamp of the last read, or null if not read.
 *
 * Returns null if AIDE_CODE_WATCH is not enabled.
 */
export function getPreviousRead(
  binary: string,
  cwd: string,
  filePath: string,
): string | null {
  if (process.env.AIDE_CODE_WATCH !== "1") return null;

  try {
    const relPath = toRelativePath(cwd, filePath);
    const key = STATE_KEY_PREFIX + relPath;
    return getState(binary, cwd, key);
  } catch (err) {
    debug(SOURCE, `Failed to check previous read: ${err}`);
    return null;
  }
}

/**
 * Check whether a file is indexed and whether its content is fresh
 * (unchanged since last indexing) by calling `aide code read-check`.
 *
 * Returns null on any error (binary not found, command failed, etc.).
 */
export function checkFileReadFreshness(
  binary: string,
  cwd: string,
  filePath: string,
): ReadCheckResult | null {
  try {
    const relPath = toRelativePath(cwd, filePath);
    const output = execFileSync(binary, ["code", "read-check", relPath, "--json"], {
      cwd,
      encoding: "utf-8",
      timeout: 5000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    const result = JSON.parse(output.trim()) as ReadCheckResult;
    debug(SOURCE, `Read check ${relPath}: indexed=${result.indexed} fresh=${result.fresh} symbols=${result.symbols}`);
    return result;
  } catch (err) {
    debug(SOURCE, `Read check failed: ${err}`);
    return null;
  }
}

/**
 * Record a token event via `aide token record`.
 * Fire-and-forget — errors are logged but not propagated.
 */
export function recordTokenEvent(
  binary: string,
  cwd: string,
  eventType: string,
  tool: string,
  filePath: string,
  tokens: number,
  tokensSaved: number = 0,
): void {
  try {
    const args = ["token", "record", eventType, tool, filePath, String(tokens)];
    if (tokensSaved > 0) {
      args.push(String(tokensSaved));
    }
    execFileSync(binary, args, {
      cwd,
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    debug(SOURCE, `Token event: ${eventType} ${tool} ${filePath} tokens=${tokens} saved=${tokensSaved}`);
  } catch (err) {
    debug(SOURCE, `Failed to record token event: ${err}`);
  }
}

/**
 * Estimate tokens for a file by its size, using the default ratio.
 * This is a rough client-side estimate; the Go binary has per-language ratios.
 */
export function estimateTokensFromSize(sizeBytes: number): number {
  return Math.round(sizeBytes / 3.0);
}
