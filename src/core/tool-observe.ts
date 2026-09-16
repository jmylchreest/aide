/**
 * Tool observability — single source of truth for native tool → observe.Event
 * mapping. Used by both the Claude Code PostToolUse hook and the OpenCode
 * tool.execute.after handler so dashboard categorisation stays consistent
 * across plugins.
 *
 * Native tools and recognised aide MCP calls are observed here at the
 * host boundary. MCP middleware independently observes server results; those
 * stages must never be added together as if they were separate deliveries.
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";
import { recordFileRead } from "./read-tracking.js";
import { contextWindow } from "./context-window.js";
import { aideRetrievalTool, retrievalEvidence } from "./retrieval-evidence.js";
import { aideWorkTool, workReceiptEvidence } from "./work-receipt.js";

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
  exec_command: "Bash",
  write_stdin: "Bash",
  fetch: "WebFetch",
  search_web: "WebSearch",
};

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

/** Extract supported textual components, not serialized metadata or opaque media.
 * Undefined means unmeasured; an empty string is a known empty text result.
 * Alternate wrappers are preferred in order rather than counted twice.
 */
export function extractOutputText(payload: unknown): string | undefined {
  if (typeof payload === "string") return payload;
  if (Array.isArray(payload)) {
    if (payload.length === 0) return "";
    const parts = payload
      .map(extractOutputText)
      .filter((v): v is string => v !== undefined);
    return parts.length ? parts.join("") : undefined;
  }
  if (!payload || typeof payload !== "object") return undefined;
  const obj = payload as Record<string, unknown>;
  if (typeof obj.type === "string" && !["text", "resource"].includes(obj.type))
    return undefined;
  const output = extractOutputText(obj.output);
  if (output !== undefined) return output;
  if (typeof obj.stdout === "string" || typeof obj.stderr === "string") {
    return (
      (typeof obj.stdout === "string" ? obj.stdout : "") +
      (typeof obj.stderr === "string" ? obj.stderr : "")
    );
  }
  for (const key of [
    "content",
    "text",
    "result",
    "file",
    "resource",
    "error",
  ]) {
    const text = extractOutputText(obj[key]);
    if (text !== undefined) return text;
  }
  return undefined;
}

/**
 * Decide whether a tool call failed and, if so, return the error text to
 * attach to the observe event (drives the friction detector). Returns "" when
 * the call succeeded.
 *
 * The meaningful friction signal is a *tool-level* failure — an Edit whose
 * target string wasn't found, a Read of a missing file, a command the shell
 * couldn't run — not every non-zero shell exit (a failing test mid-TDD is
 * expected, not friction). We treat as failed: an explicit success===false,
 * or a tool_response carrying an is_error / error / failed marker (how Claude
 * Code flags tool-level errors). When failed but no text is recoverable, a
 * generic marker still lets the detector count the recurrence.
 */
export function toolFailureText(
  success: boolean | undefined,
  toolResponse: unknown,
): string {
  let failed = success === false;
  let errorField = "";
  if (toolResponse && typeof toolResponse === "object") {
    const r = toolResponse as Record<string, unknown>;
    if (r.is_error === true || r.isError === true) failed = true;
    if (r.status === "error" || r.status === "failed") failed = true;
    if (typeof r.error === "string" && r.error.length > 0) {
      failed = true;
      errorField = r.error;
    }
  }
  if (!failed) return "";
  // Prefer an explicit error field, then any rendered output, then a marker so
  // the detector can still count the recurrence even with no text.
  const text = errorField || extractOutputText(toolResponse);
  return (text || "tool reported failure").slice(0, 500);
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
  /** Some hooks put the command exit status beside, not inside, tool_response. */
  exitCode?: unknown;
  /**
   * Explicit error text from a harness failure event (Claude Code's
   * PostToolUseFailure, OpenCode tool errors). When set, it's used verbatim as
   * the event's error rather than inferring failure from toolResponse — the
   * harness already told us it failed. Empty/undefined means "use the
   * heuristic in toolFailureText".
   */
  errorText?: string;
  sessionId?: string;
  host?: string;
  invocationId?: string;
  actorId?: string;
}

/**
 * Resolve a native tool name (any casing) to its taxonomy entry. Returns
 * `null` for tools we don't classify — callers skip recording rather than
 * pollute the dashboard with an "other" bucket.
 *
 * Lookup order: exact → case-insensitive → cross-harness alias.
 */
