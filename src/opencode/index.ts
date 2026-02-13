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
 * @example opencode.json
 * ```json
 * {
 *   "$schema": "https://opencode.ai/config.json",
 *   "plugin": ["@jmylchreest/aide-plugin"],
 *   "mcp": {
 *     "aide": {
 *       "type": "local",
 *       "command": ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"],
 *       "environment": { "AIDE_CODE_WATCH": "1", "AIDE_CODE_WATCH_DELAY": "30s" },
 *       "enabled": true
 *     }
 *   }
 * }
 * ```
 */

import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { createHooks } from "./hooks.js";
import type { Plugin, PluginInput, Hooks } from "./types.js";

// Resolve the plugin package root so we can find bundled skills.
// Works whether running from source (repo) or installed via npm.
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
// index.ts lives in src/opencode/, so the package root is two levels up.
const pluginRoot = join(__dirname, "..", "..");

export const AidePlugin: Plugin = async (ctx: PluginInput): Promise<Hooks> => {
  const cwd = ctx.worktree || ctx.directory;
  return createHooks(cwd, ctx.worktree, ctx.client, pluginRoot);
};

export default AidePlugin;

// Re-export types for consumers
export type { Plugin, PluginInput, Hooks } from "./types.js";
