/**
 * @jmylchreest/aide-plugin — standalone npm package entry point.
 *
 * This file exists for development reference only.
 * The published package uses dist/opencode/index.js directly
 * (copied from the root project build).
 *
 * When installed via npm, the package is self-contained:
 *   dist/opencode/  — plugin entry + hooks
 *   dist/core/      — shared aide logic
 *   dist/lib/       — binary downloader + utilities
 *   bin/            — aide-wrapper.sh (binary bootstrap)
 */

// This re-export is only valid in the monorepo context (development).
// The published package points main/exports directly at dist/opencode/index.js.
export { AidePlugin, default } from "../../src/opencode/index.js";
export type { Plugin, PluginInput, Hooks } from "../../src/opencode/types.js";
