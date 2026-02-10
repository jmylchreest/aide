#!/usr/bin/env node
/**
 * Comment Checker Hook (PostToolUse)
 *
 * Detects excessive or obvious comments in code written by AI agents.
 * Injects a warning via additionalContext to nudge the agent toward
 * cleaner code without blocking the tool call.
 *
 * Core logic is in src/core/comment-checker.ts for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import {
  checkComments,
  getCheckableFilePath,
  getContentToCheck,
} from "../core/comment-checker.js";

const SOURCE = "comment-checker";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_id?: string;
  tool_input?: Record<string, unknown>;
  tool_result?: {
    success: boolean;
    duration?: number;
  };
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  continue: true;
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

    // Only check Write/Edit/MultiEdit tool calls
    const filePath = getCheckableFilePath(toolName, toolInput);
    if (!filePath) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    // Get the content to analyze
    const contentResult = getContentToCheck(toolName, toolInput);
    if (!contentResult) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const [content, isNewContent] = contentResult;
    const result = checkComments(filePath, content, isNewContent);

    if (result.hasExcessiveComments) {
      debug(
        SOURCE,
        `Detected ${result.suspiciousCount} suspicious comments in ${filePath}`,
      );
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PostToolUse",
          additionalContext: result.warning,
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
