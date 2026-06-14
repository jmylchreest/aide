#!/usr/bin/env node
/**
 * Tool Observe Hook (PostToolUse + PostToolUseFailure)
 *
 * Single-purpose: record every Claude-native tool invocation as an
 * observe.KindToolCall event. Mirror image of the MCP middleware on the Go
 * side — together they give the dashboard complete tool-call coverage.
 *
 * Claude Code fires PostToolUse only on success and PostToolUseFailure on
 * failure (the latter carries top-level is_error/error/exit_code). This hook is
 * registered for BOTH so failed tool calls are recorded with their error text —
 * the signal the friction detector reads.
 *
 * All taxonomy (tool → category/subtype) and the recording itself live in
 * src/core/tool-observe.ts so the OpenCode tool.execute.after handler can
 * reuse the same logic.
 */

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
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
  // Claude Code passes the tool's actual output payload as tool_response.
  // Shape varies per tool (string for Bash, object for others).
  tool_response?: unknown;
  // PostToolUseFailure-only fields: the harness flags the failure at the top
  // level (PostToolUse never sets these — it only fires on success).
  is_error?: boolean;
  error?: string;
  exit_code?: number;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      emitHookResult();
      return;
    }
    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const toolName = data.tool_name;
    if (!toolName) {
      emitHookResult();
      return;
    }
    const binary = findAideBinary(cwd);
    if (!binary) {
      emitHookResult();
      return;
    }
    recordToolEvent(binary, cwd, {
      toolName,
      toolInput: data.tool_input as ToolInput,
      toolResponse: data.tool_response,
      // PostToolUseFailure marks failure at the top level; PostToolUse omits
      // these (success only). Map both into the shared recorder.
      success: data.is_error === true ? false : data.tool_result?.success,
      errorText: data.error,
      sessionId: data.session_id,
    });
  } catch (err) {
    debug(SOURCE, `Hook error: ${err}`);
  }
  emitHookResult();
}

type ToolInput = {
  file_path?: string;
  offset?: number;
  limit?: number;
  command?: string;
  pattern?: string;
};

installHookSafetyNet(SOURCE);

main();
