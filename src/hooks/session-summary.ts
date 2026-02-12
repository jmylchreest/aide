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

import { debug, setDebugCwd } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import {
  buildSessionSummary,
  getSessionCommits,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import {
  gatherPartials,
  buildSummaryFromPartials,
  cleanupPartials,
} from "../core/partial-memory.js";

const SOURCE = "session-summary";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  transcript_path?: string;
  stop_hook_active?: boolean;
}

/**
 * Generate and store a final session summary.
 *
 * Uses partials (if available) to enrich the transcript-based summary.
 * After storing the final summary, cleans up all partials for this session
 * by tagging them as "forget".
 */
function captureSessionSummary(
  cwd: string,
  sessionId: string,
  transcriptPath: string,
): boolean {
  const binary = findAideBinary({
    cwd,
    pluginRoot: process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
  });
  if (!binary) {
    debug(SOURCE, "aide binary not found, cannot capture session summary");
    return false;
  }

  // Try to build the richest possible summary:
  // 1. Gather partials from this session
  // 2. Build from transcript (Claude Code has this)
  // 3. Merge partials data with transcript summary
  const partials = gatherPartials(binary, cwd, sessionId);
  let summary: string | null = null;

  if (partials.length > 0) {
    // Build from partials + git data
    const commits = getSessionCommits(cwd);
    summary = buildSummaryFromPartials(partials, commits, []);
    debug(SOURCE, `Built summary from ${partials.length} partials`);
  }

  // Fall back to transcript-based summary if partials didn't produce enough
  if (!summary) {
    summary = buildSessionSummary(transcriptPath, cwd);
  }

  if (!summary) {
    debug(SOURCE, "No summary generated (insufficient activity or transcript)");
    // Still clean up partials even if no summary was generated
    if (partials.length > 0) {
      cleanupPartials(binary, cwd, sessionId);
    }
    return false;
  }

  const stored = storeSessionSummary(binary, cwd, sessionId, summary);
  if (stored) {
    debug(SOURCE, `Stored final session summary for ${sessionId.slice(0, 8)}`);
    // Clean up partials now that the final summary is stored
    const cleaned = cleanupPartials(binary, cwd, sessionId);
    debug(SOURCE, `Cleaned up ${cleaned} partials after final summary`);
  } else {
    debug(SOURCE, "Failed to store session summary");
  }
  return stored;
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
