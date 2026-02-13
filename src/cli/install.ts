/**
 * Install command — registers aide plugin and MCP server in OpenCode config.
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

export interface InstallFlags {
  project?: boolean;
  noMcp?: boolean;
}

/**
 * Check if an existing MCP config has the current expected command format.
 * Returns false if the command is missing, empty, or uses an outdated format.
 */
function isMcpCommandCurrent(config: ReturnType<typeof readConfig>): boolean {
  const mcpConfig = config.mcp?.[MCP_SERVER_NAME];
  if (!mcpConfig?.command || mcpConfig.command.length === 0) {
    return false;
  }

  const cmd = mcpConfig.command;

  // Current format: ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"]
  if (
    cmd.length === 4 &&
    cmd[0] === "bunx" &&
    cmd[1] === "-y" &&
    cmd[2] === PLUGIN_NAME &&
    cmd[3] === "mcp"
  ) {
    return true;
  }

  return false;
}

export async function install(flags: InstallFlags): Promise<void> {
  const configPath = flags.project
    ? getProjectConfigPath()
    : getGlobalConfigPath();

  const scope = flags.project ? "project" : "global";
  console.log(`Installing aide plugin (${scope})...\n`);

  // Read existing config
  const existing = readConfig(configPath);
  const before = isAideConfigured(existing);

  // Check if MCP command needs updating (stale format from older versions)
  const mcpNeedsUpdate =
    !flags.noMcp && before.mcp && !isMcpCommandCurrent(existing);

  if (before.plugin && before.mcp && !mcpNeedsUpdate) {
    console.log(`aide is already configured in ${configPath}`);
    console.log("  plugin: registered");
    console.log("  mcp:    registered");
    console.log("\nNothing to do.");
    return;
  }

  // If MCP config is stale, remove it so addAideToConfig will write the current version
  if (mcpNeedsUpdate && existing.mcp) {
    delete existing.mcp[MCP_SERVER_NAME];
  }

  // Apply changes
  const updated = addAideToConfig(existing, { noMcp: flags.noMcp });
  writeConfig(configPath, updated);

  // Report what was done
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
