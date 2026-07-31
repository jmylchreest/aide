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
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
      return {};
    return parsed as OpenCodeConfig;
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
 * Is this plugin-array entry ours? Matches the bare name and any pin
 * (`@jmylchreest/aide-plugin@0.1.13`, `...@latest`, `...@^0.1.0`).
 */
export function isAidePluginEntry(entry: string): boolean {
  return entry === PLUGIN_NAME || entry.startsWith(`${PLUGIN_NAME}@`);
}

/**
 * The npm spec to write into config.
 *
 * Pinning the exact version is what makes upgrades work at all:
 *
 *  - OpenCode caches plugin installs under a directory keyed by the spec
 *    string and returns early if it already exists (Npm.add), so a bare
 *    name — which it rewrites to `@latest` — is installed once and then
 *    frozen forever. Changing the pin changes the cache key, which is the
 *    only thing that triggers a real reinstall.
 *  - `bunx <name>` reuses its cached temp install with no registry check,
 *    so the MCP server drifts the same way. `bunx <name>@latest` would
 *    re-resolve, but that means a registry round-trip on every MCP launch
 *    and a hard failure when offline.
 *
 * An exact pin is reproducible, offline-safe, and matches the plugin, the
 * MCP server and the bundled per-arch binary to one version by construction.
 */
export function pluginSpec(version?: string | null): string {
  if (!version || version === "0.0.0") return PLUGIN_NAME;
  return `${PLUGIN_NAME}@${version}`;
}

/** The MCP command for a given plugin version. */
export function mcpCommand(version?: string | null): string[] {
  return ["bunx", "-y", pluginSpec(version), "mcp"];
}

/** Version pinned in a config's plugin array, "" when unpinned, null when absent. */
export function configuredPluginVersion(config: OpenCodeConfig): string | null {
  const entry = config.plugin?.find(isAidePluginEntry);
  if (entry === undefined) return null;
  return entry === PLUGIN_NAME ? "" : entry.slice(PLUGIN_NAME.length + 1);
}

/** Does the configured MCP command already launch the wanted version? */
export function isMcpCommandCurrent(
  config: OpenCodeConfig,
  version?: string | null,
): boolean {
  const command = config.mcp?.[MCP_SERVER_NAME]?.command;
  if (!command) return false;
  const wanted = mcpCommand(version);
  return (
    command.length === wanted.length && command.every((part, i) => part === wanted[i])
  );
}

/**
 * Add (or re-pin) the aide plugin and MCP server in a config object.
 * Preserves all existing config keys, and any user customisation of the
 * MCP entry other than the command itself.
 */
export function addAideToConfig(
  config: OpenCodeConfig,
  options: { noMcp?: boolean; version?: string | null } = {},
): OpenCodeConfig {
  const result = { ...config };
  const spec = pluginSpec(options.version);

  // Ensure schema
  if (!result.$schema) {
    result.$schema = SCHEMA_URL;
  }

  // Replace any existing aide entry (bare or differently pinned) in place,
  // so re-running install upgrades rather than accumulating duplicates.
  const plugins = (result.plugin ?? []).filter((p) => !isAidePluginEntry(p));
  plugins.push(spec);
  result.plugin = plugins;

  // Add or re-pin the MCP server config
  if (!options.noMcp) {
    const mcp = result.mcp ?? {};
    const existing = mcp[MCP_SERVER_NAME];
    mcp[MCP_SERVER_NAME] = existing
      ? { ...existing, command: mcpCommand(options.version) }
      : {
          type: "local",
          command: mcpCommand(options.version),
          enabled: true,
        };
    result.mcp = mcp;
  }

  return result;
}

/**
 * Remove the aide plugin and MCP server from a config object.
 */
export function removeAideFromConfig(config: OpenCodeConfig): OpenCodeConfig {
  const result = { ...config };

  // Remove plugin from array (bare or pinned)
  if (result.plugin) {
    result.plugin = result.plugin.filter((p) => !isAidePluginEntry(p));
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
    plugin: config.plugin?.some(isAidePluginEntry) ?? false,
    mcp: config.mcp?.[MCP_SERVER_NAME] != null,
  };
}

export { PLUGIN_NAME, MCP_SERVER_NAME };
