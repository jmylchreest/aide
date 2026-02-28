/**
 * Git Worktree Manager
 *
 * STATUS: UTILITY LIBRARY - Not yet integrated into hooks
 *
 * This library manages git worktrees for parallel agent execution in swarm mode.
 * Each agent gets its own worktree to avoid file conflicts when multiple agents
 * work on different tasks simultaneously.
 *
 * Currently, worktree management is handled by the aide Go binary directly,
 * called via the CLI from swarm mode orchestration. This TypeScript library
 * provides an alternative implementation for hooks or plugins that need
 * worktree management without the aide binary.
 *
 * Future integration:
 * - swarm skill could use this for TypeScript-native worktree management
 * - Automated cleanup of stale worktrees on session start
 * - Integration with subagent-tracker for per-agent worktree assignment
 *
 * The Go implementation is currently preferred because:
 * 1. It integrates with aide's task and memory systems
 * 2. Git operations are faster from native code
 * 3. Error handling and edge cases are better tested
 */

import { execFileSync } from "child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
  readdirSync,
  statSync,
} from "fs";
import { join } from "path";

export type WorktreeStatus = "active" | "agent-complete" | "merged";

export interface Worktree {
  name: string;
  path: string;
  branch: string;
  taskId?: string;
  agentId?: string;
  status: WorktreeStatus;
  createdAt: string;
  completedAt?: string;
}

export interface WorktreeState {
  active: Worktree[];
  baseBranch: string;
}

const WORKTREE_DIR = ".aide/worktrees";
const STATE_FILE = ".aide/state/worktrees.json";

/**
 * Validate and sanitize an ID (taskId, agentId, branch name)
 * Only allows alphanumeric characters, hyphens, and underscores
 */
function sanitizeId(id: string): string {
  // Remove any characters that aren't alphanumeric, hyphens, or underscores
  return id.replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 64);
}

/**
 * Load worktree state
 */
export function loadWorktreeState(cwd: string): WorktreeState {
  const statePath = join(cwd, STATE_FILE);
  if (existsSync(statePath)) {
    try {
      return JSON.parse(readFileSync(statePath, "utf-8"));
    } catch {
      // Return default
    }
  }
  return {
    active: [],
    baseBranch: getCurrentBranch(cwd),
  };
}

/**
 * Save worktree state
 */
export function saveWorktreeState(cwd: string, state: WorktreeState): void {
  const stateDir = join(cwd, ".aide", "state");
  if (!existsSync(stateDir)) {
    mkdirSync(stateDir, { recursive: true });
  }
  writeFileSync(join(cwd, STATE_FILE), JSON.stringify(state, null, 2));
}

/**
 * Get current git branch
 */
export function getCurrentBranch(cwd: string): string {
  try {
    return execFileSync("git", ["rev-parse", "--abbrev-ref", "HEAD"], {
      cwd,
      encoding: "utf-8",
    }).trim();
  } catch {
    return "main";
  }
}

/**
 * Check if we're in a git repository
 */
