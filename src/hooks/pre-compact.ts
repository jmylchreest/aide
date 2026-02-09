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

import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { saveStateSnapshot as coreSaveStateSnapshot } from "../core/pre-compact-logic.js";

interface PreCompactInput {
  event: "PreCompact";
  session_id: string;
  cwd: string;
  summary_prompt?: string;
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

    // Save state snapshot before compaction — delegates to core
    const binary = findAideBinary({
      cwd,
      pluginRoot: process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (binary) {
      coreSaveStateSnapshot(binary, cwd, sessionId);
    }

    // Always allow compaction to continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, allow compaction to continue
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
