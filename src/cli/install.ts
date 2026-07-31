/**
 * Install command — registers aide plugin and MCP server for OpenCode or Codex CLI.
 *
 * Config is written with the exact version of the package this command ran
 * from (`@jmylchreest/aide-plugin@0.1.13`), for both the plugin array and the
 * MCP command. Neither OpenCode nor bunx re-resolves an unpinned spec once
 * cached, so the pin is what makes upgrades happen at all — and it matches
 * plugin, MCP server and bundled binary to a single version. Re-running
 * `bunx @jmylchreest/aide-plugin@latest install` is therefore the upgrade
 * action; on reinstall this also repairs stale pins and command formats.
 *
 * Binary provisioning is handled by npm (via `optionalDependencies` on
 * `@jmylchreest/aide-binary-<arch>`) and the MCP wrapper. This command
 * reports the current binary status without modifying it.
 */

import {
  addAideToConfig,
  configuredPluginVersion,
  getGlobalConfigPath,
  getProjectConfigPath,
  isAideConfigured,
  isMcpCommandCurrent,
  pluginSpec,
  readConfig,
  writeConfig,
} from "./config.js";
import { installCodex, isCodexConfigured } from "./codex-config.js";
import { execFileSync } from "child_process";
import { existsSync } from "fs";
import { join } from "path";
import {
  compareVersions,
  getBaseVersion,
  isDevBuild,
} from "../lib/aide-downloader.js";
import { resolveBundledBinary } from "../lib/binary-resolve.js";
import {
  getRunningPluginRoot,
  getRunningPluginVersion,
} from "../lib/plugin-version.js";

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
  const pluginRoot = getRunningPluginRoot();

  // Hoisting means the per-arch package sits next to the plugin, not inside
  // it — resolveBundledBinary() follows node resolution to find it.
  const bundledPath = resolveBundledBinary({ pluginRoot, from: import.meta.url });
  if (bundledPath) {
    const version = await readBinaryVersion(bundledPath);
    if (version) {
      return { kind: "bundled", version, path: bundledPath };
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

async function installOpenCode(flags: InstallFlags): Promise<void> {
  const configPath = flags.project
    ? getProjectConfigPath()
    : getGlobalConfigPath();

  const scope = flags.project ? "project" : "global";
  console.log(`Installing aide plugin for OpenCode (${scope})...\n`);

  // The version of the package this command was launched from — that is what
  // gets pinned, so `bunx @jmylchreest/aide-plugin@latest install` is the
  // upgrade action for both the plugin and the MCP server.
  const version = getRunningPluginVersion();
  const spec = pluginSpec(version);

  const existing = readConfig(configPath);
  const before = isAideConfigured(existing);
  const pinnedBefore = configuredPluginVersion(existing);
  const pluginNeedsRepin = before.plugin && pinnedBefore !== (version ?? "");
  const mcpNeedsUpdate =
    !flags.noMcp && before.mcp && !isMcpCommandCurrent(existing, version);

  const configUnchanged =
    before.plugin && (before.mcp || flags.noMcp) && !pluginNeedsRepin && !mcpNeedsUpdate;

  if (configUnchanged) {
    console.log(`aide is already configured in ${configPath}\n`);
    console.log(`  plugin: ${spec}`);
    console.log("  mcp:    registered");
  } else {
    const updated = addAideToConfig(existing, {
      noMcp: flags.noMcp,
      version,
    });
    writeConfig(configPath, updated);

    const after = isAideConfigured(updated);
    console.log(`Updated: ${configPath}\n`);

    if (!before.plugin && after.plugin) {
      console.log(`  + Added "${spec}" to plugin array`);
    } else if (pluginNeedsRepin) {
      const was = pinnedBefore ? `v${pinnedBefore}` : "unpinned";
      console.log(`  ~ Re-pinned plugin to ${spec} (was ${was})`);
    } else {
      console.log(`  = Plugin already registered (${spec})`);
    }

    if (!flags.noMcp) {
      if (mcpNeedsUpdate) {
        console.log(`  ~ Updated "aide" MCP server command to ${spec}`);
      } else if (!before.mcp && after.mcp) {
        console.log(`  + Added "aide" MCP server (${spec})`);
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

  if (!version) {
    console.log(
      "\n  ! Running from an unversioned checkout — config left unpinned.\n" +
        "    Released installs pin the exact version so OpenCode picks up upgrades.",
    );
  } else if (pluginNeedsRepin) {
    console.log(
      `\nOpenCode installs the new plugin version on next start (pin changed to v${version}).`,
    );
  }

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
