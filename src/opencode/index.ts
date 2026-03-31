/**
 * aide OpenCode plugin entry point.
 *
 * Provides first-class integration with OpenCode (opencode.ai),
 * reusing the same core logic as the Claude Code hooks.
 *
 * Usage:
 *   - As a local plugin: copy/symlink to .opencode/plugins/
 *   - As an npm package: add "@jmylchreest/aide-plugin" to opencode.json
 *
 * Environment variables:
 *   AIDE_FORCE_INIT=1  Force aide initialization even in non-git directories.
 *                      By default, aide skips initialization when no .git/ or
 *                      .aide/ directory is found. Set this in the MCP server's
 *                      "environment" config to always create .aide/ in the
 *                      working directory.
 *
 * @example opencode.json
 * ```json
 * {
 *   "$schema": "https://opencode.ai/config.json",
 *   "plugin": ["@jmylchreest/aide-plugin"],
 *   "mcp": {
 *     "aide": {
 *       "type": "local",
 *       "command": ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"],
 *       "environment": {
 *         "AIDE_CODE_WATCH": "1",
 *         "AIDE_CODE_WATCH_DELAY": "30s",
 *         "AIDE_FORCE_INIT": "1"
 *       },
 *       "enabled": true
 *     }
 *   }
 * }
 * ```
 */

import { basename, dirname, join, resolve } from "path";
import { fileURLToPath } from "url";
import { existsSync, readFileSync, statSync } from "fs";
import { createHooks } from "./hooks.js";
import { isDebugEnabled } from "../lib/logger.js";
import type { Plugin, PluginInput, Hooks } from "./types.js";

// Resolve the plugin package root so we can find bundled skills.
// Works whether running from source (repo) or installed via npm.
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
// index.ts lives in src/opencode/, so the package root is two levels up.
const pluginRoot = join(__dirname, "..", "..");

if (!process.env.AIDE_PLUGIN_ROOT) {
  process.env.AIDE_PLUGIN_ROOT = pluginRoot;
}

/**
 * Resolve the project root directory from the OpenCode plugin context.
 *
 * OpenCode provides three directory-related values:
 *   ctx.worktree  — `git rev-parse --show-toplevel` (sandbox root, "/" for non-git)
 *   ctx.directory — `process.cwd()` or `--dir` (where OpenCode was invoked)
 *   ctx.project.worktree — `dirname(git rev-parse --git-common-dir)` (main repo root)
 *
 * Priority:
 *   1. ctx.project.worktree — the main repo root (shared across all worktrees)
 *   2. ctx.worktree — the git sandbox root (fallback if project.worktree missing)
 *   3. ctx.directory — where OpenCode was invoked (fallback for non-git)
 *
 * IMPORTANT: We use ctx.project.worktree (git common dir parent) rather than
 * ctx.worktree (show-toplevel) so that all git worktrees resolve to the SAME
 * main repository root. This matches the Go binary's findProjectRoot() which
 * follows .git worktree pointers to the main repo. Without this, each worktree
 * would create its own .aide/ directory while the Go binary opens the database
 * in the main repo's .aide/, causing BoltDB/SQLite lock contention.
 *
 * Both ctx.worktree and ctx.directory are "/" for non-git projects, so we
 * detect that case and skip initialization.
 */
function resolveProjectRoot(ctx: PluginInput): {
  root: string;
  hasProjectRoot: boolean;
} {
  const directory = ctx.directory;

  // Prefer ctx.project.worktree — this is dirname(git rev-parse --git-common-dir),
  // i.e. the main repo root that is shared across all worktrees.
  // This matches the Go binary's resolveWorktreeRoot() behavior.
  const projectWorktree = ctx.project?.worktree;
  if (projectWorktree && projectWorktree !== "/") {
    return { root: resolve(projectWorktree), hasProjectRoot: true };
  }

  // Fallback: ctx.worktree is git rev-parse --show-toplevel (per-worktree root).
  // For normal (non-worktree) repos, this equals the main repo root anyway.
  // For non-git projects, OpenCode sets this to "/".
  const worktree = ctx.worktree;
  const isNonGitWorktree = !worktree || worktree === "/";

  if (!isNonGitWorktree) {
    return { root: resolve(worktree), hasProjectRoot: true };
  }

  // Worktree is "/" (non-git) — try to find a project root from the directory.
  // This handles the case where the user is in a git repo but OpenCode's
  // resolution somehow failed, or where .aide/ already exists.
  if (directory && directory !== "/") {
    const resolved = walkUpForProjectRoot(directory);
    if (resolved) {
      return { root: resolved, hasProjectRoot: true };
    }
  }

  // No git root found anywhere.
  // If AIDE_FORCE_INIT is set, treat the directory as the project root anyway.
  const forceInit = !!process.env.AIDE_FORCE_INIT;
  if (forceInit && directory && directory !== "/") {
    return { root: resolve(directory), hasProjectRoot: true };
  }

  return { root: directory || "/", hasProjectRoot: false };
}

