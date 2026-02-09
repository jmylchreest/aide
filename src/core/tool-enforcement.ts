/**
 * Tool enforcement logic — platform-agnostic.
 *
 * Extracted from src/hooks/pre-tool-enforcer.ts.
 * Enforces tool access rules for named agents and provides
 * contextual mode reminders.
 *
 * Used by both Claude Code hooks and OpenCode plugin.
 *
 * NOTE: Active mode is read via `aide state get mode` (BBolt store),
 * NOT from filesystem JSON files. Callers must resolve the active mode
 * via getActiveMode(binary, cwd) from persistence-logic.ts and pass it in.
 */

import { debug } from "../lib/logger.js";

const SOURCE = "tool-enforcement";

// Tools that modify state
export const WRITE_TOOLS = [
  "Edit",
  "Write",
  "Bash",
  "NotebookEdit",
  "MultiEdit",
];

// Agents that should have access to specific tool categories
export const AGENT_TOOL_RESTRICTIONS: Record<
  string,
  { allowed?: string[]; denied?: string[] }
> = {
  architect: {
    denied: ["Edit", "Write", "Bash", "NotebookEdit"],
  },
  explore: {
    denied: ["Edit", "Write", "Bash", "NotebookEdit"],
  },
  researcher: {
    denied: ["Edit", "Write", "Bash", "NotebookEdit"],
  },
  planner: {
    denied: ["Edit", "Write", "Bash", "NotebookEdit"],
  },
  reviewer: {
    denied: ["Edit", "Write", "NotebookEdit"], // Can use Bash for running tests
  },
  writer: {
    // Can write documentation
  },
  executor: {
    // Full access
  },
  designer: {
    // Full access for UI work
  },
};

/**
 * Check if an agent is restricted from using a tool
 */
export function isToolDenied(agentName: string, toolName: string): boolean {
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
 * Build contextual reminder based on active mode
 */
export function buildReminder(mode: string | null): string | null {
  if (!mode) return null;

  const reminders: Record<string, string> = {
    ralph: `[aide:ralph] Persistence active. Verify work is complete before stopping.`,
    autopilot: `[aide:autopilot] Autonomous mode. Continue until all tasks verified.`,
    eco: `[aide:eco] Token-efficient mode. Minimize context, use fast models.`,
    swarm: `[aide:swarm] Swarm active. Use aide-memory for coordination.`,
  };

  return reminders[mode] || null;
}

export interface ToolEnforcementResult {
  /** Whether the tool use should continue */
  allowed: boolean;
  /** Message explaining why the tool was denied */
  denyMessage?: string;
  /** Contextual reminder to inject (if any) */
  reminder?: string;
}

/**
 * Evaluate tool enforcement for a given tool use.
 *
 * @param toolName - The tool being invoked
 * @param agentName - The agent's role name (if known)
 * @param activeMode - The current aide mode (from `aide state get mode`).
 *                     Caller is responsible for resolving this via the aide binary.
 */
export function evaluateToolUse(
  toolName: string,
  agentName: string | undefined,
  activeMode: string | null,
): ToolEnforcementResult {
  // Check tool restrictions for agents
  if (agentName && toolName && isToolDenied(agentName, toolName)) {
    return {
      allowed: false,
      denyMessage: `Agent "${agentName}" is read-only and cannot use "${toolName}". Delegate to executor for modifications.`,
    };
  }

  // Build reminder based on active mode
  const reminder = buildReminder(activeMode);

  return {
    allowed: true,
    reminder: reminder || undefined,
  };
}
