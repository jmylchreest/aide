/**
 * Status command — shows current aide installation status for OpenCode.
 */

import { existsSync } from "fs";
import {
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  readConfig,
} from "./config.js";

export async function status(): Promise<void> {
  console.log("aide plugin status\n");

  const globalPath = getGlobalConfigPath();
  const projectPath = getProjectConfigPath();

  // Global config
  console.log(`Global config: ${globalPath}`);
  if (existsSync(globalPath)) {
    const globalConfig = readConfig(globalPath);
    const globalStatus = isAideConfigured(globalConfig);
    console.log(
      `  plugin: ${globalStatus.plugin ? "registered" : "not found"}`,
    );
    console.log(`  mcp:    ${globalStatus.mcp ? "registered" : "not found"}`);
  } else {
    console.log("  (file does not exist)");
  }

  console.log();

  // Project config
  console.log(`Project config: ${projectPath}`);
  if (existsSync(projectPath)) {
    const projectConfig = readConfig(projectPath);
    const projectStatus = isAideConfigured(projectConfig);
    console.log(
      `  plugin: ${projectStatus.plugin ? "registered" : "not found"}`,
    );
    console.log(`  mcp:    ${projectStatus.mcp ? "registered" : "not found"}`);
  } else {
    console.log("  (file does not exist)");
  }
}
