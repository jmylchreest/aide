/**
 * Git Worktree Manager
 *
 * Manages git worktrees for parallel agent execution in swarm mode.
 * Each agent gets its own worktree to avoid file conflicts.
 */

import { execSync } from 'child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'fs';
import { join } from 'path';

export interface Worktree {
  name: string;
  path: string;
  branch: string;
  taskId?: string;
  agentId?: string;
  createdAt: string;
}

export interface WorktreeState {
  active: Worktree[];
  baseBranch: string;
}

const WORKTREE_DIR = '.aide/worktrees';
const STATE_FILE = '.aide/state/worktrees.json';

/**
 * Load worktree state
 */
export function loadWorktreeState(cwd: string): WorktreeState {
  const statePath = join(cwd, STATE_FILE);
  if (existsSync(statePath)) {
    try {
      return JSON.parse(readFileSync(statePath, 'utf-8'));
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
  const stateDir = join(cwd, '.aide', 'state');
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
    return execSync('git rev-parse --abbrev-ref HEAD', { cwd, encoding: 'utf-8' }).trim();
  } catch {
    return 'main';
  }
}

/**
 * Check if we're in a git repository
 */
export function isGitRepo(cwd: string): boolean {
  try {
    execSync('git rev-parse --git-dir', { cwd, stdio: 'ignore' });
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
  agentId: string
): Worktree | null {
  if (!isGitRepo(cwd)) {
    console.error('Not a git repository');
    return null;
  }

  const state = loadWorktreeState(cwd);
  const worktreeDir = join(cwd, WORKTREE_DIR);

  // Ensure worktree directory exists
  if (!existsSync(worktreeDir)) {
    mkdirSync(worktreeDir, { recursive: true });
  }

  // Generate unique names
  // Use feat/<task>-<agent> branch naming for cleaner git history
  const name = `${taskId.slice(0, 8)}-${agentId}`;
  const branch = `feat/${name}`;
  const worktreePath = join(worktreeDir, name);

  // Check if worktree already exists
  if (existsSync(worktreePath)) {
    const existing = state.active.find(w => w.path === worktreePath);
    if (existing) {
      return existing;
    }
  }

  try {
    // Create branch and worktree
    execSync(`git worktree add -b "${branch}" "${worktreePath}" HEAD`, {
      cwd,
      stdio: 'pipe',
    });

    const worktree: Worktree = {
      name,
      path: worktreePath,
      branch,
      taskId,
      agentId,
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
  const worktree = state.active.find(w => w.name === name);

  if (!worktree) {
    console.error(`Worktree not found: ${name}`);
    return false;
  }

  try {
    // Remove worktree
    execSync(`git worktree remove "${worktree.path}" --force`, {
      cwd,
      stdio: 'pipe',
    });

    // Delete branch
    execSync(`git branch -D "${worktree.branch}"`, {
      cwd,
      stdio: 'pipe',
    });

    // Update state
    state.active = state.active.filter(w => w.name !== name);
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
  const worktree = state.active.find(w => w.name === name);

  if (!worktree) {
    console.error(`Worktree not found: ${name}`);
    return false;
  }

  try {
    // Checkout base branch
    execSync(`git checkout "${state.baseBranch}"`, { cwd, stdio: 'pipe' });

    // Merge worktree branch
    execSync(`git merge "${worktree.branch}" --no-edit`, { cwd, stdio: 'pipe' });

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
    execSync('git worktree prune', { cwd, stdio: 'pipe' });
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
export function getWorktreeForTask(cwd: string, taskId: string): Worktree | undefined {
  const state = loadWorktreeState(cwd);
  return state.active.find(w => w.taskId === taskId);
}

/**
 * Execute command in a worktree
 */
export function execInWorktree(
  cwd: string,
  name: string,
  command: string
): string | null {
  const state = loadWorktreeState(cwd);
  const worktree = state.active.find(w => w.name === name);

  if (!worktree) {
    console.error(`Worktree not found: ${name}`);
    return null;
  }

  try {
    return execSync(command, {
      cwd: worktree.path,
      encoding: 'utf-8',
    });
  } catch (error) {
    console.error(`Command failed in worktree: ${error}`);
    return null;
  }
}
