/**
 * Install command — registers aide plugin and MCP server in OpenCode config.
 */

import {
  addAideToConfig,
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
  writeConfig,
  PLUGIN_NAME,
} from "./config.js";

export interface InstallFlags {
  project?: boolean;
  noMcp?: boolean;
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

  if (before.plugin && before.mcp) {
    console.log(`aide is already configured in ${configPath}`);
    console.log("  plugin: registered");
    console.log("  mcp:    registered");
    console.log("\nNothing to do.");
    return;
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
    if (!before.mcp && after.mcp) {
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
