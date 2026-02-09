#!/usr/bin/env node
/**
 * Pre-Tool Enforcer Hook (PreToolUse)
 *
 * Enforces tool access rules:
 * - Read-only agents cannot use write tools
 * - Injects contextual reminders
 * - Tracks active state
 *
 * Core logic is in src/core/tool-enforcement.ts for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { evaluateToolUse } from "../core/tool-enforcement.js";
import { findAideBinary, getState } from "../core/aide-client.js";

const SOURCE = "pre-tool-enforcer";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_name?: string;
  agent_id?: string;
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  continue: boolean;
  message?: string;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const agentName = data.agent_name || "";
    const cwd = data.cwd || process.cwd();

    // Resolve active mode from aide binary (source of truth: BBolt store)
    let activeMode: string | null = null;
    try {
      const binary = findAideBinary({
        cwd,
        pluginRoot:
          process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
      });
      if (binary) {
        activeMode = getState(binary, cwd, "mode");
      }
    } catch (err) {
      debug(SOURCE, `Failed to resolve active mode (non-fatal): ${err}`);
    }

    const result = evaluateToolUse(
      toolName,
      agentName || undefined,
      activeMode,
    );

    if (!result.allowed) {
      const output: HookOutput = {
        continue: false,
        message: result.denyMessage,
      };
      console.log(JSON.stringify(output));
      return;
    }

    if (result.reminder) {
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.reminder,
        },
      };
      console.log(JSON.stringify(output));
    } else {
      console.log(JSON.stringify({ continue: true }));
    }
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
