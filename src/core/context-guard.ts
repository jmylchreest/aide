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
import { resolve, isAbsolute, normalize } from "path";
import { tmpdir } from "os";
import { join } from "path";
import { debug } from "../lib/logger.js";

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
