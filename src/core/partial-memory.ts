/**
 * Partial memory logic — platform-agnostic.
 *
 * Writes granular "partial" memories on significant tool events.
 * These are tagged with "partial" so they:
 * - Are excluded from normal recall (via ExcludeTags)
 * - Can be gathered and rolled up into a final session summary
 * - Get cleaned up (tagged "forget") after the final summary is written
 *
 * What counts as "significant":
 * - Write/Edit/MultiEdit to a file (code change)
 * - Bash commands that succeed (shell operations)
 * - Task tool completions (subagent work)
 * - NOT: Read, Grep, Glob (pure reads — no side effects)
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";

const SOURCE = "partial-memory";

/** Tools that represent significant (state-changing) actions */
const SIGNIFICANT_TOOLS = new Set([
  "Write",
  "Edit",
  "MultiEdit",
  "Bash",
  "Task",
]);

/** Information about a completed tool use */
export interface ToolCompletionInfo {
  toolName: string;
  sessionId: string;
  /** File path affected (Write/Edit) */
  filePath?: string;
  /** Bash command executed */
  command?: string;
  /** Task description */
  description?: string;
  /** Whether the tool succeeded */
  success?: boolean;
}

/**
 * Check whether a tool completion is "significant" enough to write a partial.
 */
export function isSignificantToolUse(info: ToolCompletionInfo): boolean {
  if (!SIGNIFICANT_TOOLS.has(info.toolName)) return false;

  // For Bash, only track if we have a command (skip empty/failed)
  if (info.toolName === "Bash") {
    if (!info.command) return false;
    // Skip trivial read-only commands
    const cmd = info.command.trim().toLowerCase();
    if (
      cmd.startsWith("cat ") ||
      cmd.startsWith("ls ") ||
      cmd.startsWith("echo ") ||
      cmd.startsWith("pwd") ||
      cmd.startsWith("which ") ||
      cmd.startsWith("type ")
    ) {
      return false;
    }
  }

  return true;
}

/**
 * Build a concise partial memory content string from tool completion info.
 */
export function buildPartialContent(info: ToolCompletionInfo): string {
  switch (info.toolName) {
    case "Write":
      return info.filePath
        ? `Created file: ${info.filePath}`
        : "Created a file";
    case "Edit":
    case "MultiEdit":
      return info.filePath ? `Edited file: ${info.filePath}` : "Edited a file";
    case "Bash": {
      const cmd =
        info.command && info.command.length > 100
          ? info.command.slice(0, 97) + "..."
          : info.command;
      return `Ran command: ${cmd}`;
    }
    case "Task":
      return info.description
        ? `Completed task: ${info.description}`
        : "Completed a subtask";
    default:
      return `Used tool: ${info.toolName}`;
  }
}

/**
 * Build the tags for a partial memory.
 */
export function buildPartialTags(
  sessionId: string,
  info: ToolCompletionInfo,
): string[] {
  const tags = [
    "partial",
    `session:${sessionId.slice(0, 8)}`,
    `tool:${info.toolName.toLowerCase()}`,
  ];
  if (info.filePath) {
    // Add a tag for the file extension to allow grouping
    const ext = info.filePath.split(".").pop();
    if (ext) tags.push(`ext:${ext}`);
  }
  return tags;
}

/**
 * Store a partial memory for a significant tool event.
 *
 * Returns true if stored successfully.
 */
