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
import { isTruthy } from "../lib/hook-utils.js";

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
  if (!isTruthy(process.env.AIDE_CODE_WATCH)) return;

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
  if (!isTruthy(process.env.AIDE_CODE_WATCH)) return null;

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

export function previewContent(text: string, maxChars = 300): string {
  const collapsed = text.replace(/\s+/g, " ").trim();
  if (collapsed.length <= maxChars) return collapsed;
  return collapsed.slice(0, maxChars - 1) + "…";
}

/**
 * Record an arbitrary observe event via `aide observe record`.
 * Prefer `emitInjectionEvent` for `kind=injection` callers — this raw
 * recorder is reserved for non-injection kinds (e.g. `hook` user_prompt
 * events). Fire-and-forget.
 */
export function recordObserveEvent(
  binary: string,
  cwd: string,
  opts: {
    kind: string;
    name: string;
    category?: string;
    subtype?: string;
    tokens?: number;
    saved?: number;
    file?: string;
    session?: string;
    attrs?: Record<string, string>;
  },
): void {
  try {
    const args = ["observe", "record", `--kind=${opts.kind}`, `--name=${opts.name}`];
    if (opts.category) args.push(`--category=${opts.category}`);
    if (opts.subtype) args.push(`--subtype=${opts.subtype}`);
    if (opts.tokens !== undefined) args.push(`--tokens=${opts.tokens}`);
    if (opts.saved !== undefined) args.push(`--saved=${opts.saved}`);
    if (opts.file) args.push(`--file=${opts.file}`);
    if (opts.session) args.push(`--session=${opts.session}`);
    for (const [k, v] of Object.entries(opts.attrs ?? {})) {
      args.push(`--attr=${k}=${v}`);
    }
    execFileSync(binary, args, {
      cwd,
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    debug(SOURCE, `Observe event: ${opts.kind} ${opts.name} subtype=${opts.subtype ?? ""} tokens=${opts.tokens ?? 0}`);
  } catch (err) {
    debug(SOURCE, `Failed to record observe event: ${err}`);
  }
}

/**
 * Emit a `kind=injection` observe event for any hook that pushes
 * `additionalContext` back to the harness. Centralises field naming so the
 * Injections page can group/colour consistently.
 *
 * `subtype` should come from a small fixed taxonomy:
 *   memory | decision | session_memory | skill | enrichment | guard |
 *   signal | pruning
 *
 * `source` is the emitting hook name (e.g. "search-enrichment"); it lands in
 * both `file` and `name` so the UI can show "who injected this" without
 * forcing every caller to invent a unique `name`.
 *
 * Fire-and-forget; failures are logged at debug level and never thrown.
 */
export function emitInjectionEvent(
  binary: string,
  cwd: string,
  opts: {
    source: string;
    subtype: string;
    content: string;
    sessionId?: string;
    name?: string;
    attrs?: Record<string, string>;
  },
): void {
  const baseAttrs: Record<string, string> = {
    source_id: opts.source,
    source_kind: opts.subtype,
    content_preview: previewContent(opts.content, 2000),
  };
  recordObserveEvent(binary, cwd, {
    kind: "injection",
    name: opts.name ?? opts.source,
    category: "inject",
    subtype: opts.subtype,
    tokens: Math.round(opts.content.length / 3.0),
    file: opts.source,
    session: opts.sessionId,
    attrs: { ...baseAttrs, ...(opts.attrs ?? {}) },
  });
}
