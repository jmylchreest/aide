/**
 * Install command — registers aide plugin and MCP server for OpenCode or Codex CLI.
 *
 * On reinstall, detects and upgrades stale MCP command configurations
 * (e.g. old `aide-wrapper` commands) to the current format.
 *
 * Binary provisioning is handled by npm (via `optionalDependencies` on
 * `@jmylchreest/aide-binary-<arch>`) and the MCP wrapper. This command
 * reports the current binary status without modifying it; users upgrade
 * the binary by running `bunx @jmylchreest/aide-plugin@latest` (which
 * re-extracts the npm package and refreshes the optional dep).
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
import { join, dirname } from "path";
import { execFileSync } from "child_process";
import { existsSync, readFileSync } from "fs";
import { fileURLToPath } from "url";
import {
  compareVersions,
  getBaseVersion,
  isDevBuild,
} from "../lib/aide-downloader.js";

/**
 * Derive the plugin root from this script's location.
 *
 * We cannot trust `getPluginRoot()` (AIDE_PLUGIN_ROOT /
 * CLAUDE_PLUGIN_ROOT env vars): when `install` runs from
 * `bunx ...@latest install` it lives in bunx's cache, while the env may
 * be inherited from a parent OpenCode/Claude session pointing at a
 * different (often stale) plugin cache. Anchoring to `import.meta.url`
 * ties the version source to the package the user just installed.
 */
function getInstallScriptPluginRoot(): string {
  const scriptPath = fileURLToPath(import.meta.url);
  return join(dirname(scriptPath), "..", "..");
}

function getInstallScriptPluginVersion(): string | null {
  const root = getInstallScriptPluginRoot();
  try {
    const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf-8"));
    return pkg.version && pkg.version !== "0.0.0" ? pkg.version : null;
  } catch {
    return null;
  }
}

/** Map process.platform + process.arch → per-arch npm package suffix. */
function getArchPackageSuffix(): string | null {
  const key = `${process.platform}-${process.arch}`;
  const ARCH_PKG: Record<string, string> = {
    "linux-x64": "linux-amd64",
    "linux-arm64": "linux-arm64",
    "darwin-x64": "darwin-amd64",
    "darwin-arm64": "darwin-arm64",
    "win32-x64": "windows-amd64",
    "win32-arm64": "windows-arm64",
  };
  return ARCH_PKG[key] ?? null;
}

export interface InstallFlags {
  project?: boolean;
  noMcp?: boolean;
  platform?: "opencode" | "codex";
}

export type BinaryStatus =
  | { kind: "bundled"; version: string; path: string }
  | { kind: "legacy"; version: string; path: string }
  | { kind: "missing"; reason: string };

/**
 * Report where the aide binary lives (bundled via npm or legacy download).
 * Does NOT modify the filesystem — the wrapper handles actual provisioning.
 */
export async function checkBinaryStatus(): Promise<BinaryStatus> {
  const isWindows = process.platform === "win32";
  const ext = isWindows ? ".exe" : "";
  const pluginRoot = getInstallScriptPluginRoot();

  const archSuffix = getArchPackageSuffix();
  if (archSuffix) {
    const bundledPath = join(
      pluginRoot,
      "node_modules",
      `@jmylchreest/aide-binary-${archSuffix}`,
      "bin",
      `aide${ext}`,
    );
    if (existsSync(bundledPath)) {
      const version = await readBinaryVersion(bundledPath);
      if (version) {
        return { kind: "bundled", version, path: bundledPath };
      }
    }
  }

  const legacyPath = join(pluginRoot, "bin", `aide${ext}`);
  if (existsSync(legacyPath)) {
    const version = await readBinaryVersion(legacyPath);
    if (version) {
      return { kind: "legacy", version, path: legacyPath };
    }
  }

  return {
    kind: "missing",
    reason:
      "no binary on disk; wrapper will download on first MCP launch " +
      "(users on OpenCode/Codex with bundling get it via `bunx @latest`)",
  };
}

export async function readBinaryVersion(path: string): Promise<string | null> {
  try {
    const out = execFileSync(path, ["version"], { stdio: "pipe", timeout: 5000 })
      .toString()
      .trim();
    const m = out.match(/(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.+]+)?)/);
    return m ? m[1] : null;
  } catch {
    return null;
  }
}

export function isBinaryCurrent(binaryVersion: string, pluginVersion: string): boolean {
  const base = isDevBuild(binaryVersion) ? getBaseVersion(binaryVersion) : binaryVersion;
  return compareVersions(base, pluginVersion) >= 0;
}

function printBinaryStatus(status: BinaryStatus): void {
  switch (status.kind) {
    case "bundled":
      console.log(`  = Binary bundled via npm: v${status.version}`);
      return;
    case "legacy":
      console.log(`  = Binary (legacy download path): v${status.version}`);
      console.log("    Re-run `bunx @jmylchreest/aide-plugin@latest install` to migrate to bundled.");
      return;
    case "missing":
      console.log(`  ! ${status.reason}`);
      return;
  }
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

  const configUnchanged = before.plugin && before.mcp && !mcpNeedsUpdate;

  if (configUnchanged) {
    console.log(`aide is already configured in ${configPath}\n`);
    console.log("  plugin: registered");
    console.log("  mcp:    registered");
  } else {
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
  }

  console.log("");
  const status = await checkBinaryStatus();
  printBinaryStatus(status);

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
  const result = installCodex(scope);

  if (result.pluginManaged) {
    console.log(
      "  = Codex plugin detected — MCP server and skills are plugin-managed",
    );
    console.log("    (hooks stay here: Codex has no plugin hook support)");
    if (result.configWritten) {
      console.log("  - Removed redundant aide MCP server from config.toml");
    }
  } else if (result.configWritten) {
    console.log(
      result.mcpRepaired
        ? "  + Repaired stale aide MCP server command in config.toml"
        : "  + Added aide MCP server to config.toml",
    );
  } else if (before.mcp) {
    console.log("  = MCP server already registered in config.toml");
  }

  if (result.hooksWritten) {
    console.log(
      result.hooksRepaired
        ? "  + Regenerated stale aide hook commands in hooks.json"
        : "  + Generated hooks.json with aide hooks",
    );
  } else if (before.hooks) {
    console.log("  = Hooks already registered in hooks.json");
  }

  const skills = result.skills;
  const skillChanges =
    skills.installed.length + skills.updated.length + skills.removed.length;
  if (result.pluginManaged) {
    if (skills.removed.length) {
      console.log(
        `  - Removed ${skills.removed.length} loose skill copies (plugin provides skills)`,
      );
    }
  } else if (skillChanges > 0) {
    const parts: string[] = [];
    if (skills.installed.length) parts.push(`${skills.installed.length} installed`);
    if (skills.updated.length) parts.push(`${skills.updated.length} updated`);
    if (skills.removed.length) parts.push(`${skills.removed.length} removed`);
    console.log(`  + Skills synced: ${parts.join(", ")}`);
  } else {
    console.log("  = Skills up to date");
  }
  if (skills.skipped.length) {
    console.log(
      `  ! Skipped existing non-aide skills: ${skills.skipped.join(", ")}`,
    );
  }

  if (!result.configWritten && !result.hooksWritten && skillChanges === 0) {
    const status = await checkBinaryStatus();
    console.log("");
    printBinaryStatus(status);
    return;
  }

  const status = await checkBinaryStatus();
  console.log("");
  printBinaryStatus(status);

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
