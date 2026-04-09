#!/usr/bin/env bun
/**
 * aide CLI — install/uninstall the aide plugin for OpenCode and Codex CLI.
 *
 * Usage:
 *   aide-plugin install [--platform codex|opencode]   # Install for detected platform
 *   aide-plugin uninstall [--platform codex|opencode]  # Remove from platform config
 *   aide-plugin status [--platform codex|opencode]     # Show installation status
 *   aide-plugin mcp                                    # Start MCP server
 *   aide-plugin hook <name>                            # Dispatch hook (Codex CLI)
 */

import { install } from "./install.js";
import { uninstall } from "./uninstall.js";
import { status } from "./status.js";
import { mcp } from "./mcp.js";
import { dispatchHook, listHooks } from "./hook.js";

const args = process.argv.slice(2);
const command = args[0];

type Platform = "opencode" | "codex";

function detectPlatform(): Platform {
  const idx = args.indexOf("--platform");
  if (idx !== -1 && args[idx + 1]) {
    const val = args[idx + 1] as string;
    if (val === "codex" || val === "opencode") return val;
    console.error(`Unknown platform: ${val}. Use "opencode" or "codex".`);
    process.exit(1);
  }

  if (process.env.CODEX_HOME || process.env.CODEX_SANDBOX_TYPE) return "codex";

  return "opencode";
}

function printUsage(): void {
  console.log(`aide - AI Development Environment plugin for OpenCode and Codex CLI

Usage:
  aide-plugin install     Install aide plugin (auto-detects platform)
  aide-plugin uninstall   Remove aide plugin from platform config
  aide-plugin status      Show current installation status
  aide-plugin mcp         Start MCP server (delegates to aide-wrapper)
  aide-plugin hook <name> Dispatch a hook by name (used by Codex hooks.json)
  aide-plugin --help      Show this help message

Options:
  --platform codex|opencode   Target platform (auto-detected if omitted)
  --project                   Apply to project-level config instead of global
  --no-mcp                    Skip MCP server registration (plugin only)

Available hooks:
  ${listHooks().join(", ")}

Examples:
  bunx @jmylchreest/aide-plugin install
  aide-plugin install --platform codex
  aide-plugin install --project
  aide-plugin hook session-start`);
}

async function main(): Promise<void> {
  const flags = {
    project: args.includes("--project"),
    noMcp: args.includes("--no-mcp"),
    platform: detectPlatform(),
  };

  switch (command) {
    case "install":
      await install(flags);
      break;
    case "uninstall":
      await uninstall(flags);
      break;
    case "status":
      await status(flags);
      break;
    case "mcp":
      await mcp(args.slice(1));
      break;
    case "hook": {
      const hookName = args[1];
      if (!hookName) {
        console.error(
          `Missing hook name.\nAvailable hooks: ${listHooks().join(", ")}`,
        );
        process.exit(1);
      }
      await dispatchHook(hookName);
      break;
    }
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
