#!/usr/bin/env node
/**
 * Pre-Compact Hook (PreCompact)
 *
 * Called before Claude Code compacts/summarizes the conversation.
 * Preserves important context in aide-memory before summarization.
 *
 * PreCompact data from Claude Code:
 * - session_id, cwd
 * - summary_prompt (the prompt used for summarization)
 */

import {
  readStdin,
  findAideBinary,
  runAide,
  setMemoryState,
} from "../lib/hook-utils.js";

interface PreCompactInput {
  event: "PreCompact";
  session_id: string;
  cwd: string;
  summary_prompt?: string;
}

/**
 * Save current state snapshot before compaction
 */
function saveStateSnapshot(cwd: string, sessionId: string): void {
  if (!findAideBinary(cwd)) return;

  // Record compaction event (best effort)
  runAide(cwd, [
    "message",
    "send",
    `Context compaction initiated for session ${sessionId}`,
    "--from=system",
    "--type=system",
  ]);

  // Get current state and preserve it
  const stateOutput = runAide(cwd, ["state", "list"]);
  if (stateOutput?.trim()) {
    // Store a snapshot of the current state
    const safeState = stateOutput.replace(/\n/g, " ").slice(0, 1000);
    setMemoryState(cwd, "precompact_snapshot", safeState);
  }

  // Record the compaction timestamp
  setMemoryState(cwd, "last_compaction", new Date().toISOString());
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: PreCompactInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Save state snapshot before compaction
    saveStateSnapshot(cwd, sessionId);

    // Always allow compaction to continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, allow compaction to continue
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
