/**
 * Tool tracking and HUD updating — platform-agnostic.
 *
 * Extracted from src/hooks/tool-tracker.ts and src/hooks/hud-updater.ts.
 * Tracks tool usage per-agent and updates session statistics.
 */

import { setState, getState } from "./aide-client.js";
import type { ToolUseInfo } from "./types.js";

/**
 * Format tool description for HUD display
 */
export function formatToolDescription(
  toolName: string,
  toolInput?: ToolUseInfo["toolInput"],
): string {
  if (!toolInput) return toolName;

  switch (toolName) {
    case "Bash":
      if (toolInput.command) {
        const cmd =
          toolInput.command.length > 40
            ? toolInput.command.slice(0, 37) + "..."
            : toolInput.command;
        return `Bash(${cmd})`;
      }
      return toolName;

    case "Read":
      if (toolInput.file_path) {
        const filename =
          toolInput.file_path.split("/").pop() || toolInput.file_path;
        return `Read(${filename})`;
      }
      return toolName;

    case "Edit":
    case "Write":
      if (toolInput.file_path) {
        const filename =
          toolInput.file_path.split("/").pop() || toolInput.file_path;
        return `${toolName}(${filename})`;
      }
      return toolName;

    case "Task":
      if (toolInput.description) {
        const desc =
          toolInput.description.length > 30
            ? toolInput.description.slice(0, 27) + "..."
            : toolInput.description;
        return `Task(${desc})`;
      }
      return toolName;

    case "Grep":
    case "Glob":
      return toolName;

    default:
      return toolName;
  }
}

/**
 * Track a tool use event (PreToolUse)
 */
export function trackToolUse(
  binary: string,
  cwd: string,
  info: ToolUseInfo,
): void {
  const { toolName, agentId, toolInput } = info;

  if (agentId && toolName) {
    const toolDesc = formatToolDescription(toolName, toolInput);
    setState(binary, cwd, "currentTool", toolDesc, agentId);
  }
}

/**
 * Update session state after tool completion (PostToolUse)
 */
export function updateToolStats(
  binary: string,
  cwd: string,
  toolName: string,
  agentId?: string,
): void {
  // Initialize startedAt if not set
  const existingStartedAt = getState(binary, cwd, "startedAt");
  if (!existingStartedAt) {
    setState(binary, cwd, "startedAt", new Date().toISOString());
  }

  // Track tool calls
  const currentToolCalls = parseInt(
    getState(binary, cwd, "toolCalls") || "0",
    10,
  );
  setState(binary, cwd, "toolCalls", String(currentToolCalls + 1));
  setState(binary, cwd, "lastToolUse", new Date().toISOString());
  setState(binary, cwd, "lastTool", toolName);

  // Clear currentTool since PostToolUse means the tool completed
  if (agentId) {
    setState(binary, cwd, "currentTool", "", agentId);
    setState(binary, cwd, "lastTool", toolName, agentId);
  }
}
