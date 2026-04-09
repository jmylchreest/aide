/**
 * Uninstall command — removes aide plugin and MCP server from OpenCode or Codex CLI config.
 */

import {
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
  removeAideFromConfig,
  writeConfig,
} from "./config.js";
import { uninstallCodex, isCodexConfigured } from "./codex-config.js";

export interface UninstallFlags {
  project?: boolean;
  platform?: "opencode" | "codex";
}

async function uninstallOpenCode(flags: UninstallFlags): Promise<void> {
  const configPath = flags.project
    ? getProjectConfigPath()
    : getGlobalConfigPath();

  const scope = flags.project ? "project" : "global";
  console.log(`Uninstalling aide plugin from OpenCode (${scope})...\n`);

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
  if (before.plugin) console.log(`  - Removed aide plugin from plugin array`);
  if (before.mcp) console.log(`  - Removed aide MCP server`);
  console.log("\nUninstallation complete.");
}

async function uninstallFromCodex(flags: UninstallFlags): Promise<void> {
  const scope = flags.project ? "project" : "user";
  console.log(`Uninstalling aide from Codex CLI (${scope})...\n`);

  const before = isCodexConfigured(scope);
  if (!before.mcp && !before.hooks) {
    console.log("aide is not configured for Codex CLI.");
    console.log("\nNothing to do.");
    return;
  }

  const result = uninstallCodex(scope);
  if (result.configRemoved) console.log("  - Removed aide MCP server from config.toml");
  if (result.hooksRemoved) console.log("  - Removed aide hooks from hooks.json");
  console.log("\nUninstallation complete.");
}

export async function uninstall(flags: UninstallFlags): Promise<void> {
  if (flags.platform === "codex") {
    await uninstallFromCodex(flags);
  } else {
    await uninstallOpenCode(flags);
  }
}
