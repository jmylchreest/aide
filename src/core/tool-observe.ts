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

/**
 * Cross-harness aliases. Codex and other harnesses name the same primitives
 * differently — `update`/`apply_patch` for Edit, `view` for Read, `shell`
 * for Bash, etc. Mapping them to Claude Code's canonical names keeps the
 * dashboard's per-tool aggregation coherent across plugins (no separate
 * "update" + "Edit" buckets that mean the same thing).
 *
 * Lookup is case-insensitive; aliases here are the lowercase form.
 */
const TOOL_ALIASES: Record<string, string> = {
  // Codex / OpenAI-style tool names
  update: "Edit",
  apply_patch: "Edit",
  str_replace_editor: "Edit",
  view: "Read",
  read_file: "Read",
  get: "Read",
  create: "Write",
  shell: "Bash",
  exec: "Bash",
  fetch: "WebFetch",
  search_web: "WebSearch",
};

/** Tools whose tokens we estimate from on-disk file size (the Read path). */
const FILE_SIZED_TOOLS = new Set(["Read"]);

/**
 * Tools whose token cost is the size of content the agent *writes* — the
 * `new_string` for Edit, the `content` for Write. We track these so the
 * "modify" category in per-tool efficiency surfaces something other than
 * a flat zero.
 */
const CONTENT_WRITE_TOOLS: Record<string, string> = {
  Edit: "new_string",
  Write: "content",
  NotebookEdit: "new_source",
};

/**
 * Tools whose cost is the size of the *output* they produce — Bash stdout,
 * WebFetch page body, WebSearch results, Grep match lines. The PostToolUse
 * payload carries the tool's response so we can estimate the tokens that
 * flowed back into the agent's context.
 */
const OUTPUT_SIZED_TOOLS = new Set(["Bash", "WebFetch", "WebSearch", "Grep"]);

/**
 * Pull the textual output from a tool_response / tool_result payload. The
 * shape varies by tool and by harness (Claude Code passes string for Bash,
 * objects for others; OpenCode wraps things differently), so we try the
 * common keys defensively and return "" when there's no text to count.
 */
function extractOutputText(payload: unknown): string {
  if (!payload) return "";
  if (typeof payload === "string") return payload;
  if (typeof payload === "object") {
    const obj = payload as Record<string, unknown>;
    for (const key of ["output", "stdout", "content", "text", "result"]) {
      const v = obj[key];
      if (typeof v === "string") return v;
    }
  }
  return "";
}

export interface ToolObserveInput {
  toolName: string;
  toolInput?: {
    file_path?: string;
    offset?: number;
    limit?: number;
    command?: string;
    pattern?: string;
    new_string?: string;
    content?: string;
    new_source?: string;
    [key: string]: unknown;
  };
  /**
   * The tool's response payload, used to estimate output token cost for
   * Bash/WebFetch/WebSearch/Grep. Shape varies per tool and per harness;
   * extractOutputText handles the common cases.
   */
  toolResponse?: unknown;
  success?: boolean;
  sessionId?: string;
}

/**
 * Resolve a native tool name (any casing) to its taxonomy entry. Returns
 * `null` for tools we don't classify — callers skip recording rather than
 * pollute the dashboard with an "other" bucket.
 *
 * Lookup order: exact → case-insensitive → cross-harness alias.
 */
function lookupTool(name: string): ToolTax | null {
  if (NATIVE_TOOL_TAXONOMY[name]) return NATIVE_TOOL_TAXONOMY[name];
  const lower = name.toLowerCase();
  for (const [k, v] of Object.entries(NATIVE_TOOL_TAXONOMY)) {
    if (k.toLowerCase() === lower) return v;
  }
  const canonical = TOOL_ALIASES[lower];
  if (canonical && NATIVE_TOOL_TAXONOMY[canonical]) {
    return NATIVE_TOOL_TAXONOMY[canonical];
  }
  return null;
}

/**
 * Resolve to the canonical tool name (Edit/Read/Write/...) so observe
 * events from different harnesses aggregate into the same bucket on the
 * dashboard. Falls back to the original name when no alias matches.
 */
function canonicalToolName(name: string): string {
  if (NATIVE_TOOL_TAXONOMY[name]) return name;
  const lower = name.toLowerCase();
  for (const k of Object.keys(NATIVE_TOOL_TAXONOMY)) {
    if (k.toLowerCase() === lower) return k;
  }
  return TOOL_ALIASES[lower] ?? name;
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
  let startLine: number | undefined;
  let endLine: number | undefined;
  if (FILE_SIZED_TOOLS.has(input.toolName) && filePath) {
    const offset = input.toolInput?.offset as number | undefined;
    const limit = input.toolInput?.limit as number | undefined;
    tokens = estimateReadTokens(cwd, filePath, offset, limit);
    // Read tool offset/limit are line-based (1-based when present, default
    // 1..end). Persist the range so the dashboard's file viewer can
    // scroll/highlight the slice the agent actually consumed.
    startLine = offset && offset > 0 ? offset : 1;
    if (limit && limit > 0) {
      endLine = startLine + limit - 1;
    }
    // Smart-read-hint state: record that this file was read so subsequent
    // re-reads can be flagged as candidates for code_outline/code_symbols.
    // No-op when AIDE_CODE_WATCH is unset.
    recordFileRead(binary, cwd, filePath);
  } else if (CONTENT_WRITE_TOOLS[input.toolName]) {
    // Modify tools: the cost is the new content the agent generates,
    // not the existing file. Same chars/3 estimator the Read path uses.
    const field = CONTENT_WRITE_TOOLS[input.toolName];
    const content = input.toolInput?.[field];
    if (typeof content === "string" && content.length > 0) {
      tokens = Math.round(content.length / 3.0);
    }
  } else if (OUTPUT_SIZED_TOOLS.has(input.toolName)) {
    // Output-sized tools: cost = how much text came back into context.
    // Stays 0 when the harness didn't pass a tool_response (some hooks
    // strip it for size). That's still useful — we get the call count.
    const text = extractOutputText(input.toolResponse);
    if (text.length > 0) {
      tokens = Math.round(text.length / 3.0);
    }
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
    if (startLine !== undefined) args.push(`--attr=start_line=${startLine}`);
    if (endLine !== undefined) args.push(`--attr=end_line=${endLine}`);
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
