#!/usr/bin/env node
/**
 * Tool Tracker Hook (PreToolUse)
 *
 * Tracks the currently running tool per agent for HUD display.
 * Sets currentTool in aide-memory before tool execution.
 */

import { readStdin, setMemoryState, findAideBinary } from "../lib/hook-utils.js";
import {
  trackToolUse,
  formatToolDescription,
} from "../core/tool-tracking.js";
import { findAideBinary as coreFindBinary } from "../core/aide-client.js";

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
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const agentId = data.agent_id || data.session_id;
    const toolName = data.tool_name || "";

    if (agentId && toolName) {
      // Try core binary discovery first, fall back to hook-utils
      const binary = coreFindBinary({
        cwd,
        pluginRoot: process.env.CLAUDE_PLUGIN_ROOT,
      }) || findAideBinary(cwd);

      if (binary) {
        trackToolUse(binary, cwd, {
          toolName,
          agentId,
          toolInput: data.tool_input,
        });
      } else {
        // Fall back to hook-utils setMemoryState (uses its own binary discovery)
        const toolDesc = formatToolDescription(toolName, data.tool_input);
        setMemoryState(cwd, "currentTool", toolDesc, agentId);
      }
    }

    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    console.log(JSON.stringify({ continue: true }));
  }
}


process.on("uncaughtException", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});
process.on("unhandledRejection", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});

main();
