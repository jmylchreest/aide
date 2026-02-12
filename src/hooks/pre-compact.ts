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
  getSessionCommits,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import {
  gatherPartials,
  buildSummaryFromPartials,
} from "../core/partial-memory.js";
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
      // Uses partials (if available) for a richer summary, falling back to git-only.
      try {
        const partials = gatherPartials(binary, cwd, sessionId);
        let summary: string | null = null;

        if (partials.length > 0) {
          // Build from partials + git data
          const commits = getSessionCommits(cwd);
          summary = buildSummaryFromPartials(partials, commits, []);
          debug(
            SOURCE,
            `Built pre-compact summary from ${partials.length} partials`,
          );
        }

        // Fall back to state-only summary if no partials
        if (!summary) {
          summary = buildSessionSummaryFromState(cwd);
        }

        if (summary) {
          // Tag as partial so the session-end summary supersedes it
          const dbPath = (await import("path")).join(
            cwd,
            ".aide",
            "memory",
            "store.db",
          );
          const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
          const tags = `partial,session-summary,session:${sessionId.slice(0, 8)}`;
          (await import("child_process")).execFileSync(
            binary,
            ["memory", "add", "--category=session", `--tags=${tags}`, summary],
            { env, stdio: "pipe", timeout: 5000 },
          );
          debug(
            SOURCE,
            `Saved pre-compaction partial session summary for ${sessionId.slice(0, 8)}`,
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
