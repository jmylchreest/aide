#!/usr/bin/env node
/**
 * Persistence Hook (Stop)
 *
 * Prevents Claude from stopping when work is incomplete.
 * Checks for active modes (autopilot) via aide-memory state.
 */

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
} from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { checkPersistence } from "../core/persistence-logic.js";
import { debug } from "../lib/logger.js";

const SOURCE = "persistence";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  stop_hook_active?: boolean;
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  decision?: "block";
  reason?: string;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      emitHookResult({});
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();

    if (data.stop_hook_active) {
      emitHookResult({});
      return;
    }

    const binary = findAideBinary({
      cwd,
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (!binary) {
      emitHookResult({});
      return;
    }

    const result = checkPersistence(binary, cwd, data.session_id);
    if (!result) {
      emitHookResult({});
      return;
    }

    const output: HookOutput = {
      decision: "block",
      reason: result.reason,
    };

    emitHookResult(output);
  } catch (err) {
    debug(SOURCE, `Hook error: ${err}`);
    emitHookResult({});
  }
}

// This Stop hook emits {} (not {continue:true}) as its neutral result.
installHookSafetyNet(SOURCE, {});

main();
