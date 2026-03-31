/**
 * MCP subcommand — delegates to aide-wrapper.ts to start the MCP server.
 *
 * This is the entry point used by OpenCode's MCP config:
 *   "command": ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"]
 *
 * The wrapper handles binary discovery/download, then exec's `aide mcp`.
 */

import spawn from "cross-spawn";
import { existsSync } from "fs";
import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";

/**
 * Find the aide-wrapper.ts script relative to this CLI module.
 *
 * Resolution: this file lives at <plugin-root>/src/cli/mcp.ts,
 * so the wrapper is at <plugin-root>/bin/aide-wrapper.ts.
 */
function findWrapper(): string {
  // __dirname equivalent for ESM
  const thisDir = dirname(fileURLToPath(import.meta.url));
  // src/cli -> src -> plugin-root
  const pluginRoot = resolve(thisDir, "..", "..");
  const wrapper = join(pluginRoot, "bin", "aide-wrapper.ts");

  if (!existsSync(wrapper)) {
    throw new Error(
      `aide-wrapper.ts not found at ${wrapper}\n` +
        `Expected plugin root: ${pluginRoot}\n` +
        `Ensure the package is installed correctly.`,
    );
  }

  return wrapper;
}

/**
 * Start the MCP server by exec'ing aide-wrapper.ts with "mcp" + any extra args.
 * This replaces the current process so stdio is inherited directly.
 */
export async function mcp(extraArgs: string[]): Promise<void> {
  const wrapper = findWrapper();
  const args = [wrapper, "mcp", ...extraArgs];

  // Use bun (via cross-spawn for Windows .cmd resolution) to run the
  // TypeScript wrapper. stdio is inherited so the MCP JSON-RPC protocol
  // flows directly between OpenCode and the aide binary.
  const result = spawn.sync("bun", args, {
    stdio: "inherit",
    env: process.env,
  });

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}