export function isGitRepo(cwd: string): boolean {
  try {
    execFileSync("git", ["rev-parse", "--git-dir"], { cwd, stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

/**
 * Create a new worktree for an agent
 */
export function createWorktree(
  cwd: string,
  taskId: string,
  agentId: string,
): Worktree | null {
  if (!isGitRepo(cwd)) {
    console.error("Not a git repository");
    return null;
  }

  const state = loadWorktreeState(cwd);
  const worktreeDir = join(cwd, WORKTREE_DIR);

  // Ensure worktree directory exists
  if (!existsSync(worktreeDir)) {
    mkdirSync(worktreeDir, { recursive: true });
  }

  // Generate unique names with sanitized IDs
  // Use feat/<task>-<agent> branch naming for cleaner git history
  const safeTaskId = sanitizeId(taskId).slice(0, 8);
  const safeAgentId = sanitizeId(agentId);
  const name = `${safeTaskId}-${safeAgentId}`;
  const branch = `feat/${name}`;
  const worktreePath = join(worktreeDir, name);

  // Check if worktree already exists
  if (existsSync(worktreePath)) {
    const existing = state.active.find((w) => w.path === worktreePath);
    if (existing) {
      return existing;
    }
  }

  try {
    // Create branch and worktree using execFileSync with argument array
    execFileSync(
      "git",
      ["worktree", "add", "-b", branch, worktreePath, "HEAD"],
      {
        cwd,
        stdio: "pipe",
      },
    );

    const worktree: Worktree = {
      name,
      path: worktreePath,
      branch,
      taskId,
      agentId,
      status: "active",
      createdAt: new Date().toISOString(),
    };

    // Update state
    state.active.push(worktree);
    saveWorktreeState(cwd, state);

    return worktree;
  } catch (error) {
    console.error(`Failed to create worktree: ${error}`);
    return null;
  }
}

/**
 * Remove a worktree
 */
export function removeWorktree(cwd: string, name: string): boolean {
  const state = loadWorktreeState(cwd);
  const worktree = state.active.find((w) => w.name === name);

  if (!worktree) {
    console.error(`Worktree not found: ${name}`);
    return false;
  }

  try {
    // Remove worktree using execFileSync with argument array
    execFileSync("git", ["worktree", "remove", worktree.path, "--force"], {
      cwd,
      stdio: "pipe",
    });

    // Delete branch using execFileSync with argument array
    execFileSync("git", ["branch", "-D", worktree.branch], {
      cwd,
      stdio: "pipe",
    });

    // Update state
    state.active = state.active.filter((w) => w.name !== name);
    saveWorktreeState(cwd, state);

    return true;
  } catch (error) {
    console.error(`Failed to remove worktree: ${error}`);
    return false;
  }
}

/**
 * Merge worktree changes back to base branch
 */
export function mergeWorktree(cwd: string, name: string): boolean {
  const state = loadWorktreeState(cwd);
  const worktree = state.active.find((w) => w.name === name);

  if (!worktree) {
    console.error(`Worktree not found: ${name}`);
    return false;
  }

  try {
    // Checkout base branch - sanitize branch name just in case
    const safeBranch = sanitizeId(state.baseBranch);
    execFileSync("git", ["checkout", safeBranch], { cwd, stdio: "pipe" });

    // Merge worktree branch - branch name was sanitized at creation
    execFileSync("git", ["merge", worktree.branch, "--no-edit"], {
      cwd,
      stdio: "pipe",
    });

    return true;
  } catch (error) {
    console.error(`Failed to merge worktree: ${error}`);
    return false;
  }
}

/**
 * Cleanup all worktrees
 */
export function cleanupWorktrees(cwd: string): void {
  const state = loadWorktreeState(cwd);

  for (const worktree of [...state.active]) {
    removeWorktree(cwd, worktree.name);
  }

  // Prune any orphaned worktrees
  try {
    execFileSync("git", ["worktree", "prune"], { cwd, stdio: "pipe" });
  } catch {
    // Ignore errors
  }
}

/**
 * List active worktrees
 */
export function listWorktrees(cwd: string): Worktree[] {
  const state = loadWorktreeState(cwd);
  return state.active;
}

/**
 * Get worktree for a specific task
 */
export function getWorktreeForTask(
  cwd: string,
  taskId: string,
): Worktree | undefined {
  const state = loadWorktreeState(cwd);
  return state.active.find((w) => w.taskId === taskId);
}

/**
 * Get worktree for a specific agent
 */
export function getWorktreeForAgent(
  cwd: string,
  agentId: string,
): Worktree | undefined {
  const state = loadWorktreeState(cwd);
  return state.active.find((w) => w.agentId === agentId);
}

/**
 * Register an existing worktree that was created externally (e.g., via git CLI)
 * This allows the hooks to track worktrees created by the orchestrator
 */
export function registerWorktree(
  cwd: string,
  worktreePath: string,
  branch: string,
  storyId: string,
  agentId: string,
): Worktree | null {
  if (!existsSync(worktreePath)) {
    console.error(`Worktree path does not exist: ${worktreePath}`);
    return null;
  }

  const state = loadWorktreeState(cwd);

  // Check if already registered
  const existing = state.active.find((w) => w.path === worktreePath);
  if (existing) {
    // Update agentId if different (agent may have been assigned later)
    if (existing.agentId !== agentId) {
      existing.agentId = agentId;
      saveWorktreeState(cwd, state);
    }
    return existing;
  }

  // Extract name from path
  const name = worktreePath.split("/").pop() || storyId;

  const worktree: Worktree = {
    name,
    path: worktreePath,
    branch,
    taskId: storyId,
    agentId,
    status: "active",
    createdAt: new Date().toISOString(),
  };

  state.active.push(worktree);
  saveWorktreeState(cwd, state);

  return worktree;
}

/**
 * Auto-discover worktrees in the standard location that aren't registered
 * Scans .aide/worktrees/ for directories and registers them
 */
export function discoverWorktrees(cwd: string): Worktree[] {
  const worktreeDir = join(cwd, WORKTREE_DIR);
  if (!existsSync(worktreeDir)) {
    return [];
  }

  const state = loadWorktreeState(cwd);
  const discovered: Worktree[] = [];

  try {
    const entries = readdirSync(worktreeDir);

    for (const entry of entries) {
      const entryPath = join(worktreeDir, entry);
      if (!statSync(entryPath).isDirectory()) continue;

      // Skip if already registered
      if (state.active.find((w) => w.path === entryPath)) continue;

      // Try to get the branch name from the worktree
      try {
        const branch = execFileSync(
          "git",
          ["rev-parse", "--abbrev-ref", "HEAD"],
          { cwd: entryPath, encoding: "utf-8" },
        ).trim();

        const worktree: Worktree = {
          name: entry,
          path: entryPath,
          branch,
          taskId: entry, // Use directory name as story ID
          status: "active",
          createdAt: new Date().toISOString(),
        };

        state.active.push(worktree);
        discovered.push(worktree);
      } catch {
        // Not a valid git worktree, skip
      }
    }

    if (discovered.length > 0) {
      saveWorktreeState(cwd, state);
    }
  } catch {
    // Directory read failed
  }

  return discovered;
}

/**
 * Mark a worktree as agent-complete (ready for merge review)
 * Called when the subagent finishes its work
 */
export function markWorktreeComplete(cwd: string, agentId: string): boolean {
  const state = loadWorktreeState(cwd);
  const worktree = state.active.find((w) => w.agentId === agentId);

  if (!worktree) {
    return false;
  }

  worktree.status = "agent-complete";
  worktree.completedAt = new Date().toISOString();
  saveWorktreeState(cwd, state);

  return true;
}

/**
 * Mark a worktree as merged
 * Called after successful merge to main branch
 */
export function markWorktreeMerged(cwd: string, name: string): boolean {
  const state = loadWorktreeState(cwd);
  const worktree = state.active.find((w) => w.name === name);

  if (!worktree) {
    return false;
  }

  worktree.status = "merged";
  saveWorktreeState(cwd, state);

  return true;
}

/**
 * Get all worktrees ready for merge (status: agent-complete)
 */
export function getWorktreesReadyForMerge(cwd: string): Worktree[] {
  const state = loadWorktreeState(cwd);
  return state.active.filter((w) => w.status === "agent-complete");
}
