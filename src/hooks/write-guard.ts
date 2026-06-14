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

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
} from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { checkWriteGuard } from "../core/write-guard.js";
import { findAideBinary } from "../core/aide-client.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "";

    const result = checkWriteGuard(toolName, toolInput, cwd);

    if (!result.allowed) {
      const filePath =
        (toolInput.file_path as string) ||
        (toolInput.filePath as string) ||
        (toolInput.path as string) ||
        "";
      debug(SOURCE, `Advisory: Write to existing file: ${filePath}`);
      try {
        const binary = findAideBinary({
          cwd,
          pluginRoot:
            process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
        });
        if (binary && result.message) {
          emitInjectionEvent(binary, cwd, {
            source: SOURCE,
            subtype: "guard",
            content: result.message,
            sessionId,
            attrs: { tool: toolName, ...(filePath ? { file: filePath } : {}) },
          });
        }
      } catch {
        // Non-fatal
      }
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: data.hook_event_name || "PreToolUse",
          additionalContext: result.message,
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
