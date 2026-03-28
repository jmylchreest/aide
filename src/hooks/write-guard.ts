#!/usr/bin/env node
/**
 * Write Guard Hook (PreToolUse)
 *
 * Advises the agent to use Edit instead of Write on existing files.
 * Injects advisory context (soft warning) rather than blocking,
 * preventing excessive permission prompts in Claude Code.
 *
 * Core logic is in src/core/write-guard.ts for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { checkWriteGuard } from "../core/write-guard.js";

const SOURCE = "write-guard";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_name?: string;
  agent_id?: string;
  tool_input?: Record<string, unknown>;
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
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();

    const result = checkWriteGuard(toolName, toolInput, cwd);

    if (!result.allowed) {
      debug(
        SOURCE,
        `Advisory: Write to existing file: ${toolInput.file_path || toolInput.filePath || toolInput.path}`,
      );
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: data.hook_event_name || "PreToolUse",
          additionalContext: result.message,
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
