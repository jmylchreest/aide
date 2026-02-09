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
import {
  buildSessionSummaryFromState,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import { debug } from "../lib/logger.js";

const SOURCE = "pre-compact";

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
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (binary) {
      coreSaveStateSnapshot(binary, cwd, sessionId);

      // Persist a session summary as a memory before context is compacted.
      // This ensures the work-so-far is recoverable after compaction.
      try {
        const summary = buildSessionSummaryFromState(cwd);
        if (summary) {
          storeSessionSummary(binary, cwd, sessionId, summary);
          debug(
            SOURCE,
            `Saved pre-compaction session summary for ${sessionId.slice(0, 8)}`,
          );
        }
      } catch (err) {
        debug(
          SOURCE,
          `Failed to save pre-compaction summary (non-fatal): ${err}`,
        );
      }
    }

    // Always allow compaction to continue
    console.log(JSON.stringify({ continue: true }));
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
