/**
 * @aide/opencode-plugin — standalone npm package entry point.
 *
 * Re-exports the OpenCode plugin from the main aide source.
 * When built as a standalone package, this provides:
 *   - AidePlugin: The main plugin function
 *   - Types for consumers
 */

export { AidePlugin, default } from "../../src/opencode/index.js";
export type { Plugin, PluginInput, Hooks } from "../../src/opencode/types.js";
