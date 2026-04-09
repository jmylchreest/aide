/**
 * Status command — shows current aide installation status for OpenCode or Codex CLI.
 */

import { existsSync } from "fs";
import {
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
} from "./config.js";
import {
  isCodexConfigured,
  getCodexConfigTomlPath,
  getCodexHooksJsonPath,
} from "./codex-config.js";

export interface StatusFlags {
  platform?: "opencode" | "codex";
}

function showOpenCodeStatus(): void {
  console.log("aide plugin status (OpenCode)\n");

  const globalPath = getGlobalConfigPath();
  const projectPath = getProjectConfigPath();

  console.log(`Global config: ${globalPath}`);
  if (existsSync(globalPath)) {
    const s = isAideConfigured(readConfig(globalPath));
    console.log(`  plugin: ${s.plugin ? "registered" : "not found"}`);
    console.log(`  mcp:    ${s.mcp ? "registered" : "not found"}`);
  } else {
    console.log("  (file does not exist)");
  }

  console.log();

  console.log(`Project config: ${projectPath}`);
  if (existsSync(projectPath)) {
    const s = isAideConfigured(readConfig(projectPath));
    console.log(`  plugin: ${s.plugin ? "registered" : "not found"}`);
    console.log(`  mcp:    ${s.mcp ? "registered" : "not found"}`);
  } else {
    console.log("  (file does not exist)");
  }
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
