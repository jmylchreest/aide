/**
 * Uninstall command — removes aide plugin and MCP server from OpenCode config.
 */

import {
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
  removeAideFromConfig,
  writeConfig,
} from "./config.js";

export interface UninstallFlags {
  project?: boolean;
}

export async function uninstall(flags: UninstallFlags): Promise<void> {
  const configPath = flags.project
    ? getProjectConfigPath()
    : getGlobalConfigPath();

  const scope = flags.project ? "project" : "global";
  console.log(`Uninstalling aide plugin (${scope})...\n`);

  const existing = readConfig(configPath);
  const before = isAideConfigured(existing);

  if (!before.plugin && !before.mcp) {
    console.log(`aide is not configured in ${configPath}`);
    console.log("\nNothing to do.");
    return;
  }

  const updated = removeAideFromConfig(existing);
  writeConfig(configPath, updated);

  console.log(`Updated: ${configPath}\n`);

  if (before.plugin) {
    console.log(`  - Removed aide plugin from plugin array`);
  }
  if (before.mcp) {
    console.log(`  - Removed aide MCP server`);
  }

  console.log("\nUninstallation complete.");
}
