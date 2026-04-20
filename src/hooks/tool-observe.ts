#!/usr/bin/env node
/**
 * Tool Observe Hook (PostToolUse)
 *
 * Single-purpose: record every Claude-native tool invocation as an
 * observe.KindToolCall event. Mirror image of the MCP middleware on the Go
 * side — together they give the dashboard complete tool-call coverage.
 *
 * All taxonomy (tool → category/subtype) and the recording itself live in
 * src/core/tool-observe.ts so the OpenCode tool.execute.after handler can
 * reuse the same logic.
 */

import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { recordToolEvent } from "../core/tool-observe.js";
import { debug } from "../lib/logger.js";

const SOURCE = "tool-observe";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  tool_result?: { success: boolean };
}

function outputContinue(): void {
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    console.log('{"continue":true}');
  }
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      outputContinue();
      return;
    }
    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const toolName = data.tool_name;
    if (!toolName) {
      outputContinue();
      return;
    }
    const binary = findAideBinary({
      cwd,
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (!binary) {
      outputContinue();
      return;
    }
    recordToolEvent(binary, cwd, {
      toolName,
      toolInput: data.tool_input as ToolInput,
      success: data.tool_result?.success,
      sessionId: data.session_id,
    });
  } catch (err) {
    debug(SOURCE, `Hook error: ${err}`);
  }
  outputContinue();
}

type ToolInput = {
  file_path?: string;
  offset?: number;
  limit?: number;
  command?: string;
  pattern?: string;
};

process.on("uncaughtException", (err) => {
  debug(SOURCE, `UNCAUGHT EXCEPTION: ${err}`);
  outputContinue();
  process.exit(0);
});

process.on("unhandledRejection", (reason) => {
  debug(SOURCE, `UNHANDLED REJECTION: ${reason}`);
  outputContinue();
  process.exit(0);
});

main();