export function storePartialMemory(
  binary: string,
  cwd: string,
  info: ToolCompletionInfo,
): boolean {
  if (!isSignificantToolUse(info)) return false;

  try {
    const content = buildPartialContent(info);
    const tags = buildPartialTags(info.sessionId, info);

    execFileSync(
      binary,
      [
        "memory",
        "add",
        "--category=session",
        `--tags=${tags.join(",")}`,
        content,
      ],
      { cwd, stdio: "pipe", timeout: 3000 },
    );

    debug(SOURCE, `Stored partial: ${content} [${tags.join(", ")}]`);
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to store partial (non-fatal): ${err}`);
    return false;
  }
}

/**
 * Gather all partial memories for a session.
 *
 * Uses `aide memory list` with tag filtering to find all partials.
 * Returns the raw output or null if none found.
 */
export function gatherPartials(
  binary: string,
  cwd: string,
  sessionId: string,
): string[] {
  try {
    const sessionTag = `session:${sessionId.slice(0, 8)}`;

    const output = execFileSync(
      binary,
      [
        "memory",
        "list",
        "--tags=partial",
        "--all", // Include even if tagged forget (shouldn't be, but defensive)
        "--format=json",
        "--limit=500",
      ],
      {
        cwd,
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 5000,
      },
    ).trim();

    if (!output || output === "[]") return [];

    interface PartialMemory {
      id: string;
      tags: string[];
      content: string;
    }

    const memories: PartialMemory[] = JSON.parse(output);
    // Filter to this session's partials
    return memories
      .filter((m) => m.tags?.includes(sessionTag))
      .map((m) => m.content);
  } catch (err) {
    debug(SOURCE, `Failed to gather partials: ${err}`);
    return [];
  }
}

/**
 * Gather partial memory IDs for a session (for cleanup).
 */
export function gatherPartialIds(
  binary: string,
  cwd: string,
  sessionId: string,
): string[] {
  try {
    const sessionTag = `session:${sessionId.slice(0, 8)}`;

    const output = execFileSync(
      binary,
      [
        "memory",
        "list",
        "--tags=partial",
        "--all",
        "--format=json",
        "--limit=500",
      ],
      {
        cwd,
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 5000,
      },
    ).trim();

    if (!output || output === "[]") return [];

    interface PartialMemory {
      id: string;
      tags: string[];
    }

    const memories: PartialMemory[] = JSON.parse(output);
    return memories
      .filter((m) => m.tags?.includes(sessionTag))
      .map((m) => m.id);
  } catch (err) {
    debug(SOURCE, `Failed to gather partial IDs: ${err}`);
    return [];
  }
}

/**
 * Clean up partials for a session by tagging them as "forget".
 *
 * This soft-deletes them so they won't appear in future queries
 * but remain recoverable if needed.
 */
export function cleanupPartials(
  binary: string,
  cwd: string,
  sessionId: string,
): number {
  const ids = gatherPartialIds(binary, cwd, sessionId);
  let cleaned = 0;

  for (const id of ids) {
    try {
      execFileSync(binary, ["memory", "tag", id, "--add=forget"], {
        cwd,
        stdio: "pipe",
        timeout: 3000,
      });
      cleaned++;
    } catch (err) {
      debug(SOURCE, `Failed to cleanup partial ${id}: ${err}`);
    }
  }

  if (cleaned > 0) {
    debug(
      SOURCE,
      `Cleaned up ${cleaned} partials for session ${sessionId.slice(0, 8)}`,
    );
  }
  return cleaned;
}

/**
 * Build a final session summary that incorporates partials.
 *
 * If partials are available, they're included alongside git data
 * to produce a richer summary than either source alone.
 */
export function buildSummaryFromPartials(
  partials: string[],
  gitCommits: string[],
  gitFiles: string[],
): string | null {
  const summaryParts: string[] = [];

  // Deduplicate and categorise partials
  const fileChanges = new Set<string>();
  const commands: string[] = [];
  const tasks: string[] = [];
  const other: string[] = [];

  for (const p of partials) {
    if (p.startsWith("Created file: ") || p.startsWith("Edited file: ")) {
      fileChanges.add(p.replace(/^(Created|Edited) file: /, ""));
    } else if (p.startsWith("Ran command: ")) {
      commands.push(p.replace("Ran command: ", ""));
    } else if (p.startsWith("Completed task: ")) {
      tasks.push(p.replace("Completed task: ", ""));
    } else {
      other.push(p);
    }
  }

  if (tasks.length > 0) {
    summaryParts.push(
      `## Tasks\n${tasks
        .slice(0, 5)
        .map((t) => `- ${t}`)
        .join("\n")}`,
    );
  }

  if (gitCommits.length > 0) {
    summaryParts.push(
      `## Commits\n${gitCommits.map((c) => `- ${c}`).join("\n")}`,
    );
  }

  // Merge file changes from partials and git
  const allFiles = new Set([...fileChanges, ...gitFiles]);
  if (allFiles.size > 0) {
    const files = Array.from(allFiles).slice(0, 15);
    summaryParts.push(
      `## Files Modified\n${files.map((f) => `- ${f}`).join("\n")}`,
    );
  }

  if (commands.length > 0) {
    summaryParts.push(
      `## Commands\n${commands
        .slice(0, 10)
        .map((c) => `- ${c}`)
        .join("\n")}`,
    );
  }

  const summary = summaryParts.join("\n\n");
  return summary.length >= 50 ? summary : null;
}
