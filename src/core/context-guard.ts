/**
 * Context Guard — platform-agnostic core logic.
 *
 * Monitors Read tool calls and advises agents to use code_outline
 * before reading large files. This preserves context window for
 * the actual task by avoiding dumping entire files into conversation.
 *
 * Behaviour:
 *   - Triggers on Read tool calls for files > 5KB (~150 lines)
 *   - Returns an advisory message (never blocks)
 *
 * Used by both Claude Code hooks (PreToolUse) and OpenCode plugin.
 */

import { statSync } from "fs";
import { resolve, isAbsolute, normalize, extname } from "path";
import { debug } from "../lib/logger.js";
import { codeWatchEnabled } from "../lib/hook-utils.js";
import { getPreviousRead, checkFileReadFreshness } from "./read-tracking.js";
import type { ContextIdentity } from "./context-window.js";

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
 * Estimate line count from file size (rough: ~35 bytes per line average).
 */
function estimateLines(sizeBytes: number): number {
  return Math.round(sizeBytes / 35);
}

/**
 * Check whether a Read call should receive a context-efficiency advisory.
 *
 * PreToolUse cannot establish that an outline was successfully delivered.
 */
export function checkContextGuard(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
  _sessionId: string,
): ContextGuardResult {
  const normalizedTool = toolName.toLowerCase();

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
 *   1. Full-file text was observed in the current context window
 *   2. The file still matches those observed bytes (content hash)
 *   3. The file is indexed with symbols (code_outline would be useful)
 *
 * Gated on code.watch (default on) and requires a valid aide binary.
 */
export function checkSmartReadHint(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
  binary: string | null,
  identity?: ContextIdentity,
): SmartReadHintResult {
  // Only advise on Read tool calls (case-insensitive for OpenCode compat)
  if (toolName.toLowerCase() !== "read") {
    return { shouldHint: false };
  }

  // Require code watcher to be enabled
  if (!codeWatchEnabled(cwd)) {
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
  const previousRead = getPreviousRead(binary, cwd, filePath, identity);
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
      `[aide:smart-read] Matching full-file text was observed in this context window${tokenInfo}. ` +
      `Consider using code_outline (for structure), ` +
      `code_symbols (for API surface), or code_references (for call sites) ` +
      `to avoid re-reading the full file.`;

    debug(SOURCE, `Smart read hint for: ${filePath} (${tokens} tokens)`);
    return { shouldHint: true, hint };
  }

  return { shouldHint: false };
}
