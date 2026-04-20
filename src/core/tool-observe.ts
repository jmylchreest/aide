/**
 * Tool observability — single source of truth for native tool → observe.Event
 * mapping. Used by both the Claude Code PostToolUse hook and the OpenCode
 * tool.execute.after handler so dashboard categorisation stays consistent
 * across plugins.
 *
 * Mirror image of the MCP-side mcpToolTaxonomy in cmd_mcp.go: native tools
 * (Read, Edit, Bash, ...) flow through here; MCP tools (code_outline,
 * findings_search, ...) flow through the middleware. Together they give
 * complete tool-call coverage in the observe store.
 */

import { execFileSync } from "child_process";
import { statSync } from "fs";
import { isAbsolute, resolve } from "path";
import { debug } from "../lib/logger.js";
import { recordFileRead } from "./read-tracking.js";

const SOURCE = "tool-observe";

/**
 * Category + subtype for one native tool. Categories mirror the MCP taxonomy:
 *   consume   — pulls content into context (Read)
 *   modify    — changes files (Edit, Write, NotebookEdit)
 *   search    — finds things without consuming much (Grep, Glob)
 *   execute   — runs external commands (Bash)
 *   network   — fetches over the network (WebFetch, WebSearch)
 *   coordinate— delegates work (Task)
 *   navigate  — read-only state queries (TodoWrite read-side, etc.)
 */
interface ToolTax {
  category: string;
  subtype: string;
}

/**
 * Native tool → (category, subtype). Names use Claude Code's canonical casing
 * (PascalCase). The OpenCode call site lowercases before lookup so the same
 * table serves both ("read" → "Read").
 */
const NATIVE_TOOL_TAXONOMY: Record<string, ToolTax> = {
  Read: { category: "consume", subtype: "file" },
  Edit: { category: "modify", subtype: "file" },
  Write: { category: "modify", subtype: "file" },
  NotebookEdit: { category: "modify", subtype: "notebook" },
  Grep: { category: "search", subtype: "content" },
  Glob: { category: "search", subtype: "path" },
  Bash: { category: "execute", subtype: "shell" },
  WebFetch: { category: "network", subtype: "fetch" },
  WebSearch: { category: "network", subtype: "search" },
  Task: { category: "coordinate", subtype: "subagent" },
  TodoWrite: { category: "coordinate", subtype: "todo" },
};

/** Tools whose tokens we estimate from file size on the recording side. */
const FILE_SIZED_TOOLS = new Set(["Read"]);

export interface ToolObserveInput {
  toolName: string;
  toolInput?: {
    file_path?: string;
    offset?: number;
    limit?: number;
    command?: string;
    pattern?: string;
    [key: string]: unknown;
  };
  success?: boolean;
  sessionId?: string;
}

/**
 * Resolve a native tool name (any casing) to its taxonomy entry. Returns
 * `null` for tools we don't classify — callers skip recording rather than
 * pollute the dashboard with an "other" bucket.
 */
function lookupTool(name: string): ToolTax | null {
  if (NATIVE_TOOL_TAXONOMY[name]) return NATIVE_TOOL_TAXONOMY[name];
  // OpenCode passes lowercased tool names; try a case-insensitive lookup.
  const lower = name.toLowerCase();
  for (const [k, v] of Object.entries(NATIVE_TOOL_TAXONOMY)) {
    if (k.toLowerCase() === lower) return v;
  }
  return null;
}

/**
 * Estimate tokens for the Read tool. If offset/limit are present, scale by
 * the portion actually read. Returns 0 on stat failure (caller still records
 * the event so the call shows up in the timeline).
 */
function estimateReadTokens(
  cwd: string,
  filePath: string,
  offset?: number,
  limit?: number,
): number {
  try {
    const abs = isAbsolute(filePath) ? filePath : resolve(cwd, filePath);
    const stat = statSync(abs);
    const fullTokens = Math.round(stat.size / 3.0);
    if (limit !== undefined && limit > 0 && stat.size > 0) {
      const estTotalLines = Math.max(1, Math.round(stat.size / 35));
      const linesRead = Math.min(limit, estTotalLines - (offset || 0));
      return Math.round(fullTokens * (linesRead / estTotalLines));
    }
    return fullTokens;
  } catch {
    return 0;
  }
}

/**
 * Record a native tool invocation as an observe.KindToolCall event. Pure
 * fire-and-forget: failures are logged but never thrown so this is safe to
 * call from tight hook hot paths. Callers should pass success=true; we still
 * record on success=false so failed invocations are visible in the timeline.
 */
export function recordToolEvent(
  binary: string,
  cwd: string,
  input: ToolObserveInput,
): void {
  const tax = lookupTool(input.toolName);
  if (!tax) {
    debug(SOURCE, `Skipping unclassified tool: ${input.toolName}`);
    return;
  }

  const filePath = input.toolInput?.file_path as string | undefined;
  let tokens = 0;
  if (FILE_SIZED_TOOLS.has(input.toolName) && filePath) {
    tokens = estimateReadTokens(
      cwd,
      filePath,
      input.toolInput?.offset as number | undefined,
      input.toolInput?.limit as number | undefined,
    );
    // Smart-read-hint state: record that this file was read so subsequent
    // re-reads can be flagged as candidates for code_outline/code_symbols.
    // No-op when AIDE_CODE_WATCH is unset.
    recordFileRead(binary, cwd, filePath);
  }

  try {
    const args = [
      "observe",
      "record",
      "--kind=tool_call",
      `--name=${input.toolName}`,
      `--category=${tax.category}`,
      `--subtype=${tax.subtype}`,
    ];
    if (tokens > 0) args.push(`--tokens=${tokens}`);
    if (filePath) args.push(`--file=${filePath}`);
    if (input.sessionId) args.push(`--session=${input.sessionId}`);
    execFileSync(binary, args, {
      cwd,
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    debug(
      SOURCE,
      `Recorded ${input.toolName} ${tax.category}/${tax.subtype} tokens=${tokens}`,
    );
  } catch (err) {
    debug(SOURCE, `Failed to record ${input.toolName}: ${err}`);
  }
}
