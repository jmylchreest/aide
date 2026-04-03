/**
 * Context Guard — platform-agnostic core logic.
 *
 * Monitors Read tool calls and advises agents to use code_outline
 * before reading large files. This preserves context window for
 * the actual task by avoiding dumping entire files into conversation.
 *
 * Behaviour:
 *   - Triggers on Read tool calls for files > 5KB (~150 lines)
 *   - Tracks which files have been outlined (code_outline/code_symbols)
 *   - Returns an advisory message (never blocks)
 *   - Also tracks code_outline/code_symbols calls to mark files as "known"
 *
 * Used by both Claude Code hooks (PreToolUse) and OpenCode plugin.
 */

import { statSync, readFileSync, writeFileSync, existsSync } from "fs";
import { resolve, isAbsolute, normalize, extname } from "path";
import { tmpdir } from "os";
import { join } from "path";
import { debug } from "../lib/logger.js";
import { getPreviousRead, checkFileReadFreshness } from "./read-tracking.js";

const SOURCE = "context-guard";

/** Default size threshold in bytes (~150 lines) */
const DEFAULT_SIZE_THRESHOLD = 5120;

/** File extensions that are typically not source code (skip advisory) */
const SKIP_EXTENSIONS = new Set([
  ".json",
  ".lock",
  ".sum",
  ".mod",
  ".yaml",
  ".yml",
  ".toml",
  ".env",
  ".md",
  ".txt",
  ".csv",
  ".svg",
  ".png",
  ".jpg",
  ".gif",
  ".ico",
  ".woff",
  ".woff2",
  ".ttf",
  ".eot",
]);

export interface ContextGuardResult {
  /** Whether to inject an advisory message */
  shouldAdvise: boolean;
  /** Advisory message to inject */
  advisory?: string;
  /** Whether this call should be tracked (code_outline/code_symbols) */
  tracked?: boolean;
}

/**
 * Get the path to the tracking file for this session.
 */
function getTrackingPath(sessionId: string): string {
  return join(tmpdir(), `aide-context-guard-${sessionId}.json`);
}

/**
 * Load the set of files that have been outlined in this session.
 */
function loadOutlinedFiles(sessionId: string): Set<string> {
  const trackingPath = getTrackingPath(sessionId);
  try {
    if (existsSync(trackingPath)) {
      const data = JSON.parse(readFileSync(trackingPath, "utf-8"));
      return new Set(data.files || []);
    }
  } catch {
    // Corrupted file, start fresh
  }
  return new Set();
}

/**
 * Save a file as "outlined" in the tracking file.
 */
function trackOutlinedFile(sessionId: string, filePath: string): void {
  const files = loadOutlinedFiles(sessionId);
  files.add(filePath);
  const trackingPath = getTrackingPath(sessionId);
  try {
    writeFileSync(
      trackingPath,
      JSON.stringify({ files: Array.from(files) }),
      "utf-8",
    );
  } catch (err) {
    debug(SOURCE, `Failed to write tracking file: ${err}`);
  }
}

/**
 * Estimate line count from file size (rough: ~35 bytes per line average).
 */
function estimateLines(sizeBytes: number): number {
  return Math.round(sizeBytes / 35);
}

/**
 * Check whether a Read call should receive a context-efficiency advisory.
 *
 * Also handles tracking code_outline/code_symbols calls.
 */
