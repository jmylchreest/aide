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
 *       "enabled": true
 *     }
 *   }
 * }
 * ```
 */

import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { createHooks } from "./hooks.js";
import { isDebugEnabled } from "../lib/logger.js";
import { resolveProjectRoot } from "./resolve-root.js";
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