function canonicalTool(name: string): string | undefined {
  return (
    Object.keys(NATIVE_TOOL_TAXONOMY).find(
      (k) => k.toLowerCase() === name.toLowerCase(),
    ) ?? TOOL_ALIASES[name.toLowerCase()]
  );
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
  const retrieval = aideRetrievalTool(input.toolName);
  const aideTool = aideWorkTool(input.toolName);
  const name = canonicalTool(input.toolName) ?? aideTool;
  const tax = retrieval
    ? {
        category: "consume",
        subtype: retrieval === "code_outline" ? "outline" : "symbol",
      }
    : aideTool
      ? { category: "other", subtype: "mcp" }
      : name
        ? NATIVE_TOOL_TAXONOMY[name]
        : undefined;
  if (!tax || !name) {
    debug(SOURCE, `Skipping unclassified tool: ${input.toolName}`);
    return;
  }

  const toolInput = input.toolInput ?? {};
  const path =
    toolInput.file_path ??
    toolInput.filePath ??
    (retrieval ? toolInput.file : undefined);
  const filePath = typeof path === "string" ? path : undefined;
  const errText =
    (input.errorText && input.errorText.slice(0, 500)) ||
    toolFailureText(input.success, input.toolResponse);
  const text = extractOutputText(input.toolResponse);
  const retrievalAttrs = retrievalEvidence(
    cwd,
    name,
    toolInput,
    text,
    input.toolResponse,
    !!errText,
    input.exitCode,
    // Codex hook input can omit the actual exec working directory. Relative
    // shell reads need that evidence before attributing them to project files.
    { requireShellWorkdir: input.host === "codex" },
  );
  const identity = {
    host: input.host,
    sessionId: input.sessionId,
    actorId: input.actorId || input.sessionId,
  };
  const window = contextWindow(binary, cwd, identity);
  const generated = toolInput[CONTENT_WRITE_TOOLS[name]];
  let startLine: number | undefined;
  let endLine: number | undefined;
  if (
    name === "Read" &&
    filePath &&
    !errText &&
    text !== undefined &&
    retrievalAttrs.retrieval_status !== "failed"
  ) {
    const offset = toolInput.offset;
    const limit = toolInput.limit;
    startLine = typeof offset === "number" && offset > 0 ? offset : 1;
    if (typeof limit === "number" && limit > 0) endLine = startLine + limit - 1;
    if (retrievalAttrs.retrieval_status === "full_file") {
      // Only the locally verified full rendering may supply this fingerprint;
      // neither MCP receipts nor partial/native metadata establish full reads.
      const verifiedRenderedHash =
        retrievalAttrs.source_verification === "current_rendered_file_match"
          ? (
              JSON.parse(retrievalAttrs.source_references) as {
                sha256: string;
              }[]
            )[0].sha256
          : undefined;
      recordFileRead(binary, cwd, filePath, {
        identity,
        content: text,
        ...(verifiedRenderedHash ? { verifiedRenderedHash } : {}),
      });
    }
  }

  try {
    const args = [
      "observe",
      "record",
      "--kind=tool_call",
      `--name=${name}`,
      `--category=${tax.category}`,
      `--subtype=${tax.subtype}`,
    ];
    args.push(
      "--attr=accounting_version=1",
      "--attr=observation_stage=host_result",
      `--attr=raw_tool=${input.toolName}`,
    );
    for (const [key, value] of Object.entries(retrievalAttrs))
      args.push(`--attr=${key}=${value}`);
    for (const [key, value] of Object.entries(
      workReceiptEvidence(aideTool, text, input.toolResponse),
    ))
      args.push(`--attr=${key}=${value}`);
    // Conversion is owned by the backend. These are exact UTF-8 text bytes at
    // this hook boundary, not provider tokens or proof of final delivery.
    if (text !== undefined)
      args.push(`--attr=payload_bytes=${Buffer.byteLength(text, "utf8")}`);
    if (typeof generated === "string")
      args.push(
        `--attr=argument_bytes=${Buffer.byteLength(generated, "utf8")}`,
      );
    if (input.host) args.push(`--attr=host=${input.host}`);
    args.push(`--attr=context_status=${window?.status ?? "unknown"}`);
    if (window)
      args.push(
        `--attr=context_epoch=${window.id}`,
        `--attr=context_continuity=${window.continuity}`,
      );
    if (input.invocationId)
      args.push(`--attr=invocation_id=${input.invocationId}`);
    if (input.actorId || input.sessionId)
      args.push(`--attr=actor_id=${input.actorId || input.sessionId}`);
    if (filePath) args.push(`--file=${filePath}`);
    if (input.sessionId) args.push(`--session=${input.sessionId}`);
    if (startLine !== undefined) args.push(`--attr=start_line=${startLine}`);
    if (endLine !== undefined) args.push(`--attr=end_line=${endLine}`);
    // Capture the command (Bash) or pattern (Grep) text so the repetition
    // detector can group calls by canonical signature instead of lumping
    // every Bash invocation under a single "Bash" bucket. Truncated to keep
    // the attr cheap; the normaliser only uses the first token anyway.
    const cmd = toolInput.command ?? toolInput.cmd;
    if (typeof cmd === "string" && cmd.length > 0) {
      args.push(`--attr=command=${cmd.slice(0, 500)}`);
    }
    const pattern = input.toolInput?.pattern;
    if (typeof pattern === "string" && pattern.length > 0) {
      args.push(`--attr=pattern=${pattern.slice(0, 200)}`);
    }
    // Mark tool-level failures so the friction detector can spot a recurring
    // obstacle (the same tool failing on the same target). An explicit
    // errorText from a harness failure event wins; otherwise we infer from the
    // response. Empty when the call succeeded, so successes record as before.
    if (errText) {
      args.push(`--error=${errText}`);
    }
    execFileSync(binary, args, {
      cwd,
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    debug(
      SOURCE,
      `Recorded ${input.toolName} ${tax.category}/${tax.subtype} textBytes=${text === undefined ? "unknown" : Buffer.byteLength(text, "utf8")}`,
    );
  } catch (err) {
    debug(SOURCE, `Failed to record ${input.toolName}: ${err}`);
  }
}
