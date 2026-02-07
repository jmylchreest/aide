#!/usr/bin/env node
/**
 * Session End Hook (SessionEnd)
 *
 * Called when a Claude Code session ends cleanly.
 * Performs final cleanup and records session metrics.
 *
 * SessionEnd data from Claude Code:
 * - session_id, cwd
 * - duration (optional)
 */

import {
  readStdin,
  findAideBinary,
  clearAgentState,
  deleteMemoryState,
  setMemoryState,
  runAide,
} from "../lib/hook-utils.js";

interface SessionEndInput {
  event: "SessionEnd";
  session_id: string;
  cwd: string;
  duration?: number;
}

/**
 * Record session end and cleanup temporary state
 */
function cleanupSession(
  cwd: string,
  sessionId: string,
  duration?: number,
): void {
  if (!findAideBinary(cwd)) return;

  // Record session end (best effort)
  const durationStr = duration ? ` (${Math.round(duration / 1000)}s)` : "";
  runAide(cwd, [
    "message",
    "send",
    `Session ${sessionId} ended${durationStr}`,
    "--from=system",
    "--type=system",
  ]);

  // Clear transient state for this session/agent
  clearAgentState(cwd, sessionId);

  // Clear global session state (these are set by hud-updater and keyword-detector)
  const globalSessionKeys = [
    "mode",
    "startedAt",
    "modelTier",
    "agentCount",
    "toolCalls",
    "lastToolUse",
    "lastTool",
  ];
  for (const key of globalSessionKeys) {
    deleteMemoryState(cwd, key);
  }

  // Record session metrics
  if (duration) {
    setMemoryState(cwd, "last_session_duration", String(duration));
  }
  setMemoryState(cwd, "last_session_end", new Date().toISOString());
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: SessionEndInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Cleanup session
    cleanupSession(cwd, sessionId, data.duration);

    // Always continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, continue anyway
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
