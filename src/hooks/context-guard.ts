#!/usr/bin/env node
/**
 * Context Guard Hook (PreToolUse)
 *
 * Offers conditional navigation advice for large unbounded reads, and reuse
 * hints when matching full-file text was observed in the current context window.
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
  detectPlatform,
} from "../lib/hook-utils.js";
import { setSessionContext } from "../lib/anchor.js";
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
    setSessionContext(sessionId);

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
      const hintResult = checkSmartReadHint(toolName, toolInput, cwd, binary, {
        host: detectPlatform(),
        sessionId: data.session_id,
        actorId: data.agent_id || data.session_id,
      });
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
