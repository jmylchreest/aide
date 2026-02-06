#!/usr/bin/env node
/**
 * Tool Tracker Hook (PreToolUse)
 *
 * Tracks the currently running tool per agent for HUD display.
 * Sets currentTool in aide-memory before tool execution.
 */

import { readStdin, setMemoryState } from '../lib/hook-utils.js';

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
    // Task tool specific
    model?: string;
    subagent_type?: string;
  };
  transcript_path?: string;
  permission_mode?: string;
}

/**
 * Format tool description for HUD display
 */
function formatToolDescription(toolName: string, tool_input?: HookInput['tool_input']): string {
  if (!tool_input) return toolName;

  // Show relevant details based on tool type
  switch (toolName) {
    case 'Bash':
      if (tool_input.command) {
        // Truncate long commands
        const cmd = tool_input.command.length > 40 ? tool_input.command.slice(0, 37) + '...' : tool_input.command;
        return `Bash(${cmd})`;
      }
      return toolName;

    case 'Read':
      if (tool_input.file_path) {
        const filename = tool_input.file_path.split('/').pop() || tool_input.file_path;
        return `Read(${filename})`;
      }
      return toolName;

    case 'Edit':
    case 'Write':
      if (tool_input.file_path) {
        const filename = tool_input.file_path.split('/').pop() || tool_input.file_path;
        return `${toolName}(${filename})`;
      }
      return toolName;

    case 'Task':
      if (tool_input.description) {
        const desc = tool_input.description.length > 30 ? tool_input.description.slice(0, 27) + '...' : tool_input.description;
        return `Task(${desc})`;
      }
      return toolName;

    case 'Grep':
    case 'Glob':
      return toolName;

    default:
      return toolName;
  }
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
    const toolName = data.tool_name || '';

    if (agentId && toolName) {
      const toolDesc = formatToolDescription(toolName, data.tool_input);
      setMemoryState(cwd, 'currentTool', toolDesc, agentId);
    }

    // Always continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
