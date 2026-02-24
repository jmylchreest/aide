#!/usr/bin/env node
/**
 * Context Guard Hook (PreToolUse)
 *
 * Monitors Read tool calls and advises agents to use code_outline
 * before reading large files. Also tracks code_outline/code_symbols
 * calls so it knows which files have been outlined.
 *
 * This is a soft warning — it never blocks, only injects advisory context.
 *
 * Core logic is in src/core/context-guard.ts for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { checkContextGuard } from "../core/context-guard.js";

const SOURCE = "context-guard";

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
    const sessionId = data.session_id || "unknown";

    const result = checkContextGuard(toolName, toolInput, cwd, sessionId);

    if (result.shouldAdvise && result.advisory) {
      debug(SOURCE, `Advising on large file read`);
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.advisory,
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
