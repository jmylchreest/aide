#!/usr/bin/env node
/**
 * Persistence Hook (Stop)
 *
 * Prevents Claude from stopping when work is incomplete.
 * Checks for active modes (ralph, autopilot) via aide-memory state.
 */

import {
  readStdin,
  getMemoryState,
  setMemoryState,
} from "../lib/hook-utils.js";

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

const PERSISTENCE_MODES = ["ralph", "autopilot"];
const MAX_ITERATIONS = 20;

function getActiveMode(cwd: string): string | null {
  const mode = getMemoryState(cwd, "mode");
  if (mode && PERSISTENCE_MODES.includes(mode)) {
    return mode;
  }
  return null;
}

function buildReinforcement(mode: string, iteration: number): string {
  if (iteration > MAX_ITERATIONS) {
    return `Maximum reinforcements (${MAX_ITERATIONS}) reached. Releasing ${mode} mode.`;
  }

  return `**${mode.toUpperCase()} MODE ACTIVE** (iteration ${iteration}/${MAX_ITERATIONS})

You attempted to stop but work may be incomplete.

Before stopping, verify:
- All tasks in your todo list are marked complete
- All requested functionality is implemented
- Tests pass (if applicable)
- No errors remain unaddressed

If ANY item is incomplete, CONTINUE WORKING.

Use TaskList to check your progress.`;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      // Empty object = allow stop (no blocking)
      console.log(JSON.stringify({}));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();

    // Prevent infinite loops - if stop_hook_active, we're already being reinforced
    if (data.stop_hook_active) {
      // Allow stop to prevent infinite loop
      console.log(JSON.stringify({}));
      return;
    }

    const mode = getActiveMode(cwd);
    if (!mode) {
      // No persistence mode active, allow stop
      console.log(JSON.stringify({}));
      return;
    }

    // Get and increment iteration counter
    const iterStr = getMemoryState(cwd, `${mode}_iterations`) || "0";
    const iteration = parseInt(iterStr, 10) + 1;
    setMemoryState(cwd, `${mode}_iterations`, String(iteration));

    if (iteration > MAX_ITERATIONS) {
      // Clear the mode after max iterations, allow stop
      setMemoryState(cwd, "mode", "");
      setMemoryState(cwd, `${mode}_iterations`, "0");
      console.log(JSON.stringify({}));
      return;
    }

    // Block stop and provide reason
    const output: HookOutput = {
      decision: "block",
      reason: buildReinforcement(mode, iteration),
    };

    console.log(JSON.stringify(output));
  } catch {
    console.log(JSON.stringify({}));
  }
}

main();
