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

import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";
import { existsSync, statSync } from "fs";
import { createHooks } from "./hooks.js";
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
 *   1. ctx.worktree — the git sandbox root (correct for both normal repos and worktrees)
 *   2. ctx.directory — where OpenCode was invoked (fallback for non-git)
 *
 * Both ctx.worktree and ctx.directory are "/" for non-git projects, so we
 * detect that case and skip initialization.
 */
function resolveProjectRoot(ctx: PluginInput): {
  root: string;
  hasProjectRoot: boolean;
} {
  // ctx.worktree is the git working tree root (from `git rev-parse --show-toplevel`).
  // For non-git projects, OpenCode sets this to "/".
  const worktree = ctx.worktree;
  const directory = ctx.directory;

  // OpenCode sets worktree to "/" for non-git projects — treat as no project.
  // Also guard against empty strings.
  const isNonGitWorktree = !worktree || worktree === "/";

  if (!isNonGitWorktree) {
    // The worktree is a valid git root — use it directly.
    // No need to walk up the filesystem; OpenCode already resolved it.
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
        // .git can be a directory (normal repo) or a file (worktree pointer)
        if (stat.isDirectory() || stat.isFile()) {
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

  // Also log to stderr for direct observability
  console.error(rawLog);

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
  console.error(resolvedLog);

  return createHooks(resolved.root, ctx.worktree, ctx.client, pluginRoot, {
    skipInit: !resolved.hasProjectRoot,
  });
};

export default AidePlugin;

// Re-export types for consumers
export type { Plugin, PluginInput, Hooks } from "./types.js";
