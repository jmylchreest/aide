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

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import {
  checkContextGuard,
  checkSmartReadHint,
} from "../core/context-guard.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    const result = checkContextGuard(toolName, toolInput, cwd, sessionId);
    const binary = findAideBinary(cwd, data.session_id);

    if (result.shouldAdvise && result.advisory) {
      debug(SOURCE, `Advising on large file read`);
      if (binary) {
        try {
          emitInjectionEvent(binary, cwd, {
            source: SOURCE,
            subtype: "guard",
            name: "large-file-advisory",
            content: result.advisory,
            sessionId,
            attrs: { tool: toolName },
          });
        } catch {
          // Non-fatal
        }
      }
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.advisory,
        },
      };
      emitHookResult(output);
    } else {
      // Smart read hint: suggest code index for re-reads of unchanged files
      const hintResult = checkSmartReadHint(toolName, toolInput, cwd, binary);
      if (hintResult.shouldHint && hintResult.hint) {
        debug(SOURCE, `Smart read hint triggered`);
        if (binary) {
          try {
            emitInjectionEvent(binary, cwd, {
              source: SOURCE,
              subtype: "guard",
              name: "smart-read-hint",
              content: hintResult.hint,
              sessionId,
              attrs: { tool: toolName },
            });
          } catch {
            // Non-fatal
          }
        }
        const output: HookOutput = {
          continue: true,
          hookSpecificOutput: {
            hookEventName: "PreToolUse",
            additionalContext: hintResult.hint,
          },
        };
        emitHookResult(output);
      } else {
        emitHookResult({ continue: true });
      }
    }
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult({ continue: true });
  }
}

installHookSafetyNet(SOURCE);

main();
