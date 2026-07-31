/**
 * Status command — shows current aide installation status for OpenCode or Codex CLI.
 */

import { existsSync } from "fs";
import {
  configuredPluginVersion,
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  isMcpCommandCurrent,
  readConfig,
} from "./config.js";
import {
  isCodexConfigured,
  getCodexConfigTomlPath,
  getCodexHooksJsonPath,
} from "./codex-config.js";
import { getRunningPluginVersion } from "../lib/plugin-version.js";

export interface StatusFlags {
  platform?: "opencode" | "codex";
}

/**
 * Describe the plugin pin, flagging anything that leaves OpenCode stuck on
 * an old version: no pin at all (OpenCode caches the first install forever)
 * or a pin behind the package this command runs from.
 */
function describePin(pinned: string | null, running: string | null): string {
  if (pinned === null) return "not found";
  if (pinned === "") return "registered (unpinned — run install to pin a version)";
  if (running && pinned !== running) {
    return `v${pinned} (stale — this package is v${running}, run install to re-pin)`;
  }
  return `v${pinned}`;
}

function showOpenCodeConfig(path: string, running: string | null): void {
  if (!existsSync(path)) {
    console.log("  (file does not exist)");
    return;
  }
  const config = readConfig(path);
  const s = isAideConfigured(config);
  console.log(`  plugin: ${describePin(configuredPluginVersion(config), running)}`);
  if (!s.mcp) {
    console.log("  mcp:    not found");
  } else if (isMcpCommandCurrent(config, running)) {
    console.log("  mcp:    registered");
  } else {
    console.log("  mcp:    registered (command out of date — run install)");
  }
}

function showOpenCodeStatus(): void {
  console.log("aide plugin status (OpenCode)\n");

  const running = getRunningPluginVersion();
  const globalPath = getGlobalConfigPath();
  const projectPath = getProjectConfigPath();

  console.log(`Global config: ${globalPath}`);
  showOpenCodeConfig(globalPath, running);

  console.log();

  console.log(`Project config: ${projectPath}`);
  showOpenCodeConfig(projectPath, running);
}

function showCodexStatus(): void {
  console.log("aide plugin status (Codex CLI)\n");

  const userConfig = getCodexConfigTomlPath("user");
  const userHooks = getCodexHooksJsonPath("user");
  const userStatus = isCodexConfigured("user");

  console.log(`User config:  ${userConfig}`);
  console.log(`User hooks:   ${userHooks}`);
  console.log(`  mcp:   ${userStatus.mcp ? "registered" : "not found"}`);
  console.log(`  hooks: ${userStatus.hooks ? "registered" : "not found"}`);

  console.log();

  const projectConfig = getCodexConfigTomlPath("project");
  const projectHooks = getCodexHooksJsonPath("project");
  const projectStatus = isCodexConfigured("project");

  console.log(`Project config: ${projectConfig}`);
  console.log(`Project hooks:  ${projectHooks}`);
  console.log(`  mcp:   ${projectStatus.mcp ? "registered" : "not found"}`);
  console.log(`  hooks: ${projectStatus.hooks ? "registered" : "not found"}`);
}

export async function status(flags?: StatusFlags): Promise<void> {
  if (flags?.platform === "codex") {
    showCodexStatus();
  } else {
    showOpenCodeStatus();
  }
}
