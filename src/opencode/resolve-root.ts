/**
 * Project-root resolution for the OpenCode plugin.
 *
 * Lives in its own module (NOT the plugin entry point): OpenCode's plugin
 * loader iterates every function export of the entry module and invokes it
 * as a plugin, so exporting helpers from index.ts would get them called
 * with PluginInput and their return values registered as hook objects.
 */

import { resolve } from "path";
import { findProjectRoot } from "../lib/project-root.js";
import type { PluginInput } from "./types.js";

/**
 * Resolve the project root directory from the OpenCode plugin context.
 *
 * OpenCode provides three directory-related values:
 *   ctx.worktree  — `git rev-parse --show-toplevel` (sandbox root, "/" for non-git)
 *   ctx.directory — `process.cwd()` or `--dir` (where OpenCode was invoked)
 *   ctx.project.worktree — `dirname(git rev-parse --git-common-dir)` (main repo root)
 *
 * Priority:
 *   1. findProjectRoot(ctx.directory) — the canonical resolver shared with the
 *      Go binary and every hook: honors AIDE_PROJECT_ROOT, walks up with
 *      nearest-VCS-root-wins, resolves linked worktrees to the main repo root,
 *      and anchors submodule checkouts at the submodule itself.
 *   2. ctx.project.worktree / ctx.worktree — OpenCode's own git resolution,
 *      used only when the canonical walk finds no marker (e.g. ctx.directory
 *      missing). For submodules these point at the SUPERPROJECT side, which
 *      is why they cannot be the primary source: preferring them pinned
 *      submodule sessions to the superproject while `aide mcp` (resolving
 *      from its own cwd) anchored the submodule — a hooks-vs-MCP split-brain.
 *   3. ctx.directory with AIDE_FORCE_INIT — non-git fallback.
 *
 * Both ctx.worktree and ctx.directory are "/" for non-git projects, so we
 * detect that case and skip initialization.
 */
export function resolveProjectRoot(ctx: PluginInput): {
  root: string;
  hasProjectRoot: boolean;
} {
  const directory = ctx.directory;

  // 1. Canonical resolver, seeded with where OpenCode was invoked. Same walk
  // as the Go binary and all hooks, so every half of the session agrees on
  // the root (worktrees share the main repo; submodules anchor themselves).
  if (directory && directory !== "/") {
    const resolved = findProjectRoot(directory);
    if (resolved.hasMarker) {
      return { root: resolved.root, hasProjectRoot: true };
    }
  }

  // 2. Fallback to OpenCode's own git resolution when the canonical walk
  // found no marker. project.worktree is dirname(git rev-parse
  // --git-common-dir), shared across worktrees; worktree is show-toplevel.
  const projectWorktree = ctx.project?.worktree;
  if (projectWorktree && projectWorktree !== "/") {
    return { root: resolve(projectWorktree), hasProjectRoot: true };
  }
  const worktree = ctx.worktree;
  if (worktree && worktree !== "/") {
    return { root: resolve(worktree), hasProjectRoot: true };
  }

  // 3. No git root found anywhere.
  // If AIDE_FORCE_INIT is set, treat the directory as the project root anyway.
  const forceInit = !!process.env.AIDE_FORCE_INIT;
  if (forceInit && directory && directory !== "/") {
    return { root: resolve(directory), hasProjectRoot: true };
  }

  return { root: directory || "/", hasProjectRoot: false };
}
