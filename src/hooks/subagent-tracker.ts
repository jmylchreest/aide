#!/usr/bin/env node
/**
 * Subagent Tracker Hook (SubagentStart, SubagentStop)
 *
 * Tracks spawned subagents for HUD display and coordination.
 * Registers agents in aide-memory with their type, model, and task.
 * Also injects memory context for subagents (global preferences, decisions).
 *
 * SubagentStart data from Claude Code:
 * - agent_id, agent_type, session_id, prompt
 * - model, cwd, permission_mode
 *
 * SubagentStop data from Claude Code:
 * - agent_id, agent_type, output, success
 */

import { join } from "path";
import { execFileSync } from "child_process";
import { Logger } from "../lib/logger.js";
import {
  readStdin,
  findAideBinary,
  setMemoryState,
} from "../lib/hook-utils.js";
import { refreshHud } from "../lib/hud.js";
import {
  getWorktreeForAgent,
  markWorktreeComplete,
  discoverWorktrees,
  Worktree,
} from "../lib/worktree.js";

// Global logger instance
let log: Logger | null = null;

// Claude Code hook input format (uses hook_event_name, not event)
interface SubagentStartInput {
  hook_event_name: "SubagentStart";
  agent_id: string;
  agent_type: string;
  session_id: string;
  transcript_path?: string;
  cwd: string;
  permission_mode?: string;
  // Note: prompt and model are NOT provided by Claude Code
  // We'll need to get these from PreToolUse if needed
}

interface SubagentStopInput {
  hook_event_name: "SubagentStop";
  agent_id: string;
  agent_type: string;
  session_id: string;
  transcript_path?: string;
  agent_transcript_path?: string;
  stop_hook_active?: boolean;
  cwd: string;
  permission_mode?: string;
}

type HookInput = SubagentStartInput | SubagentStopInput;

