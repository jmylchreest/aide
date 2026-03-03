#!/usr/bin/env node
/**
 * Context Pruning Hook (PostToolUse)
 *
 * Reduces context usage by deduplicating repeated tool outputs,
 * annotating superseded reads, and purging large error outputs.
 *
 * For MCP tools: uses `updatedMCPToolOutput` to replace the output.
 * For built-in tools: uses `additionalContext` to add dedup notes.
 *
 * Tracker state is persisted to a temp file per session so it survives
 * across separate hook process invocations.
 *
 * Core logic is in src/core/context-pruning/ for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { ContextPruningTracker } from "../core/context-pruning/index.js";
import type { ToolRecord } from "../core/context-pruning/types.js";
import { tmpdir } from "os";
import { join } from "path";
import { existsSync, readFileSync, writeFileSync } from "fs";

const SOURCE = "context-pruning";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  tool_output?: string;
  mcp_server_name?: string;
  transcript_path?: string;
}

interface HookOutput {
  continue: boolean;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
    updatedMCPToolOutput?: string;
  };
}

/** Path to the persisted tracker history for a session. */
function historyPath(sessionId: string): string {
  return join(tmpdir(), `aide-context-pruning-${sessionId}.json`);
}

/** Load tracker history from disk. */
function loadHistory(sessionId: string): {
  history: ToolRecord[];
  hasExplainedPruning: boolean;
} {
  const path = historyPath(sessionId);
  try {
    if (existsSync(path)) {
      const data = JSON.parse(readFileSync(path, "utf-8"));
      return {
        history: data.history || [],
        hasExplainedPruning: data.hasExplainedPruning || false,
      };
    }
  } catch {
    // Corrupt file — start fresh
  }
  return { history: [], hasExplainedPruning: false };
}

/** Save tracker history to disk. */
function saveHistory(
  sessionId: string,
  history: ToolRecord[],
  hasExplainedPruning: boolean,
): void {
  const path = historyPath(sessionId);
  try {
    // Keep only last 200 entries to prevent unbounded growth
    const trimmed = history.length > 200 ? history.slice(-200) : history;
    writeFileSync(
      path,
      JSON.stringify({ history: trimmed, hasExplainedPruning }),
      "utf-8",
    );
  } catch (err) {
    debug(SOURCE, `Failed to save history: ${err}`);
  }
}

/** Check if a tool is an MCP tool (aide or other MCP server). */
function isMCPTool(toolName: string, mcpServerName?: string): boolean {
  if (mcpServerName) return true;
  // Convention: MCP tools often have mcp__ prefix or are aide tools
  if (toolName.startsWith("mcp__")) return true;
  return false;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const toolOutput = data.tool_output || "";
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Skip if no tool output to prune
    if (!toolOutput || toolOutput.length < 50) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    // Create tracker with loaded history
    const tracker = new ContextPruningTracker(cwd);
    const { history: priorHistory, hasExplainedPruning } =
      loadHistory(sessionId);
    tracker.loadHistory(priorHistory);

    // Use a synthetic callId since CC doesn't provide one
    const callId = `cc-${sessionId}-${Date.now()}`;

    // Process through pruning strategies
    const result = tracker.process(callId, toolName, toolInput, toolOutput);

    // Track whether we've explained pruning tags to the model
    let explained = hasExplainedPruning;

    // Save updated history
    saveHistory(sessionId, tracker.getHistory(), explained);

    if (result.modified) {
      debug(
        SOURCE,
        `Pruned [${result.strategy}]: saved ${result.bytesSaved} bytes for ${toolName}`,
      );

      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PostToolUse",
        },
      };

      if (isMCPTool(toolName, data.mcp_server_name)) {
        // For MCP tools, replace the output entirely
        output.hookSpecificOutput!.updatedMCPToolOutput = result.output;
      } else {
        // For built-in tools, add context note about the dedup
        output.hookSpecificOutput!.additionalContext = result.output.includes(
          "[aide:dedup]",
        )
          ? `Note: This tool output is identical to a previous call. The full content was already provided earlier.`
          : result.output.includes("[aide:purge]")
            ? `Note: Error output was truncated. Re-run the command to see full output.`
            : undefined;
      }

      // On first prune, inject explanation of pruning tags via additionalContext
      if (!explained) {
        const pruningNotes = [
          "<aide-context-pruning>",
          "Tool outputs may contain these tags from aide's context optimization:",
          "- [aide:dedup] — This output is identical to a previous call. Refer to the earlier result.",
          "- [aide:supersede] — A prior Read of this file is now stale after a Write/Edit.",
          "- [aide:purge] — Large error output was trimmed. Re-run the command for full output.",
          "</aide-context-pruning>",
        ].join("\n");

        const existing = output.hookSpecificOutput!.additionalContext || "";
        output.hookSpecificOutput!.additionalContext = existing
          ? `${existing}\n\n${pruningNotes}`
          : pruningNotes;

        explained = true;
        // Persist the flag
        saveHistory(sessionId, tracker.getHistory(), explained);
      }

      console.log(JSON.stringify(output));
    } else {
      console.log(JSON.stringify({ continue: true }));
    }
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    console.log(JSON.stringify({ continue: true }));
  }
}

process.on("uncaughtException", (err) => {
  debug(SOURCE, `UNCAUGHT EXCEPTION: ${err}`);
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    console.log('{"continue":true}');
  }
  process.exit(0);
});
process.on("unhandledRejection", (reason) => {
  debug(SOURCE, `UNHANDLED REJECTION: ${reason}`);
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    console.log('{"continue":true}');
  }
  process.exit(0);
});

main();
