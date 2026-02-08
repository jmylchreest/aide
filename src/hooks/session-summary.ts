#!/usr/bin/env node
/**
 * Session Summary Hook (Stop)
 *
 * Captures a session summary from the transcript when a session ends.
 * Includes files modified, tools used, user tasks, and git commits.
 *
 * Storage:
 * - Uses `aide memory add` with category=session
 */

import { execFileSync } from "child_process";
import { readFileSync, existsSync } from "fs";
import { join } from "path";
import { debug, setDebugCwd } from "../lib/logger.js";
import { readStdin, findAideBinary } from "../lib/hook-utils.js";

const SOURCE = "session-summary";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  transcript_path?: string;
  stop_hook_active?: boolean;
}

/**
 * Generate and store a session summary from transcript
 */
function captureSessionSummary(
  cwd: string,
  sessionId: string,
  transcriptPath: string,
): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) {
    debug(SOURCE, "aide binary not found, cannot capture session summary");
    return false;
  }

  if (!existsSync(transcriptPath)) {
    debug(SOURCE, `Transcript not found: ${transcriptPath}`);
    return false;
  }

  try {
    // Read transcript
    const transcript = readFileSync(transcriptPath, "utf-8");
    const lines = transcript.split("\n").filter((l) => l.trim());

    if (lines.length < 5) {
      debug(SOURCE, "Transcript too short for summary");
      return false;
    }

    // Parse transcript entries
    // Transcript format is JSONL with various entry types
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
        const entry = JSON.parse(line) as TranscriptEntry;
        entries.push(entry);
      } catch {
        // Skip malformed lines
      }
    }

    // Extract key information
    const filesModified = new Set<string>();
    const toolsUsed = new Set<string>();
    const userMessages: string[] = [];

    for (const entry of entries) {
      // Track tool usage
      const contentObj =
        typeof entry.content === "object" ? entry.content : null;
      if (
        entry.type === "tool_use" ||
        (entry.type === "assistant" && contentObj?.tool_use)
      ) {
        const toolName = entry.tool_name || contentObj?.tool_use?.name;
        if (toolName) toolsUsed.add(toolName);

        // Track file modifications
        const toolInput = entry.tool_input || contentObj?.tool_use?.input;
        if (toolInput?.file_path && toolName) {
          if (["Write", "Edit"].includes(toolName)) {
            filesModified.add(toolInput.file_path);
          }
        }
      }

      // Track user messages
      if (entry.type === "human" || entry.type === "user") {
        const text =
          typeof entry.content === "string" ? entry.content : contentObj?.text;
        if (text && text.length > 10 && text.length < 500) {
          userMessages.push(text.slice(0, 200));
        }
      }
    }

    // Get git commits made during this session
    const commits = getSessionCommits(cwd);

    // Only create summary if there was meaningful activity
    if (filesModified.size === 0 && toolsUsed.size < 3 && commits.length === 0) {
      debug(SOURCE, "Not enough activity for session summary");
      return false;
    }

    // Build summary
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

    if (summary.length < 50) {
      debug(SOURCE, "Summary too short, skipping");
      return false;
    }

    // Store as memory
    const dbPath = join(cwd, ".aide", "memory", "store.db");
    const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
    const tags = `session-summary,session:${sessionId.slice(0, 8)}`;

    execFileSync(
      binary,
      ["memory", "add", "--category=session", `--tags=${tags}`, summary],
      {
        env,
        stdio: "pipe",
        timeout: 5000,
      },
    );

    debug(SOURCE, `Stored session summary for ${sessionId.slice(0, 8)}`);
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to capture session summary: ${err}`);
    return false;
  }
}

/**
 * Get git commits made during this session
 */
function getSessionCommits(cwd: string): string[] {
  try {
    const sessionPath = join(cwd, ".aide", "state", "session.json");
    if (!existsSync(sessionPath)) {
      debug(SOURCE, "No session.json found, skipping commit capture");
      return [];
    }

    const sessionData = JSON.parse(readFileSync(sessionPath, "utf-8"));
    const startedAt = sessionData.startedAt;
    if (!startedAt) {
      debug(SOURCE, "No startedAt in session.json");
      return [];
    }

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

    const commits = output.split("\n").filter((l) => l.trim());
    return commits.slice(0, 10);
  } catch (err) {
    debug(SOURCE, `Failed to get session commits: ${err}`);
    return [];
  }
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    setDebugCwd(cwd);
    debug(SOURCE, `Hook triggered: ${data.hook_event_name}`);

    // For Stop hook, capture session summary
    if (data.hook_event_name === "Stop" && data.transcript_path) {
      // Don't capture if stop hook is already active (avoid recursion)
      if (!data.stop_hook_active) {
        debug(SOURCE, "Stop hook - capturing session summary");
        captureSessionSummary(cwd, sessionId, data.transcript_path);
      }
    }

    console.log(JSON.stringify({ continue: true }));
  } catch (err) {
    debug(SOURCE, `Error: ${err}`);
    console.log(JSON.stringify({ continue: true }));
  }
}


process.on("uncaughtException", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});
process.on("unhandledRejection", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});

main();
