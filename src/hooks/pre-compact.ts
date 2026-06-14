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
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
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
import {
  buildSessionCheckpoint,
  getTaskTree,
  getGitLiveState,
} from "../core/session-checkpoint-logic.js";
import { debug } from "../lib/logger.js";
import { recordObserveEvent } from "../core/read-tracking.js";

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
      emitHookResult();
      return;
    }

    const data: PreCompactInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Save state snapshot before compaction — delegates to core
    const binary = findAideBinary(cwd);
    if (binary) {
      // Emit a lifecycle trigger so PreCompact is traceable in the dashboard,
      // symmetric with session-start and subagent-start/stop.
      recordObserveEvent(binary, cwd, {
        kind: "session",
        name: "pre-compact",
        category: "lifecycle",
        subtype: (data as { trigger?: string }).trigger || "compact",
        session: sessionId,
      });

      coreSaveStateSnapshot(binary, cwd, sessionId);

      // Persist a session summary as a memory before context is compacted.
      // This ensures the work-so-far is recoverable after compaction.
      // Uses partials (if available) for a richer summary, falling back to git-only.
      try {
        const partials = gatherPartials(binary, cwd, sessionId);
        const commits = getSessionCommits(cwd);
        let summary: string | null = null;
        // `checkpoint` tag marks the richest, structured snapshot so the
        // session-start resume path can find it; `partial` keeps it out of
        // normal recall and lets the session-end summary supersede it.
        let tags = `partial,session-summary,session:${sessionId}`;

        if (partials.length > 0) {
          // Structured checkpoint: pulls task tree + live git state from aide's
          // own stores in addition to the partial roll-up. No LLM needed.
          summary = buildSessionCheckpoint({
            sessionId,
            partials,
            commits,
            taskTree: getTaskTree(binary, cwd),
            liveState: getGitLiveState(cwd),
          });
          if (summary) {
            tags = `partial,checkpoint,session-summary,session:${sessionId}`;
            debug(
              SOURCE,
              `Built structured checkpoint from ${partials.length} partials`,
            );
          } else {
            // Defensive: partials present but checkpoint empty — fall back.
            summary = buildSummaryFromPartials(partials, commits, []);
          }
        }

        // Fall back to state-only summary if no partials
        if (!summary) {
          summary = buildSessionSummaryFromState(cwd);
        }

        if (summary) {
          (await import("child_process")).execFileSync(
            binary,
            ["memory", "add", "--category=session", `--tags=${tags}`, summary],
            { cwd, stdio: "pipe", timeout: 5000 },
          );
          debug(
            SOURCE,
            `Saved pre-compaction checkpoint for ${sessionId.slice(0, 8)}`,
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
    emitHookResult();
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult();
  }
}

installHookSafetyNet(SOURCE);

main();
