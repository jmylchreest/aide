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

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { evaluateToolUse } from "../core/tool-enforcement.js";
import { getState } from "../core/aide-client.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const agentName = data.agent_name || "";
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "";

    // Resolve active mode from aide binary (source of truth: BBolt store).
    // Mode is global by design — see the note in core/aide-client.ts.
    let activeMode: string | null = null;
    let aideBinary: string | null = null;
    try {
      aideBinary = findAideBinary(cwd);
      if (aideBinary) {
        activeMode = getState(aideBinary, cwd, "mode");
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
      emitHookResult(output);
      return;
    }

    if (result.reminder) {
      if (aideBinary) {
        try {
          emitInjectionEvent(aideBinary, cwd, {
            source: SOURCE,
            subtype: "guard",
            content: result.reminder,
            sessionId,
            attrs: {
              tool: toolName,
              ...(agentName ? { agent: agentName } : {}),
              ...(activeMode ? { mode: activeMode } : {}),
            },
          });
        } catch {
          // Non-fatal — telemetry must not block tool use
        }
      }
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.reminder,
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