export function checkContextGuard(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
  sessionId: string,
): ContextGuardResult {
  const normalizedTool = toolName.toLowerCase();

  // Track code_outline and code_symbols calls
  if (
    normalizedTool.includes("code_outline") ||
    normalizedTool.includes("code_symbols")
  ) {
    const filePath =
      (toolInput.file as string) ||
      (toolInput.filePath as string) ||
      (toolInput.file_path as string);
    if (filePath && sessionId) {
      const resolved = normalize(
        isAbsolute(filePath) ? filePath : resolve(cwd, filePath),
      );
      trackOutlinedFile(sessionId, resolved);
      debug(SOURCE, `Tracked outline for: ${filePath}`);
    }
    return { shouldAdvise: false, tracked: true };
  }

  // Only advise on Read tool calls
  if (normalizedTool !== "read") {
    return { shouldAdvise: false };
  }

  // Extract file path from tool input
  const filePath =
    (toolInput.filePath as string) ||
    (toolInput.file_path as string) ||
    (toolInput.path as string);

  if (!filePath) {
    return { shouldAdvise: false };
  }

  // Resolve to absolute path
  const resolvedPath = normalize(
    isAbsolute(filePath) ? filePath : resolve(cwd, filePath),
  );

  // Skip non-source-code files
  const ext = filePath.substring(filePath.lastIndexOf(".")).toLowerCase();
  if (SKIP_EXTENSIONS.has(ext)) {
    return { shouldAdvise: false };
  }

  // Check if the agent is already using offset/limit (targeted read)
  const offset = toolInput.offset as number | undefined;
  const limit = toolInput.limit as number | undefined;
  if (offset !== undefined && offset > 1) {
    // Agent is doing a targeted read — no advisory needed
    return { shouldAdvise: false };
  }
  if (limit !== undefined && limit < 100) {
    // Agent is limiting the read — no advisory needed
    return { shouldAdvise: false };
  }

  // Check file size
  let fileSize: number;
  try {
    const stat = statSync(resolvedPath);
    fileSize = stat.size;
  } catch {
    // Can't stat file — don't advise (file might not exist yet)
    return { shouldAdvise: false };
  }

  // Skip small files
  if (fileSize < DEFAULT_SIZE_THRESHOLD) {
    return { shouldAdvise: false };
  }

  // Check if this file has already been outlined in this session
  if (sessionId) {
    const outlinedFiles = loadOutlinedFiles(sessionId);
    if (outlinedFiles.has(resolvedPath)) {
      debug(SOURCE, `File already outlined, skipping advisory: ${filePath}`);
      return { shouldAdvise: false };
    }
  }

  // Generate advisory
  const estLines = estimateLines(fileSize);
  const sizeKB = (fileSize / 1024).toFixed(1);

  const advisory =
    `[aide:context] This file is ~${estLines} lines (${sizeKB}KB). Consider using \`code_outline\` ` +
    `first to see its structure, then \`Read\` with offset/limit for specific sections. ` +
    `This preserves your context window for the full task.`;

  debug(SOURCE, `Advisory for ${filePath}: ${estLines} lines, ${sizeKB}KB`);
  return { shouldAdvise: true, advisory };
}

// ============================================================================
// Smart Read Hint — suggest code index tools over redundant file re-reads
// ============================================================================

export interface SmartReadHintResult {
  /** Whether to inject a hint message */
  shouldHint: boolean;
  /** Hint message to inject */
  hint?: string;
}

/**
 * Check whether a Read call should receive a smart-read hint suggesting
 * the agent use code_outline/code_symbols/code_references instead.
 *
 * Triggers when:
 *   1. The file was already read this session (tracked via state store)
 *   2. The file hasn't changed since last indexing (mtime comparison)
 *   3. The file is indexed with symbols (code_outline would be useful)
 *
 * Gated behind AIDE_CODE_WATCH=1 and requires a valid aide binary.
 */
export function checkSmartReadHint(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
  binary: string | null,
): SmartReadHintResult {
  // Only advise on Read tool calls (case-insensitive for OpenCode compat)
  if (toolName.toLowerCase() !== "read") {
    return { shouldHint: false };
  }

  // Require code watcher to be enabled
  if (process.env.AIDE_CODE_WATCH !== "1") {
    return { shouldHint: false };
  }

  // Require aide binary
  if (!binary) {
    return { shouldHint: false };
  }

  // Extract file path from tool input (check multiple variants)
  // Precedence matches checkContextGuard and checkWriteGuard
  const filePath =
    (toolInput.filePath as string) ||
    (toolInput.file_path as string) ||
    (toolInput.path as string);

  if (!filePath) {
    return { shouldHint: false };
  }

  // Skip targeted reads (agent already using offset/limit)
  const offset = toolInput.offset as number | undefined;
  const limit = toolInput.limit as number | undefined;
  if (offset !== undefined && offset > 1) {
    return { shouldHint: false };
  }
  if (limit !== undefined && limit < 100) {
    return { shouldHint: false };
  }

  // Skip non-source-code files
  const ext = extname(filePath).toLowerCase();
  if (SKIP_EXTENSIONS.has(ext)) {
    return { shouldHint: false };
  }

  // Check if this file was already read this session
  const previousRead = getPreviousRead(binary, cwd, filePath);
  if (!previousRead) {
    // First read — no hint needed
    return { shouldHint: false };
  }

  // Check if the file is indexed and fresh
  const readCheck = checkFileReadFreshness(binary, cwd, filePath);
  if (!readCheck) {
    return { shouldHint: false };
  }

  if (readCheck.indexed && readCheck.fresh && readCheck.outline_available) {
    const tokens = readCheck.estimated_tokens;
    const tokenInfo = tokens > 0 ? ` (~${tokens} tokens)` : "";
    const hint =
      `[aide:smart-read] This file was already read this session and hasn't changed${tokenInfo}. ` +
      `Consider using code_outline (for structure, ~5-15% of full tokens), ` +
      `code_symbols (for API surface), or code_references (for call sites) ` +
      `to avoid re-reading the full file.`;

    debug(SOURCE, `Smart read hint for: ${filePath} (${tokens} tokens)`);
    return { shouldHint: true, hint };
  }

  return { shouldHint: false };
}
