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
 *       "command": ["npx", "-y", "@jmylchreest/aide-plugin", "mcp"],
 *       "environment": { "AIDE_CODE_WATCH": "1", "AIDE_CODE_WATCH_DELAY": "30s" },
 *       "enabled": true
 *     }
 *   }
 * }
 * ```
 */

import { createHooks } from "./hooks.js";
import type { Plugin, PluginInput, Hooks } from "./types.js";

export const AidePlugin: Plugin = async (ctx: PluginInput): Promise<Hooks> => {
  const cwd = ctx.worktree || ctx.directory;
  return createHooks(cwd, ctx.worktree, ctx.client);
};

export default AidePlugin;

// Re-export types for consumers
export type { Plugin, PluginInput, Hooks } from "./types.js";
