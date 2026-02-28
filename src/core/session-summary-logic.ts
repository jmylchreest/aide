/**
 * Session summary logic — platform-agnostic.
 *
 * Extracted from src/hooks/session-summary.ts.
 * Parses transcripts and creates session summaries stored as memories.
 */

import { execFileSync } from "child_process";
import { readFileSync, existsSync } from "fs";
import { join } from "path";

/**
 * Get git commits made during this session
 */
export function getSessionCommits(cwd: string): string[] {
  try {
    const sessionPath = join(cwd, ".aide", "state", "session.json");
    if (!existsSync(sessionPath)) return [];

    const sessionData = JSON.parse(readFileSync(sessionPath, "utf-8"));
    const startedAt = sessionData.startedAt;
    if (!startedAt) return [];

    const output = execFileSync(
      "git",
      ["log", "--oneline", `--since=${startedAt}`],
      {
        cwd,
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 5000,
      },
    ).trim();

    if (!output) return [];
    return output
      .split("\n")
      .filter((l) => l.trim())
      .slice(0, 10);
  } catch {
    return [];
  }
}

/**
 * Build a session summary from transcript data.
 *
 * @param transcriptPath - Path to JSONL transcript file (Claude Code specific)
 * @param cwd - Working directory
 * @returns Summary text or null if not enough activity
 */
export function buildSessionSummary(
  transcriptPath: string,
  cwd: string,
): string | null {
  if (!existsSync(transcriptPath)) return null;

  try {
    const transcript = readFileSync(transcriptPath, "utf-8");
    const lines = transcript.split("\n").filter((l) => l.trim());

    if (lines.length < 5) return null;

    interface TranscriptEntry {
      type?: string;
      tool_name?: string;
      tool_input?: { file_path?: string; [key: string]: unknown };
      content?:
        | string
        | {
            text?: string;
            tool_use?: { name?: string; input?: { file_path?: string } };
          };
    }

    const entries: TranscriptEntry[] = [];
    for (const line of lines) {
      try {
        entries.push(JSON.parse(line) as TranscriptEntry);
      } catch {
        // Skip malformed
      }
    }

    const filesModified = new Set<string>();
    const toolsUsed = new Set<string>();
    const userMessages: string[] = [];

    for (const entry of entries) {
      const contentObj =
        typeof entry.content === "object" ? entry.content : null;
      if (
        entry.type === "tool_use" ||
        (entry.type === "assistant" && contentObj?.tool_use)
      ) {
        const toolName = entry.tool_name || contentObj?.tool_use?.name;
        if (toolName) toolsUsed.add(toolName);

        const toolInput = entry.tool_input || contentObj?.tool_use?.input;
        if (toolInput?.file_path && toolName) {
          if (["Write", "Edit"].includes(toolName)) {
            filesModified.add(toolInput.file_path);
          }
        }
      }

      if (entry.type === "human" || entry.type === "user") {
        const text =
          typeof entry.content === "string" ? entry.content : contentObj?.text;
        if (text && text.length > 10 && text.length < 500) {
          userMessages.push(text.slice(0, 200));
        }
      }
    }

    const commits = getSessionCommits(cwd);

    if (
      filesModified.size === 0 &&
      toolsUsed.size < 3 &&
      commits.length === 0
    ) {
      return null;
    }

    const summaryParts: string[] = [];

    if (userMessages.length > 0) {
      summaryParts.push(
        `## Tasks\n${userMessages
          .slice(0, 3)
          .map((m) => `- ${m}`)
          .join("\n")}`,
      );
    }

    if (filesModified.size > 0) {
      const files = Array.from(filesModified).slice(0, 10);
      summaryParts.push(
        `## Files Modified\n${files.map((f) => `- ${f}`).join("\n")}`,
      );
    }

    if (commits.length > 0) {
      summaryParts.push(
        `## Commits\n${commits.map((c) => `- ${c}`).join("\n")}`,
      );
    }

    if (toolsUsed.size > 0) {
      summaryParts.push(`## Tools Used\n${Array.from(toolsUsed).join(", ")}`);
    }

    const summary = summaryParts.join("\n\n");
    return summary.length >= 50 ? summary : null;
  } catch {
    return null;
  }
}

/**
 * Build a session summary from tracked state (for platforms without transcript access).
 *
 * Uses aide state and git history instead of transcript parsing.
 */
export function buildSessionSummaryFromState(cwd: string): string | null {
  const commits = getSessionCommits(cwd);

  const summaryParts: string[] = [];

  if (commits.length > 0) {
    summaryParts.push(`## Commits\n${commits.map((c) => `- ${c}`).join("\n")}`);
  }

  // Check for modified files via git
  try {
    const diff = execFileSync(
      "git",
      ["diff", "--name-only", "HEAD~5", "HEAD"],
      {
        cwd,
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 5000,
      },
    ).trim();

    if (diff) {
      const files = diff
        .split("\n")
        .filter((f) => f.trim())
        .slice(0, 10);
      if (files.length > 0) {
        summaryParts.push(
          `## Files Modified\n${files.map((f) => `- ${f}`).join("\n")}`,
        );
      }
    }
  } catch {
    // Ignore
  }

  const summary = summaryParts.join("\n\n");
  return summary.length >= 50 ? summary : null;
}

/**
 * Store a session summary as a memory
 */
export function storeSessionSummary(
  binary: string,
  cwd: string,
  sessionId: string,
  summary: string,
): boolean {
  try {
    const tags = `session-summary,session:${sessionId.slice(0, 12)}`;

    execFileSync(
      binary,
      ["memory", "add", "--category=session", `--tags=${tags}`, summary],
      { cwd, stdio: "pipe", timeout: 5000 },
    );

    return true;
  } catch {
    return false;
  }
}
