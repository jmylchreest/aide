/**
 * Install command — registers aide plugin and MCP server for OpenCode or Codex CLI.
 *
 * On reinstall, detects and upgrades stale MCP command configurations
 * (e.g. old `aide-wrapper` commands) to the current format.
 *
 * Also reconciles the Go binary: if the binary on disk is older than the
 * just-installed plugin package version, downloads the matching release.
 * Skip with --no-upgrade.
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
  downloadAideBinary,
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

export interface InstallFlags {
  project?: boolean;
  noMcp?: boolean;
  noUpgrade?: boolean;
  platform?: "opencode" | "codex";
}

export type UpgradeOutcome =
  | { kind: "current"; binaryVersion: string; pluginVersion: string }
  | { kind: "upgraded"; fromVersion: string | null; toVersion: string; path: string }
  | { kind: "skipped"; reason: string }
  | { kind: "failed"; error: string };

/**
 * Ensure the on-disk aide binary matches the version of the plugin package
 * that was just installed.
 *
 * "On-disk" here means the binary in the plugin's own bin/ directory —
 * the one the wrapper will exec on the next MCP launch. We deliberately
 * do NOT touch `<cwd>/.aide/bin/aide`: that's a user-managed local copy
 * (typically built from source via the Go Makefile). Users can run
 * `aide upgrade` there themselves if they want both copies in sync.
 */
export async function ensureBinaryMatchesPluginVersion(): Promise<UpgradeOutcome> {
  const pluginVersion = getInstallScriptPluginVersion();
  if (!pluginVersion) {
    return { kind: "skipped", reason: "could not read plugin package.json version" };
  }

  const pluginRoot = getInstallScriptPluginRoot();
  const isWindows = process.platform === "win32";
  const pluginBinPath = join(pluginRoot, "bin", `aide${isWindows ? ".exe" : ""}`);

  const existingVersion = existsSync(pluginBinPath)
    ? await readBinaryVersion(pluginBinPath)
    : null;

  if (existingVersion && isBinaryCurrent(existingVersion, pluginVersion)) {
    return { kind: "current", binaryVersion: existingVersion, pluginVersion };
  }

  const result = await downloadAideBinary(join(pluginRoot, "bin"), {
    force: true,
    pluginVersion,
  });
  if (!result.success) {
    return { kind: "failed", error: result.message };
  }
  const newVersion = await readBinaryVersion(result.path!);
  return {
    kind: "upgraded",
    fromVersion: existingVersion,
    toVersion: newVersion ?? pluginVersion,
    path: result.path!,
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

function printUpgradeOutcome(outcome: UpgradeOutcome, noUpgrade: boolean): void {
  if (noUpgrade) {
    console.log("  - Skipped binary upgrade (--no-upgrade)");
    return;
  }
  switch (outcome.kind) {
    case "current":
      console.log(`  = Binary up to date (v${outcome.binaryVersion})`);
      return;
    case "upgraded":
      console.log(
        outcome.fromVersion
          ? `  + Upgraded binary: v${outcome.fromVersion} → v${outcome.toVersion}`
          : `  + Installed binary: v${outcome.toVersion}`,
      );
      return;
    case "skipped":
      console.log(`  ! Skipped binary upgrade: ${outcome.reason}`);
      return;
    case "failed":
      console.log(`  ! Binary upgrade failed: ${outcome.error}`);
      console.log("    Config is installed; run `aide upgrade` to retry.");
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

  if (configUnchanged && flags.noUpgrade) {
    console.log(`aide is already configured in ${configPath}`);
    console.log("  plugin: registered");
    console.log("  mcp:    registered");
    console.log("  upgrade: skipped (--no-upgrade)");
    return;
  }

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
  const outcome = flags.noUpgrade
    ? { kind: "skipped" as const, reason: "--no-upgrade" }
    : await ensureBinaryMatchesPluginVersion();
  printUpgradeOutcome(outcome, !!flags.noUpgrade);

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
    if (flags.noUpgrade) {
      console.log("  upgrade: skipped (--no-upgrade)");
      return;
    }
    const outcome = await ensureBinaryMatchesPluginVersion();
    console.log("");
    printUpgradeOutcome(outcome, false);
    return;
  }

  if (!flags.noUpgrade) {
    const outcome = await ensureBinaryMatchesPluginVersion();
    console.log("");
    printUpgradeOutcome(outcome, false);
  } else {
    console.log("  - Skipped binary upgrade (--no-upgrade)");
  }

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
