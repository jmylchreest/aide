/**
 * Install command — registers aide plugin and MCP server for OpenCode or Codex CLI.
 *
 * On reinstall, detects and upgrades stale MCP command configurations
 * (e.g. old `aide-wrapper` commands) to the current format.
 */

import {
  addAideToConfig,
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
  writeConfig,
  PLUGIN_NAME,
  MCP_SERVER_NAME,
} from "./config.js";
import { installCodex, isCodexConfigured } from "./codex-config.js";

export interface InstallFlags {
  project?: boolean;
  noMcp?: boolean;
  platform?: "opencode" | "codex";
}

function isMcpCommandCurrent(config: ReturnType<typeof readConfig>): boolean {
  const mcpConfig = config.mcp?.[MCP_SERVER_NAME];
  if (!mcpConfig?.command || mcpConfig.command.length === 0) return false;
  const cmd = mcpConfig.command;
  return (
    cmd.length === 4 &&
    cmd[0] === "bunx" &&
    cmd[1] === "-y" &&
    cmd[2] === PLUGIN_NAME &&
    cmd[3] === "mcp"
  );
}

async function installOpenCode(flags: InstallFlags): Promise<void> {
  const configPath = flags.project
    ? getProjectConfigPath()
    : getGlobalConfigPath();

  const scope = flags.project ? "project" : "global";
  console.log(`Installing aide plugin for OpenCode (${scope})...\n`);

  const existing = readConfig(configPath);
  const before = isAideConfigured(existing);
  const mcpNeedsUpdate =
    !flags.noMcp && before.mcp && !isMcpCommandCurrent(existing);

  if (before.plugin && before.mcp && !mcpNeedsUpdate) {
    console.log(`aide is already configured in ${configPath}`);
    console.log("  plugin: registered");
    console.log("  mcp:    registered");
    console.log("\nNothing to do.");
    return;
  }

  if (mcpNeedsUpdate && existing.mcp) {
    delete existing.mcp[MCP_SERVER_NAME];
  }

  const updated = addAideToConfig(existing, { noMcp: flags.noMcp });
  writeConfig(configPath, updated);

  const after = isAideConfigured(updated);
  console.log(`Updated: ${configPath}\n`);

  if (!before.plugin && after.plugin) {
    console.log(`  + Added "${PLUGIN_NAME}" to plugin array`);
  } else if (before.plugin) {
    console.log(`  = Plugin already registered`);
  }

  if (!flags.noMcp) {
    if (mcpNeedsUpdate) {
      console.log(`  ~ Updated "aide" MCP server command (was outdated)`);
    } else if (!before.mcp && after.mcp) {
      console.log(`  + Added "aide" MCP server`);
    } else if (before.mcp) {
      console.log(`  = MCP server already registered`);
    }
  } else {
    console.log(`  - Skipped MCP server registration (--no-mcp)`);
  }

  console.log("\nInstallation complete. Start OpenCode to use aide.");

  if (!flags.project) {
    console.log(
      "\nThe plugin is installed globally and will apply to all OpenCode projects.",
    );
  }
}

async function installForCodex(flags: InstallFlags): Promise<void> {
  const scope = flags.project ? "project" : "user";
  console.log(`Installing aide for Codex CLI (${scope})...\n`);

  const before = isCodexConfigured(scope);
  if (before.mcp && before.hooks) {
    console.log("aide is already configured for Codex CLI");
    console.log("  mcp:   registered in config.toml");
    console.log("  hooks: registered in hooks.json");
    console.log("\nNothing to do.");
    return;
  }

  const result = installCodex(scope);

  if (result.configWritten) {
    console.log("  + Added aide MCP server to config.toml");
  } else if (before.mcp) {
    console.log("  = MCP server already registered in config.toml");
  }

  if (result.hooksWritten) {
    console.log("  + Generated hooks.json with aide hooks");
  } else if (before.hooks) {
    console.log("  = Hooks already registered in hooks.json");
  }

  console.log("\nInstallation complete. Start Codex CLI to use aide.");

  if (!flags.project) {
    console.log(
      "\nThe plugin is installed globally and will apply to all Codex CLI sessions.",
    );
  }
}

export async function install(flags: InstallFlags): Promise<void> {
  if (flags.platform === "codex") {
    await installForCodex(flags);
  } else {
    await installOpenCode(flags);
  }
}
