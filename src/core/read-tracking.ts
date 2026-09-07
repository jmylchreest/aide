/**
 * Read tracking — platform-agnostic core logic.
 *
 * Tracks matching full-file results per host/session/actor/context window.
 * The independent index check is advisory, never evidence of prior coverage.
 * Used by both Claude Code hooks and OpenCode plugin
 * to provide smart read hints (suggest code_outline/code_symbols over
 * redundant file re-reads).
 *
 * Gated on code.watch (default on; disable via AIDE_CODE_WATCH=0 or config).
 */

import { execFileSync } from "child_process";
import { createHash } from "crypto";
import { readFileSync } from "fs";
import { isAbsolute, relative, resolve } from "path";
import { setState, getState } from "./aide-client.js";
import { debug } from "../lib/logger.js";
import { codeWatchEnabled } from "../lib/hook-utils.js";
import {
  contextScope,
  contextWindow,
  type ContextIdentity,
} from "./context-window.js";

const SOURCE = "read-tracking";

/** Prefix for state keys tracking file reads */
const STATE_KEY_PREFIX = "verified-file-read:";

function readKey(
  cwd: string,
  file: string,
  identity: ContextIdentity,
  epoch: string,
): string {
  return (
    STATE_KEY_PREFIX +
    createHash("sha256")
      .update(
        JSON.stringify([
          contextScope(identity),
          epoch,
          toRelativePath(cwd, file),
        ]),
      )
      .digest("hex")
  );
}

export interface ReadEvidence {
  identity: ContextIdentity;
  /** Raw returned source text, without line numbers or host decorations. */
  content?: string;
}

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
 * Record full-file coverage only when returned text matches current bytes.
 * Formatted/partial results are still measured by tool-observe, but do not
 * establish full-file coverage here. Missing context identity is unknown.
 *
 * No-op if code.watch is disabled.
 */
export function recordFileRead(
  binary: string,
  cwd: string,
  filePath: string,
  evidence?: ReadEvidence,
): void {
  if (!codeWatchEnabled(cwd) || !evidence || evidence.content === undefined)
    return;

  try {
    const window = contextWindow(binary, cwd, evidence.identity);
    if (!window || window.status !== "active") return;
    const current = readFileSync(resolve(cwd, filePath));
    // Only establish full coverage when returned bytes exactly match the
    // file. Requested ranges, formatted output and opaque results prove less.
    if (!current.equals(Buffer.from(evidence.content, "utf8"))) return;
    const key = readKey(cwd, filePath, evidence.identity, window.id);
    setState(
      binary,
      cwd,
      key,
      JSON.stringify({
        version: 1,
        hash: createHash("sha256").update(current).digest("hex"),
        at: new Date().toISOString(),
      }),
    );
  } catch (err) {
    debug(SOURCE, `Failed to record read: ${err}`);
  }
}

/**
 * Check for matching full-file text in the current active context window.
 * Returns the ISO timestamp of the last read, or null if not read.
 *
 * Returns null when disabled, changed, incomplete or continuity is unknown.
 */
export function getPreviousRead(
  binary: string,
  cwd: string,
  filePath: string,
  identity?: ContextIdentity,
): string | null {
  if (!codeWatchEnabled(cwd) || !identity) return null;

  try {
    const window = contextWindow(binary, cwd, identity);
    if (!window || window.status !== "active") return null;
    const record = JSON.parse(
      getState(binary, cwd, readKey(cwd, filePath, identity, window.id)) ??
        "null",
    );
    if (record?.version !== 1 || typeof record.at !== "string") return null;
    const hash = createHash("sha256")
      .update(readFileSync(resolve(cwd, filePath)))
      .digest("hex");
    return hash === record.hash ? record.at : null;
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
    const output = execFileSync(
      binary,
      ["code", "read-check", relPath, "--json"],
      {
        cwd,
        encoding: "utf-8",
        timeout: 5000,
        stdio: ["pipe", "pipe", "pipe"],
      },
    );
    const result = JSON.parse(output.trim()) as ReadCheckResult;
    debug(
      SOURCE,
      `Read check ${relPath}: indexed=${result.indexed} fresh=${result.fresh} symbols=${result.symbols}`,
    );
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

/** One event for the batch recorder — mirrors observe record --stdin. */
export interface ObserveBatchEvent {
  kind: string;
  name: string;
  category?: string;
  subtype?: string;
  tokens?: number;
  saved?: number;
  file?: string;
  session?: string;
  attrs?: Record<string, string>;
}

/**
 * Record many observe events in ONE binary spawn via
 * `observe record --stdin` (JSON Lines). Per-event spawns cost a process
 * start plus a bolt open each — a session start injecting dozens of
 * sources paid seconds for what is one write transaction. Falls back to
 * per-event recording when the binary predates --stdin. Fire-and-forget.
 */
export function recordObserveEventsBatch(
  binary: string,
  cwd: string,
  events: ObserveBatchEvent[],
): void {
  if (events.length === 0) return;
  try {
    const input = events.map((e) => JSON.stringify(e)).join("\n") + "\n";
    execFileSync(binary, ["observe", "record", "--stdin"], {
      cwd,
      input,
      timeout: 10000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    debug(SOURCE, `Observe batch: recorded ${events.length} event(s)`);
  } catch (err) {
    debug(SOURCE, `Observe batch failed (${err}); falling back to per-event`);
    for (const e of events) {
      recordObserveEvent(binary, cwd, e);
    }
  }
}

/**
 * Build the `kind=injection` batch event for one injected source — the
 * batch counterpart of emitInjectionEvent, sharing its field naming.
 */
export function injectionBatchEvent(opts: {
  source: string;
  subtype: string;
  content: string;
  sessionId?: string;
  name?: string;
  attrs?: Record<string, string>;
}): ObserveBatchEvent {
  return {
    kind: "injection",
    name: opts.name ?? opts.source,
    category: "inject",
    subtype: opts.subtype,
    tokens: Math.round(opts.content.length / 3.0),
    file: opts.source,
    session: opts.sessionId,
    attrs: {
      source_id: opts.source,
      source_kind: opts.subtype,
      content_preview: previewContent(opts.content, 2000),
      ...(opts.attrs ?? {}),
    },
  };
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
    const args = [
      "observe",
      "record",
      `--kind=${opts.kind}`,
      `--name=${opts.name}`,
    ];
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
    debug(
      SOURCE,
      `Observe event: ${opts.kind} ${opts.name} subtype=${opts.subtype ?? ""} tokens=${opts.tokens ?? 0}`,
    );
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
  recordObserveEvent(binary, cwd, injectionBatchEvent(opts));
}
