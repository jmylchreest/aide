/**
 * Dedup strategy: replace repeated identical tool outputs with a short pointer.
 *
 * Safe-to-dedup tools: Read (with mtime check), Glob, Grep, and aide MCP tools
 * like code_search, code_symbols, code_outline, code_references,
 * findings_list, findings_search, memory_list, memory_search.
 *
 * NEVER dedup: Bash, Write, Edit, or any tool with side effects.
 */

import type { PruneResult, PruneStrategy, ToolRecord } from "./types.js";
import { statSync } from "fs";
import { resolve, isAbsolute } from "path";

/** Tools that are safe to deduplicate. */
const SAFE_DEDUP_TOOLS = new Set([
  // Host built-in read-only tools
  "read",
  "glob",
  "grep",
  // aide MCP tools (read-only)
  "mcp__aide__code_search",
  "mcp__aide__code_symbols",
  "mcp__aide__code_outline",
  "mcp__aide__code_references",
  "mcp__aide__code_stats",
  "mcp__aide__findings_list",
  "mcp__aide__findings_search",
  "mcp__aide__findings_stats",
  "mcp__aide__memory_list",
  "mcp__aide__memory_search",
  "mcp__aide__decision_list",
  "mcp__aide__decision_get",
  "mcp__aide__decision_history",
  "mcp__aide__state_get",
  "mcp__aide__state_list",
  "mcp__aide__task_list",
  "mcp__aide__task_get",
  "mcp__aide__message_list",
  // Claude Code naming convention (no mcp__ prefix)
  "code_search",
  "code_symbols",
  "code_outline",
  "code_references",
  "code_stats",
  "findings_list",
  "findings_search",
  "findings_stats",
  "memory_list",
  "memory_search",
  "decision_list",
  "decision_get",
  "decision_history",
  "state_get",
  "state_list",
  "task_list",
  "task_get",
  "message_list",
]);

/** Extract the dedup key from tool args (the args that define "same call"). */
function dedupKey(toolName: string, args: Record<string, unknown>): string {
  const normalized = toolName.toLowerCase();
  // For Read, the key is filePath + offset + limit
  if (normalized === "read") {
    return JSON.stringify({
      tool: "read",
      filePath: args.filePath ?? args.file_path ?? args.path,
      offset: args.offset ?? 0,
      limit: args.limit ?? 2000,
    });
  }
  // For Glob, the key is pattern + path
  if (normalized === "glob") {
    return JSON.stringify({
      tool: "glob",
      pattern: args.pattern,
      path: args.path,
    });
  }
  // For Grep, the key is pattern + path + include
  if (normalized === "grep") {
    return JSON.stringify({
      tool: "grep",
      pattern: args.pattern,
      path: args.path,
      include: args.include,
    });
  }
  // For MCP tools, use all args as the key
  return JSON.stringify({ tool: normalized, ...args });
}

/** Check file mtime for Read dedup safety. */
function getFileMtime(
  args: Record<string, unknown>,
  cwd?: string,
): number | undefined {
  const filePath =
    (args.filePath as string) ??
    (args.file_path as string) ??
    (args.path as string);
  if (!filePath) return undefined;

  try {
    const resolved = isAbsolute(filePath)
      ? filePath
      : resolve(cwd || process.cwd(), filePath);
    return statSync(resolved).mtimeMs;
  } catch {
    return undefined;
  }
}

export class DedupStrategy implements PruneStrategy {
  name = "dedup" as const;
  private cwd?: string;

  constructor(cwd?: string) {
    this.cwd = cwd;
  }

  apply(
    toolName: string,
    args: Record<string, unknown>,
    output: string,
    history: ToolRecord[],
  ): PruneResult {
    const normalized = toolName.toLowerCase();

    // Only apply to safe tools
    if (!SAFE_DEDUP_TOOLS.has(normalized)) {
      return { output, modified: false, bytesSaved: 0 };
    }

    const key = dedupKey(toolName, args);

    // Find the most recent matching call in history
    for (let i = history.length - 1; i >= 0; i--) {
      const prev = history[i];
      const prevKey = dedupKey(prev.toolName, prev.args);

      if (prevKey !== key) continue;

      // For Read: check mtime hasn't changed (file might have been edited)
      if (normalized === "read") {
        const currentMtime = getFileMtime(args, this.cwd);
        if (
          currentMtime !== undefined &&
          prev.fileMtime !== undefined &&
          currentMtime !== prev.fileMtime
        ) {
          // File changed — don't dedup
          continue;
        }
      }

      // Check if output is identical
      const prevOutput = prev.prunedOutput ?? prev.originalOutput;
      if (output === prevOutput) {
        const replacement = `[aide:dedup] Identical to previous ${toolName} call (callId: ${prev.callId}). Output unchanged.`;
        return {
          output: replacement,
          modified: true,
          strategy: "dedup",
          bytesSaved: output.length - replacement.length,
        };
      }
    }

    return { output, modified: false, bytesSaved: 0 };
  }
}
