/**
 * MCP subcommand — delegates to aide-wrapper.sh to start the MCP server.
 *
 * This is the entry point used by OpenCode's MCP config:
 *   "command": ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"]
 *
 * The wrapper handles binary discovery/download, then exec's `aide mcp`.
 */

import { execFileSync } from "child_process";
import { existsSync } from "fs";
import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";

/**
 * Find the aide-wrapper.sh script relative to this CLI module.
 *
 * Resolution: this file lives at <plugin-root>/src/cli/mcp.ts,
 * so the wrapper is at <plugin-root>/bin/aide-wrapper.sh.
 */
function findWrapper(): string {
  // __dirname equivalent for ESM
  const thisDir = dirname(fileURLToPath(import.meta.url));
  // src/cli -> src -> plugin-root
  const pluginRoot = resolve(thisDir, "..", "..");
  const wrapper = join(pluginRoot, "bin", "aide-wrapper.sh");

  if (!existsSync(wrapper)) {
    throw new Error(
      `aide-wrapper.sh not found at ${wrapper}\n` +
        `Expected plugin root: ${pluginRoot}\n` +
        `Ensure the package is installed correctly.`,
    );
  }

  return wrapper;
}

/**
 * Start the MCP server by exec'ing aide-wrapper.sh with "mcp" + any extra args.
 * This replaces the current process so stdio is inherited directly.
 */
export async function mcp(extraArgs: string[]): Promise<void> {
  const wrapper = findWrapper();
  const args = ["mcp", ...extraArgs];

  // Use execFileSync with stdio inherit so the MCP JSON-RPC protocol
  // flows directly between OpenCode and the aide binary.
  // The wrapper will exec() into the aide binary, replacing itself.
  try {
    execFileSync(wrapper, args, {
      stdio: "inherit",
      env: process.env,
    });
  } catch (err: unknown) {
    // execFileSync throws on non-zero exit. If the process was killed
    // by a signal (e.g. OpenCode shutting down), exit cleanly.
    if (err && typeof err === "object" && "status" in err) {
      process.exit((err as { status: number }).status ?? 1);
    }
    throw err;
  }
}
