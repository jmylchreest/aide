#!/usr/bin/env node
/**
 * Pre-Tool Enforcer Hook (PreToolUse)
 *
 * Enforces tool access rules:
 * - Read-only agents cannot use write tools
 * - Injects contextual reminders
 * - Tracks active state
 */

import { existsSync, readFileSync } from 'fs';
import { join } from 'path';
import { readStdin } from '../lib/hook-utils.js';

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
    additionalContext?: string;
  };
}

// Tools that modify state
const WRITE_TOOLS = [
  'Edit',
  'Write',
  'Bash',
  'NotebookEdit',
  'MultiEdit',
];

// Read-only agents (should not use write tools)
const READ_ONLY_AGENTS = [
  'architect',
  'explore',
  'researcher',
  'planner',
  'reviewer',
  'analyst',
  'product-owner',
];

// Agents that should have access to specific tool categories
const AGENT_TOOL_RESTRICTIONS: Record<string, { allowed?: string[]; denied?: string[] }> = {
  'architect': {
    denied: ['Edit', 'Write', 'Bash', 'NotebookEdit'],
  },
  'explore': {
    denied: ['Edit', 'Write', 'Bash', 'NotebookEdit'],
  },
  'researcher': {
    denied: ['Edit', 'Write', 'Bash', 'NotebookEdit'],
  },
  'planner': {
    denied: ['Edit', 'Write', 'Bash', 'NotebookEdit'],
  },
  'reviewer': {
    denied: ['Edit', 'Write', 'NotebookEdit'], // Can use Bash for running tests
  },
  'writer': {
    // Can write documentation
  },
  'executor': {
    // Full access
  },
  'designer': {
    // Full access for UI work
  },
};

interface ModeState {
  active: boolean;
  mode: string;
}

/**
 * Check if an agent is restricted from using a tool
 */
function isToolDenied(agentName: string, toolName: string): boolean {
  const restrictions = AGENT_TOOL_RESTRICTIONS[agentName];
  if (!restrictions) return false;

  if (restrictions.denied && restrictions.denied.includes(toolName)) {
    return true;
  }

  if (restrictions.allowed && !restrictions.allowed.includes(toolName)) {
    return true;
  }

  return false;
}

/**
 * Read active mode state
 */
function getActiveMode(cwd: string): string | null {
  const modes = ['autopilot', 'ralph', 'eco', 'swarm', 'plan'];

  for (const mode of modes) {
    const statePath = join(cwd, '.aide', 'state', `${mode}-state.json`);
    if (existsSync(statePath)) {
      try {
        const state: ModeState = JSON.parse(readFileSync(statePath, 'utf-8'));
        if (state.active) {
          return mode;
        }
      } catch {
        // Ignore parse errors
      }
    }
  }

  return null;
}

/**
 * Build contextual reminder based on active mode
 */
function buildReminder(mode: string | null, toolName: string): string | null {
  if (!mode) return null;

  const reminders: Record<string, string> = {
    'ralph': `[aide:ralph] Persistence active. Verify work is complete before stopping.`,
    'autopilot': `[aide:autopilot] Autonomous mode. Continue until all tasks verified.`,
    'eco': `[aide:eco] Token-efficient mode. Minimize context, use fast models.`,
    'swarm': `[aide:swarm] Swarm active. Use aide-memory for coordination.`,
  };

  return reminders[mode] || null;
}


async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || '';
    // agent_name may come from Claude Code for typed agents
    const agentName = data.agent_name || '';
    const cwd = data.cwd || process.cwd();

    // Debug: log what we received
    // console.error(`[pre-tool-enforcer] tool=${toolName}, agent=${agentName}`);

    // Check tool restrictions for agents
    if (agentName && toolName && isToolDenied(agentName, toolName)) {
      const output: HookOutput = {
        continue: false,
        message: `Agent "${agentName}" is read-only and cannot use "${toolName}". Delegate to executor for modifications.`,
      };
      console.log(JSON.stringify(output));
      return;
    }

    // Get active mode and build reminder
    const activeMode = getActiveMode(cwd);
    const reminder = buildReminder(activeMode, toolName);

    if (reminder) {
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          additionalContext: reminder,
        },
      };
      console.log(JSON.stringify(output));
    } else {
      console.log(JSON.stringify({ continue: true }));
    }
  } catch (error) {
    // On error, allow continuation
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
