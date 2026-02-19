/**
 * OpenCode config file utilities.
 *
 * Handles reading, merging, and writing opencode.json at both
 * global (~/.config/opencode/opencode.json) and project (./opencode.json) scopes.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { dirname, join } from "path";
import { homedir } from "os";

export interface McpServerConfig {
  type?: string;
  command?: string[];
  environment?: Record<string, string>;
  enabled?: boolean;
}

export interface OpenCodeConfig {
  $schema?: string;
  plugin?: string[];
  mcp?: Record<string, McpServerConfig>;
  [key: string]: unknown;
}

const PLUGIN_NAME = "@jmylchreest/aide-plugin";
const MCP_SERVER_NAME = "aide";
const SCHEMA_URL = "https://opencode.ai/config.json";

/**
 * Get the path to the global OpenCode config file.
 */
export function getGlobalConfigPath(): string {
  return join(homedir(), ".config", "opencode", "opencode.json");
}

/**
 * Get the path to the project-level OpenCode config file.
 */
export function getProjectConfigPath(): string {
  return join(process.cwd(), "opencode.json");
}

/**
 * Read and parse an opencode.json file. Returns an empty config if
 * the file doesn't exist or can't be parsed.
 */
export function readConfig(configPath: string): OpenCodeConfig {
  if (!existsSync(configPath)) {
    return {};
  }
  try {
    const raw = readFileSync(configPath, "utf-8");
    return JSON.parse(raw) as OpenCodeConfig;
  } catch {
    return {};
  }
}

/**
 * Write a config object to an opencode.json file, creating
 * parent directories as needed.
 */
export function writeConfig(configPath: string, config: OpenCodeConfig): void {
  const dir = dirname(configPath);
  mkdirSync(dir, { recursive: true });
  writeFileSync(configPath, JSON.stringify(config, null, 2) + "\n");
}

/**
 * Add the aide plugin and MCP server to a config object.
 * Preserves all existing config keys.
 */
export function addAideToConfig(
  config: OpenCodeConfig,
  options: { noMcp?: boolean } = {},
): OpenCodeConfig {
  const result = { ...config };

  // Ensure schema
  if (!result.$schema) {
    result.$schema = SCHEMA_URL;
  }

  // Add plugin to array (deduplicated)
  const plugins = result.plugin ?? [];
  if (!plugins.includes(PLUGIN_NAME)) {
    plugins.push(PLUGIN_NAME);
  }
  result.plugin = plugins;

  // Add MCP server config
  if (!options.noMcp) {
    const mcp = result.mcp ?? {};
    if (!mcp[MCP_SERVER_NAME]) {
      mcp[MCP_SERVER_NAME] = {
        type: "local",
        command: ["bunx", "-y", PLUGIN_NAME, "mcp"],
        environment: {
          AIDE_CODE_WATCH: "1",
          AIDE_CODE_WATCH_DELAY: "30s",
        },
        enabled: true,
      };
    }
    result.mcp = mcp;
  }

  return result;
}

/**
 * Remove the aide plugin and MCP server from a config object.
 */
export function removeAideFromConfig(config: OpenCodeConfig): OpenCodeConfig {
  const result = { ...config };

  // Remove plugin from array
  if (result.plugin) {
    result.plugin = result.plugin.filter((p) => p !== PLUGIN_NAME);
    if (result.plugin.length === 0) {
      delete result.plugin;
    }
  }

  // Remove MCP server
  if (result.mcp) {
    delete result.mcp[MCP_SERVER_NAME];
    if (Object.keys(result.mcp).length === 0) {
      delete result.mcp;
    }
  }

  return result;
}

/**
 * Check if aide is configured in a config object.
 */
export function isAideConfigured(config: OpenCodeConfig): {
  plugin: boolean;
  mcp: boolean;
} {
  return {
    plugin: config.plugin?.includes(PLUGIN_NAME) ?? false,
    mcp: config.mcp?.[MCP_SERVER_NAME] != null,
  };
}

export { PLUGIN_NAME, MCP_SERVER_NAME };