interface HookOutput {
  continue: boolean;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

/**
 * Get project name from git remote or directory name
 */
function getProjectName(cwd: string): string {
  try {
    // Try git remote first
    const remoteUrl = execFileSync("git", ["config", "--get", "remote.origin.url"], {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 2000,
    })
      .toString()
      .trim();

    // Extract repo name from URL
    const match = remoteUrl.match(/[/:]([^/]+?)(?:\.git)?$/);
    if (match) return match[1];
  } catch {
    // Not a git repo or no remote
  }

  // Fallback to directory name
  return cwd.split("/").pop() || "unknown";
}

/**
 * Fetch essential memories for subagent context injection
 *
 * Subagents get:
 * - Global preferences (scope:global)
 * - Project memories (project:<name>)
 * - Project decisions
 *
 * This ensures subagents respect user preferences, project context,
 * and architectural decisions.
 */
function fetchSubagentMemories(cwd: string): {
  global: string[];
  project: string[];
  decisions: string[];
} {
  const result = {
    global: [] as string[],
    project: [] as string[],
    decisions: [] as string[],
  };

  // Check for disable flag
  if (process.env.AIDE_MEMORY_INJECT === "0") {
    return result;
  }

  const binary = findAideBinary(cwd);
  if (!binary) {
    return result;
  }

  const dbPath = join(cwd, ".aide", "memory", "store.db");
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
  const projectName = getProjectName(cwd);

  // Fetch global memories (scope:global)
  try {
    const globalOutput = execFileSync(
      binary,
      ["memory", "list", "--category=global", "--tags=scope:global", "--format=json"],
      { env, stdio: ["pipe", "pipe", "pipe"], timeout: 3000 },
    )
      .toString()
      .trim();

    if (globalOutput && globalOutput !== "[]") {
      const memories = JSON.parse(globalOutput);
      result.global = memories.map((m: { content: string }) => m.content);
    }
  } catch {
    // Ignore errors - memory is optional
  }

  // Fetch project memories (project:<name>)
  try {
    const projectOutput = execFileSync(
      binary,
      ["memory", "list", `--tags=project:${projectName}`, "--format=json"],
      { env, stdio: ["pipe", "pipe", "pipe"], timeout: 3000 },
    )
      .toString()
      .trim();

    if (projectOutput && projectOutput !== "[]") {
      const memories = JSON.parse(projectOutput);
      result.project = memories.map((m: { content: string }) => m.content);
    }
  } catch {
    // Ignore errors - memory is optional
  }

  // Fetch project decisions
  try {
    const decisionsOutput = execFileSync(
      binary,
      ["decision", "list", "--format=json"],
      { env, stdio: ["pipe", "pipe", "pipe"], timeout: 3000 },
    )
      .toString()
      .trim();

    if (decisionsOutput && decisionsOutput !== "[]") {
      const decisions = JSON.parse(decisionsOutput);
      result.decisions = decisions.map(
        (d: { topic: string; value: string }) => `**${d.topic}**: ${d.value}`,
      );
    }
  } catch {
    // Ignore errors
  }

  return result;
}

/**
 * Build context for subagent injection
 */
function buildSubagentContext(
  memories: {
    global: string[];
    project: string[];
    decisions: string[];
  },
  worktree?: Worktree,
): string | undefined {
  const lines: string[] = [];

  // Inject worktree information if this is a swarm agent
  if (worktree) {
    lines.push("<aide-subagent-context>");
    lines.push("");
    lines.push("## Swarm Worktree");
    lines.push("");
    lines.push(`You are working in an isolated git worktree for swarm mode.`);
    lines.push(`- **Worktree Path**: ${worktree.path}`);
    lines.push(`- **Branch**: ${worktree.branch}`);
    lines.push(`- **Story ID**: ${worktree.taskId || "unknown"}`);
    lines.push("");
    lines.push(
      `**IMPORTANT**: All file operations should be performed in: ${worktree.path}`,
    );
    lines.push(
      `Commit your changes to the ${worktree.branch} branch when complete.`,
    );
  }

  if (memories.global.length > 0) {
    if (lines.length === 0) {
      lines.push("<aide-subagent-context>");
    }
    lines.push("");
    lines.push("## User Preferences");
    lines.push("");
    for (const mem of memories.global) {
      lines.push(`- ${mem}`);
    }
  }

  if (memories.project.length > 0) {
    if (lines.length === 0) {
      lines.push("<aide-subagent-context>");
    }
    lines.push("");
    lines.push("## Project Context");
    lines.push("");
    for (const mem of memories.project) {
      lines.push(`- ${mem}`);
    }
  }

  if (memories.decisions.length > 0) {
    if (lines.length === 0) {
      lines.push("<aide-subagent-context>");
    }
    lines.push("");
    lines.push("## Project Decisions");
    lines.push("");
    for (const decision of memories.decisions) {
      lines.push(`- ${decision}`);
    }
  }

  if (lines.length > 0) {
    lines.push("");
    lines.push("</aide-subagent-context>");
    return lines.join("\n");
  }

  return undefined;
}

/**
 * Set state in aide-memory for an agent (wrapper with logging)
 */
function setAgentState(
  cwd: string,
  agentId: string,
  key: string,
  value: string,
): boolean {
  const truncatedValue = value.replace(/\n/g, " ").slice(0, 500);
  log?.debug(
    `setAgentState: setting ${key}="${truncatedValue}" for agent ${agentId}`,
  );
  const result = setMemoryState(cwd, key, truncatedValue, agentId);
  if (!result) {
    log?.warn(`setAgentState: failed to set ${key} for agent ${agentId}`);
  }
  return result;
}

/**
 * Handle SubagentStart event
 * Returns context to inject into the subagent
 */
async function processSubagentStart(
  data: SubagentStartInput,
): Promise<string | undefined> {
  const { agent_id, agent_type, session_id, cwd } = data;

  log?.info(
    `SubagentStart: agent_id=${agent_id}, type=${agent_type}, session=${session_id}`,
  );

  // Claude Code doesn't provide prompt/model in SubagentStart
  // Use agent_type directly as the type
  const type = agent_type;

  log?.debug(`SubagentStart: registering type=${type}`);

  // Register agent in aide-memory
  // Note: modelTier is NOT stored - model instructions are injected into context instead
  log?.start("registerAgent");
  setAgentState(cwd, agent_id, "status", "running");
  setAgentState(cwd, agent_id, "type", type);
  setAgentState(cwd, agent_id, "startedAt", new Date().toISOString());
  setAgentState(cwd, agent_id, "session", session_id); // Track which session owns this agent
  log?.end("registerAgent");

  // Refresh HUD to show the new running agent
  log?.start("refreshHud");
  refreshHud(cwd, session_id);
  log?.end("refreshHud");

  // Auto-discover any worktrees created by the orchestrator via git commands
  // This ensures we track worktrees even if they weren't created via our library
  log?.start("discoverWorktrees");
  const discovered = discoverWorktrees(cwd);
  if (discovered.length > 0) {
    log?.info(`Auto-discovered ${discovered.length} worktrees`);
  }
  log?.end("discoverWorktrees", { discovered: discovered.length });

  // Check if this agent has an associated worktree (swarm mode)
  // Match by agent_id or by pattern in worktree name
  log?.start("checkWorktree");
  let worktree = getWorktreeForAgent(cwd, agent_id);

  // If no direct match, try to match by agent_id pattern in worktree name
  // This handles cases where worktree was created before agent_id was known
  if (!worktree) {
    const { loadWorktreeState } = await import("../lib/worktree.js");
    const state = loadWorktreeState(cwd);
    // Look for worktree with matching name pattern (e.g., "story-auth" matches "agent-auth")
    const agentPattern = agent_id.replace(/^agent-/, "");
    worktree = state.active.find(
      (w) => w.name.includes(agentPattern) && !w.agentId,
    );
    if (worktree) {
      // Assign this agent to the worktree
      worktree.agentId = agent_id;
      const { saveWorktreeState } = await import("../lib/worktree.js");
      saveWorktreeState(cwd, state);
      log?.info(`Assigned worktree ${worktree.name} to agent ${agent_id}`);
    }
  }

  if (worktree) {
    log?.info(
      `Found worktree for agent ${agent_id}: ${worktree.path} (branch: ${worktree.branch})`,
    );
  }
  log?.end("checkWorktree", { hasWorktree: !!worktree });

  // Fetch memories for subagent context injection
  log?.start("fetchMemories");
  const memories = fetchSubagentMemories(cwd);
  log?.end("fetchMemories", {
    globalCount: memories.global.length,
    projectCount: memories.project.length,
    decisionCount: memories.decisions.length,
  });

  // Build context if we have memories or a worktree
  if (
    worktree ||
    memories.global.length > 0 ||
    memories.project.length > 0 ||
    memories.decisions.length > 0
  ) {
    const context = buildSubagentContext(memories, worktree);
    log?.info(
      `Injecting context for subagent: ${memories.global.length} preferences, ${memories.project.length} project, ${memories.decisions.length} decisions, worktree=${!!worktree}`,
    );
    return context;
  }

  return undefined;
}

/**
 * Handle SubagentStop event
 */
async function processSubagentStop(data: SubagentStopInput): Promise<void> {
  const { agent_id, session_id, cwd, stop_hook_active } = data;

  log?.info(
    `SubagentStop: agent_id=${agent_id}, session=${session_id}, stop_hook_active=${stop_hook_active}`,
  );

  // Mark as completed (Claude Code doesn't provide success/failure status)
  log?.start("updateAgentStatus");
  setAgentState(cwd, agent_id, "status", "completed");
  setAgentState(cwd, agent_id, "endedAt", new Date().toISOString());
  log?.end("updateAgentStatus");

  // Mark worktree as agent-complete if this agent had one (swarm mode)
  // The worktree stays for merge review - cleanup happens after worktree-resolve
  log?.start("checkWorktreeComplete");
  const worktreeMarked = markWorktreeComplete(cwd, agent_id);
  if (worktreeMarked) {
    log?.info(
      `Marked worktree as agent-complete for ${agent_id} - ready for merge review`,
    );
  }
  log?.end("checkWorktreeComplete", { worktreeMarked });

  // Refresh HUD to remove the completed agent
  log?.start("refreshHud");
  refreshHud(cwd, session_id);
  log?.end("refreshHud");

  log?.debug(`SubagentStop: agent ${agent_id} marked as completed`);
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

    // Initialize logger
    log = new Logger("subagent-tracker", cwd);
    log.start("total");
    log.info(`Received event: ${data.hook_event_name}`);
    log.debug(`Full input: ${JSON.stringify(data, null, 2)}`);

    let additionalContext: string | undefined;

    // Dispatch based on event type from input (Claude Code uses hook_event_name)
    if (data.hook_event_name === "SubagentStart") {
      additionalContext = await processSubagentStart(
        data as SubagentStartInput,
      );
    } else if (data.hook_event_name === "SubagentStop") {
      await processSubagentStop(data as SubagentStopInput);
    } else {
      // TypeScript narrows to never here, but handle unexpected events gracefully
      log.warn(
        `Unknown event type: ${(data as { hook_event_name?: string }).hook_event_name || "undefined"}`,
      );
    }

    log.end("total");
    log.flush();

    // Output with optional context injection for subagents
    const output: HookOutput = { continue: true };
    if (additionalContext) {
      output.hookSpecificOutput = {
        hookEventName: "SubagentStart",
        additionalContext,
      };
    }

    console.log(JSON.stringify(output));
  } catch (error) {
    // Log error but don't block
    if (log) {
      log.error("Subagent tracker failed", error);
      log.flush();
    }
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