/**
 * Walk up from `startDir` looking for .aide/ or .git/ directories.
 * Returns the project root path, or null if none found.
 *
 * For git worktrees, .git is a file containing "gitdir: <path>".
 * We follow it to the main repo root, matching the Go binary's
 * resolveWorktreeRoot() behavior.
 */
function walkUpForProjectRoot(startDir: string): string | null {
  let dir = resolve(startDir);
  for (;;) {
    if (existsSync(join(dir, ".aide"))) {
      return dir;
    }
    const gitPath = join(dir, ".git");
    if (existsSync(gitPath)) {
      try {
        const stat = statSync(gitPath);
        if (stat.isDirectory()) {
          // Normal git repo
          return dir;
        }
        if (stat.isFile()) {
          // Worktree: .git is a file containing "gitdir: <path>"
          // Follow it to the main repo root.
          const mainRoot = resolveWorktreeGitFile(gitPath);
          if (mainRoot) return mainRoot;
          // Fallback to current dir if resolution fails
          return dir;
        }
      } catch {
        return dir;
      }
    }
    const parent = resolve(dir, "..");
    if (parent === dir) {
      return null;
    }
    dir = parent;
  }
}

/**
 * Read a .git worktree file and resolve to the main repository root.
 * Mirrors the Go binary's resolveWorktreeRoot() in main.go.
 *
 * The file contains "gitdir: /path/to/repo/.git/worktrees/<name>".
 * We walk up from that gitdir path to find the .git directory,
 * then return its parent.
 */
function resolveWorktreeGitFile(gitFilePath: string): string | null {
  try {
    const content = readFileSync(gitFilePath, "utf-8").trim();
    if (!content.startsWith("gitdir:")) return null;

    let gitdir = content.slice("gitdir:".length).trim();
    // Make absolute if relative
    if (!gitdir.startsWith("/")) {
      gitdir = resolve(dirname(gitFilePath), gitdir);
    }

    // Walk up from .git/worktrees/<name> to find the .git directory,
    // then return its parent as the repo root.
    let candidate = gitdir;
    for (;;) {
      const parent = dirname(candidate);
      if (parent === candidate) break;
      if (basename(candidate) === ".git") {
        return parent;
      }
      candidate = parent;
    }
    return null;
  } catch {
    return null;
  }
}

export const AidePlugin: Plugin = async (ctx: PluginInput): Promise<Hooks> => {
  // Log raw plugin input BEFORE any resolution for diagnostics.
  // This is the key to understanding what OpenCode actually passes.
  const rawLog = [
    `aide plugin init (raw ctx):`,
    `  ctx.directory = ${JSON.stringify(ctx.directory)}`,
    `  ctx.worktree  = ${JSON.stringify(ctx.worktree)}`,
    `  ctx.project   = ${JSON.stringify(ctx.project, null, 2)?.split("\n").join("\n  ")}`,
  ].join("\n");

  // Best-effort log to OpenCode's log system
  try {
    await ctx.client.app.log({
      body: { service: "aide", level: "info", message: rawLog },
    });
  } catch {
    // Plugin may be called before the client is fully ready
  }

  // Also log to stderr when debugging
  if (isDebugEnabled()) console.error(rawLog);

  const resolved = resolveProjectRoot(ctx);

  const forceInit = !!process.env.AIDE_FORCE_INIT;
  const resolvedLog = `aide plugin resolved: root=${resolved.root} hasProjectRoot=${resolved.hasProjectRoot}${forceInit ? " (AIDE_FORCE_INIT=1)" : ""}`;
  try {
    await ctx.client.app.log({
      body: { service: "aide", level: "info", message: resolvedLog },
    });
  } catch {
    // non-fatal
  }
  if (isDebugEnabled()) console.error(resolvedLog);

  return createHooks(resolved.root, ctx.worktree, ctx.client, pluginRoot, {
    skipInit: !resolved.hasProjectRoot,
  });
};

export default AidePlugin;

// Re-export types for consumers
export type { Plugin, PluginInput, Hooks } from "./types.js";
