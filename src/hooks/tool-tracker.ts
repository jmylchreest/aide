#!/usr/bin/env node
/**
 * Tool Tracker Hook (PreToolUse)
 *
 * Tracks the currently running tool per agent for HUD display.
 * Sets currentTool in aide-memory before tool execution.
 */

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
} from "../lib/hook-utils.js";
import { trackToolUse, formatToolDescription } from "../core/tool-tracking.js";
import { findAideBinary } from "../core/aide-client.js";
import { debug } from "../lib/logger.js";

const SOURCE = "tool-tracker";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_id?: string;
  tool_input?: {
    command?: string;
    description?: string;
    prompt?: string;
    file_path?: string;
    model?: string;
    subagent_type?: string;
  };
  transcript_path?: string;
  permission_mode?: string;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const agentId = data.agent_id || data.session_id;
    const toolName = data.tool_name || "";

    if (agentId && toolName) {
      const binary = findAideBinary({
        cwd,
        pluginRoot:
          process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
      });

      if (binary) {
        trackToolUse(binary, cwd, {
          toolName,
          agentId,
          toolInput: data.tool_input,
        });
      }
    }

    emitHookResult({ continue: true });
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult({ continue: true });
  }
}

installHookSafetyNet(SOURCE);

main();
