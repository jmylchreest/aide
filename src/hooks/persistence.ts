#!/usr/bin/env node
/**
 * Persistence Hook (Stop)
 *
 * Prevents Claude from stopping when work is incomplete.
 * Checks for active modes (ralph, autopilot) via aide-memory state.
 */

import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { checkPersistence } from "../core/persistence-logic.js";

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
      console.log(JSON.stringify({}));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();

    if (data.stop_hook_active) {
      console.log(JSON.stringify({}));
      return;
    }

    const binary = findAideBinary({
      cwd,
      pluginRoot: process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (!binary) {
      console.log(JSON.stringify({}));
      return;
    }

    const result = checkPersistence(binary, cwd);
    if (!result) {
      console.log(JSON.stringify({}));
      return;
    }

    const output: HookOutput = {
      decision: "block",
      reason: result.reason,
    };

    console.log(JSON.stringify(output));
  } catch {
    console.log(JSON.stringify({}));
  }
}

main();
