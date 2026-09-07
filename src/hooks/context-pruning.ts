#!/usr/bin/env node
/** Claude-compatible PostToolUse adapter. Rewrites are proposals, not delivery receipts. */
import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
  detectPlatform,
} from "../lib/hook-utils.js";
import { setSessionContext } from "../lib/anchor.js";
import { debug } from "../lib/logger.js";
import { contextWindow } from "../core/context-window.js";
import { recordObserveEventsBatch } from "../core/read-tracking.js";
import {
  processPruningHook,
  type PruningHookInput,
} from "../core/context-pruning/claude.js";

const SOURCE = "context-pruning";
async function main(): Promise<void> {
  try {
    const data = JSON.parse(await readStdin()) as PruningHookInput;
    const cwd = data.cwd || process.cwd();
    const host = detectPlatform();
    // Codex does not establish this Claude-specific replacement contract.
    if (host !== "claude-code" || !data.session_id) {
      emitHookResult();
      return;
    }
    setSessionContext(data.session_id);
    const binary = findAideBinary(cwd, data.session_id);
    if (!binary) {
      emitHookResult();
      return;
    }
    const identity = {
      host,
      sessionId: data.session_id,
      actorId: data.agent_id || data.session_id,
    };
    const result = processPruningHook(
      cwd,
      identity,
      contextWindow(binary, cwd, identity),
      data,
    );
    if (result) {
      if (result.event) recordObserveEventsBatch(binary, cwd, [result.event]);
      emitHookResult({
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PostToolUse",
          updatedToolOutput: result.replacement,
        },
      });
    } else emitHookResult();
  } catch (err) {
    debug(SOURCE, `Hook error: ${err}`);
    emitHookResult();
  }
}
installHookSafetyNet(SOURCE);
main();
