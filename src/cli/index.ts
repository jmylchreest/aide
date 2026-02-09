#!/usr/bin/env node
/**
 * aide CLI — install/uninstall the aide plugin for OpenCode.
 *
 * Usage:
 *   bunx @jmylchreest/aide-plugin install   # Install globally for OpenCode
 *   bunx @jmylchreest/aide-plugin uninstall  # Remove from OpenCode config
 *   bunx @jmylchreest/aide-plugin status     # Show current installation status
 */

import { install } from "./install.js";
import { uninstall } from "./uninstall.js";
import { status } from "./status.js";

const args = process.argv.slice(2);
const command = args[0];

function printUsage(): void {
  console.log(`aide - AI Development Environment plugin for OpenCode

Usage:
  aide-plugin install     Install aide plugin globally for OpenCode
  aide-plugin uninstall   Remove aide plugin from OpenCode config
  aide-plugin status      Show current installation status
  aide-plugin --help      Show this help message

Options:
  --project    Apply to project-level opencode.json instead of global
  --no-mcp     Skip MCP server registration (plugin only)

Examples:
  bunx @jmylchreest/aide-plugin install
  npx @jmylchreest/aide-plugin install
  aide-plugin install --project`);
}

async function main(): Promise<void> {
  const flags = {
    project: args.includes("--project"),
    noMcp: args.includes("--no-mcp"),
  };

  switch (command) {
    case "install":
      await install(flags);
      break;
    case "uninstall":
      await uninstall(flags);
      break;
    case "status":
      await status();
      break;
    case "--help":
    case "-h":
    case "help":
      printUsage();
      break;
    default:
      if (command) {
        console.error(`Unknown command: ${command}\n`);
      }
      printUsage();
      process.exit(command ? 1 : 0);
  }
}

main().catch((err) => {
  console.error(`Error: ${err instanceof Error ? err.message : String(err)}`);
  process.exit(1);
});
