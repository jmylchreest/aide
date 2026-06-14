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

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import {
  checkComments,
  getCheckableFilePath,
  getContentToCheck,
} from "../core/comment-checker.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "";

    // Only check Write/Edit/MultiEdit tool calls
    const filePath = getCheckableFilePath(toolName, toolInput);
    if (!filePath) {
      emitHookResult({ continue: true });
      return;
    }

    // Get the content to analyze
    const contentResult = getContentToCheck(toolName, toolInput);
    if (!contentResult) {
      emitHookResult({ continue: true });
      return;
    }

    const [content, isNewContent] = contentResult;
    const result = checkComments(filePath, content, isNewContent);

    if (result.hasExcessiveComments) {
      debug(
        SOURCE,
        `Detected ${result.suspiciousCount} suspicious comments in ${filePath}`,
      );
      try {
        const binary = findAideBinary(cwd);
        if (binary && result.warning) {
          emitInjectionEvent(binary, cwd, {
            source: SOURCE,
            subtype: "guard",
            content: result.warning,
            sessionId,
            attrs: {
              tool: toolName,
              file: filePath,
              suspicious_count: String(result.suspiciousCount),
            },
          });
        }
      } catch {
        // Non-fatal
      }
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PostToolUse",
          additionalContext: result.warning,
        },
      };
      emitHookResult(output);
    } else {
      emitHookResult({ continue: true });
    }
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult({ continue: true });
  }
}

installHookSafetyNet(SOURCE);

main();
