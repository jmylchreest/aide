#!/usr/bin/env node
/**
 * Write Guard Hook (PreToolUse)
 *
 * Blocks the Write tool from being used on existing files.
 * Forces the agent to use Edit instead, preventing destructive
 * full-file rewrites that lose forgotten code.
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
        `Blocked Write to existing file: ${toolInput.file_path || toolInput.filePath || toolInput.path}`,
      );
      const output: HookOutput = {
        continue: false,
        message: result.message,
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
