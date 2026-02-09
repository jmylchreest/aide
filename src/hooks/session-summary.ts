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
  storeSessionSummary,
} from "../core/session-summary-logic.js";

const SOURCE = "session-summary";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  transcript_path?: string;
  stop_hook_active?: boolean;
}

/**
 * Generate and store a session summary from transcript — delegates to core
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

  const summary = buildSessionSummary(transcriptPath, cwd);
  if (!summary) {
    debug(SOURCE, "No summary generated (insufficient activity or transcript)");
    return false;
  }

  const stored = storeSessionSummary(binary, cwd, sessionId, summary);
  if (stored) {
    debug(SOURCE, `Stored session summary for ${sessionId.slice(0, 8)}`);
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
