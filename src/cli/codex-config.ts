/**
 * Codex CLI configuration generator.
 *
 * Generates .codex/config.toml (MCP server) and .codex/hooks.json
 * for integrating aide with Codex CLI.
 */

import {
  cpSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "fs";
import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";
import { homedir } from "os";
import * as TOML from "smol-toml";
import whichSync from "which";

const MCP_SERVER_NAME = "aide";
const AIDE_PLUGIN_BIN_NAME = "aide-plugin";

/** Check if a hook command belongs to aide (matches both global install and local dev paths). */
function isAideHookCommand(command?: string): boolean {
  if (!command) return false;
  return command.includes(AIDE_PLUGIN_BIN_NAME) || command.includes("index.ts hook");
}

/**
 * Check that a configured command can actually run: the binary resolves on
 * PATH, and if it's a script runner pointed at an absolute path, that path
 * still exists. Catches configs left behind by an uninstalled global
 * `aide-plugin` or a moved dev checkout.
 */
function commandIsRunnable(command: string, args: string[] = []): boolean {
  if (!command) return false;
  if (!whichSync.sync(command, { nothrow: true })) return false;
  if (["bun", "bunx", "node"].includes(command)) {
    const script = args.find((a) => !a.startsWith("-"));
    if (script && (script.startsWith("/") || /^[A-Za-z]:[\\/]/.test(script))) {
      return existsSync(script);
    }
  }
  return true;
}

/** Runnability check for a hooks.json command string (space-separated). */
function isHookCommandRunnable(command: string): boolean {
  const parts = command.split(" ").filter(Boolean);
  return commandIsRunnable(parts[0] ?? "", parts.slice(1));
}

/**
 * Resolve the command prefix for hook/mcp commands.
 *
 * If `aide-plugin` is on PATH (global npm install), use it directly.
 * Otherwise fall back to `bun <path-to-cli>` for local dev.
 */
function resolvePluginCommand(): { bin: string; hookPrefix: string; mcpCommand: string; mcpArgs: string[]; wrapperEnv?: Record<string, string> } {
  try {
    const resolved = whichSync.sync(AIDE_PLUGIN_BIN_NAME, { nothrow: true });
    if (resolved) {
      return {
        bin: AIDE_PLUGIN_BIN_NAME,
        hookPrefix: `${AIDE_PLUGIN_BIN_NAME} hook`,
        mcpCommand: AIDE_PLUGIN_BIN_NAME,
        mcpArgs: ["mcp"],
      };
    }
  } catch { /* not on PATH */ }

  // Fallback: use bun with full path to the wrapper/CLI
  const thisDir = dirname(fileURLToPath(import.meta.url));
  const pluginRoot = resolve(thisDir, "..", "..");
  const cliPath = join(pluginRoot, "src", "cli", "index.ts");
  const wrapperPath = join(pluginRoot, "bin", "aide-wrapper.ts");

  return {
    bin: `bun ${cliPath}`,
    hookPrefix: `bun ${cliPath} hook`,
    mcpCommand: "bun",
    mcpArgs: [wrapperPath, "mcp"],
    wrapperEnv: { AIDE_PLUGIN_ROOT: pluginRoot },
  };
}

// =============================================================================
// Config paths
// =============================================================================

export function getCodexGlobalConfigDir(): string {
  return join(homedir(), ".codex");
}

export function getCodexProjectConfigDir(cwd?: string): string {
  return join(cwd || process.cwd(), ".codex");
}

export function getCodexConfigTomlPath(scope: "user" | "project"): string {
  const dir =
    scope === "user"
      ? getCodexGlobalConfigDir()
      : getCodexProjectConfigDir();
  return join(dir, "config.toml");
}

export function getCodexHooksJsonPath(scope: "user" | "project"): string {
  const dir =
    scope === "user"
      ? getCodexGlobalConfigDir()
      : getCodexProjectConfigDir();
  return join(dir, "hooks.json");
}

/**
 * Codex discovers skills from `.agents/skills` (walking up from cwd) and
 * `~/.agents/skills` — NOT from ~/.codex/. See
 * https://developers.openai.com/codex/skills
 */
export function getCodexSkillsDir(scope: "user" | "project"): string {
  const base = scope === "user" ? homedir() : process.cwd();
  return join(base, ".agents", "skills");
}

// =============================================================================
// TOML config read/write
// =============================================================================

interface CodexTomlConfig {
  mcp_servers?: Record<string, Record<string, unknown>>;
  [key: string]: unknown;
}

function readCodexToml(path: string): CodexTomlConfig {
  if (!existsSync(path)) return {};
  try {
    return TOML.parse(readFileSync(path, "utf-8")) as CodexTomlConfig;
  } catch {
    return {};
  }
}

function writeCodexToml(path: string, config: CodexTomlConfig): void {
  const dir = dirname(path);
  mkdirSync(dir, { recursive: true });
  writeFileSync(
    path,
    TOML.stringify(config as Record<string, unknown>) + "\n",
  );
}

// =============================================================================
// hooks.json generation
// =============================================================================

interface CodexHookEntry {
  type: string;
  command: string;
  timeout?: number;
  statusMessage?: string;
}

interface CodexHookMatcher {
  matcher: string;
  hooks: CodexHookEntry[];
}

interface CodexHooksJson {
  hooks: Record<string, CodexHookMatcher[]>;
}

function generateHooksJson(hookPrefix: string): CodexHooksJson {
  return {
    hooks: {
      SessionStart: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} session-start`,
              timeout: 60,
              statusMessage: "Initializing aide session",
            },
          ],
        },
      ],
      UserPromptSubmit: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} skill-injector`,
              timeout: 5,
              statusMessage: "Matching aide skills",
            },
          ],
        },
      ],
      PreToolUse: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} tool-tracker`,
              timeout: 2,
            },
            {
              type: "command",
              command: `${hookPrefix} write-guard`,
              timeout: 3,
            },
            {
              type: "command",
              command: `${hookPrefix} pre-tool-enforcer`,
              timeout: 3,
            },
            {
              type: "command",
              command: `${hookPrefix} context-guard`,
              timeout: 2,
            },
            {
              type: "command",
              command: `${hookPrefix} search-enrichment`,
              timeout: 3,
            },
          ],
        },
      ],
      PostToolUse: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} tool-observe`,
              timeout: 3,
            },
            {
              type: "command",
              command: `${hookPrefix} hud-updater`,
              timeout: 3,
            },
            {
              type: "command",
              command: `${hookPrefix} comment-checker`,
              timeout: 3,
            },
            {
              type: "command",
              command: `${hookPrefix} context-pruning`,
              timeout: 3,
            },
          ],
        },
      ],
      // Failed tool calls arrive on a separate event (PostToolUse fires on
      // success only); route them to tool-observe so the friction detector
      // sees the failure. Same script — it reads the top-level error fields.
      PostToolUseFailure: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} tool-observe`,
              timeout: 3,
            },
          ],
        },
      ],
      Stop: [
        {
          matcher: "*",
          hooks: [
            {
              type: "command",
              command: `${hookPrefix} persistence`,
              timeout: 5,
            },
            {
              type: "command",
              command: `${hookPrefix} session-summary`,
              timeout: 10,
            },
            {
              type: "command",
              command: `${hookPrefix} agent-cleanup`,
              timeout: 5,
            },
            {
              type: "command",
              command: `${hookPrefix} session-end`,
              timeout: 10,
            },
          ],
        },
      ],
    },
  };
}

// =============================================================================
// Skills
// =============================================================================

/**
 * Codex discovers skills natively from its skills/ directory (SKILL.md
 * format — the same format aide's bundled skills already use). We copy the
 * bundled skills there and track what we copied in a manifest, so uninstall
 * and re-sync never touch skills the user created themselves.
 */
const SKILLS_MANIFEST = ".aide-skills.json";

export interface CodexSkillsResult {
  installed: string[];
  updated: string[];
  removed: string[];
  skipped: string[];
}

function getBundledSkillsDir(): string | null {
  const thisDir = dirname(fileURLToPath(import.meta.url));
  const dir = join(resolve(thisDir, "..", ".."), "skills");
  return existsSync(dir) ? dir : null;
}

function readSkillsManifest(skillsDir: string): string[] {
  try {
    const parsed = JSON.parse(
      readFileSync(join(skillsDir, SKILLS_MANIFEST), "utf-8"),
    ) as { skills?: string[] };
    return parsed.skills ?? [];
  } catch {
    return [];
  }
}

/** Deep-compare two directories by structure and file content. */
function dirsEqual(src: string, dest: string): boolean {
  const srcEntries = readdirSync(src, { withFileTypes: true });
  const destEntries = readdirSync(dest, { withFileTypes: true });
  if (srcEntries.length !== destEntries.length) return false;
  const destNames = new Set(destEntries.map((e) => e.name));
  for (const entry of srcEntries) {
    if (!destNames.has(entry.name)) return false;
    const s = join(src, entry.name);
    const d = join(dest, entry.name);
    if (entry.isDirectory()) {
      if (!statSync(d).isDirectory() || !dirsEqual(s, d)) return false;
    } else {
      if (!statSync(d).isFile()) return false;
      if (readFileSync(s, "utf-8") !== readFileSync(d, "utf-8")) return false;
    }
  }
  return true;
}

/**
 * Sync bundled skills into the Codex skills directory.
 * Skills we previously copied (per the manifest) are updated or removed to
 * match the bundle; directories we don't own are skipped, never overwritten.
 */
export function installCodexSkills(scope: "user" | "project"): CodexSkillsResult {
  const result: CodexSkillsResult = {
    installed: [],
    updated: [],
    removed: [],
    skipped: [],
  };
  const src = getBundledSkillsDir();
  if (!src) return result;

  const dest = getCodexSkillsDir(scope);
  mkdirSync(dest, { recursive: true });
  const owned = new Set(readSkillsManifest(dest));
  const synced: string[] = [];

  for (const entry of readdirSync(src, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const from = join(src, entry.name);
    if (!existsSync(join(from, "SKILL.md"))) continue;
    const to = join(dest, entry.name);

    const exists = existsSync(to);
    if (exists && !owned.has(entry.name)) {
      result.skipped.push(entry.name);
      continue;
    }
    if (exists && dirsEqual(from, to)) {
      synced.push(entry.name);
      continue;
    }
    rmSync(to, { recursive: true, force: true });
    cpSync(from, to, { recursive: true });
    (exists ? result.updated : result.installed).push(entry.name);
    synced.push(entry.name);
  }

  // Skills we own that no longer ship in the bundle
  for (const name of owned) {
    if (!synced.includes(name)) {
      const p = join(dest, name);
      if (existsSync(p)) {
        rmSync(p, { recursive: true, force: true });
        result.removed.push(name);
      }
    }
  }

  writeFileSync(
    join(dest, SKILLS_MANIFEST),
    JSON.stringify({ skills: synced }, null, 2) + "\n",
  );
  return result;
}

/** Remove every skill listed in the manifest, plus the manifest itself. */
export function uninstallCodexSkills(scope: "user" | "project"): string[] {
  const dest = getCodexSkillsDir(scope);
  if (!existsSync(dest)) return [];
  const removed: string[] = [];
  for (const name of readSkillsManifest(dest)) {
    const p = join(dest, name);
    if (existsSync(p)) {
      rmSync(p, { recursive: true, force: true });
      removed.push(name);
    }
  }
  rmSync(join(dest, SKILLS_MANIFEST), { force: true });
  return removed;
}

// =============================================================================
// Install / uninstall
// =============================================================================

/**
 * Detect an aide install managed by Codex's own plugin system
 * (`codex plugin add aide@<marketplace>`). The plugin provides the MCP
 * server and skills natively, so the installer should only manage hooks —
 * Codex dropped support for plugin-shipped hooks (`plugin_hooks` feature
 * is removed), which keeps hooks.json our job.
 */
export function isCodexPluginManaged(): boolean {
  const config = readCodexToml(getCodexConfigTomlPath("user"));
  const plugins = (config.plugins || {}) as Record<
    string,
    Record<string, unknown> | undefined
  >;
  return Object.entries(plugins).some(
    ([key, value]) => key.startsWith("aide@") && value?.enabled !== false,
  );
}

export function installCodex(scope: "user" | "project"): {
  configWritten: boolean;
  hooksWritten: boolean;
  mcpRepaired: boolean;
  hooksRepaired: boolean;
  pluginManaged: boolean;
  skills: CodexSkillsResult;
} {
  const configPath = getCodexConfigTomlPath(scope);
  const hooksPath = getCodexHooksJsonPath(scope);
  let configWritten = false;
  let hooksWritten = false;

  const config = readCodexToml(configPath);
  const resolved = resolvePluginCommand();
  let configChanged = false;

  // Enable hooks feature flag (required for Codex to process hooks.json).
  // Codex renamed [features].codex_hooks to [features].hooks and warns on
  // the old name, so migrate it away if present.
  const features = (config.features || {}) as Record<string, unknown>;
  if ("codex_hooks" in features) {
    delete features.codex_hooks;
    config.features = features;
    configChanged = true;
  }
  if (!features.hooks) {
    features.hooks = true;
    config.features = features;
    configChanged = true;
  }

  // Add aide MCP server — or repair an entry whose command no longer runs
  // (e.g. a globally-installed aide-plugin that has since been removed).
  // When the Codex plugin manages aide, the plugin provides the MCP server,
  // so an entry here would collide with it by name — remove instead.
  const pluginManaged = isCodexPluginManaged();
  const mcpServers = (config.mcp_servers || {}) as Record<
    string,
    Record<string, unknown>
  >;

  let mcpRepaired = false;
  const existingMcp = mcpServers[MCP_SERVER_NAME];

  if (pluginManaged) {
    if (existingMcp) {
      delete mcpServers[MCP_SERVER_NAME];
      if (Object.keys(mcpServers).length === 0) {
        delete config.mcp_servers;
      } else {
        config.mcp_servers = mcpServers;
      }
      configChanged = true;
    }
  } else {
    const existingMcpRunnable =
      existingMcp &&
      commandIsRunnable(
        typeof existingMcp.command === "string" ? existingMcp.command : "",
        Array.isArray(existingMcp.args) ? (existingMcp.args as string[]) : [],
      );

    if (!existingMcp || !existingMcpRunnable) {
      mcpRepaired = Boolean(existingMcp);
      mcpServers[MCP_SERVER_NAME] = {
        command: resolved.mcpCommand,
        args: resolved.mcpArgs,
        env: {
          AIDE_CODE_WATCH: "1",
          AIDE_CODE_WATCH_DELAY: "30s",
          ...(resolved.wrapperEnv || {}),
        },
      };
      config.mcp_servers = mcpServers;
      configChanged = true;
    }
  }

  if (configChanged) {
    writeCodexToml(configPath, config);
    configWritten = true;
  }

  let existingHooks: CodexHooksJson | null = null;
  if (existsSync(hooksPath)) {
    try {
      existingHooks = JSON.parse(
        readFileSync(hooksPath, "utf-8"),
      ) as CodexHooksJson;
    } catch {
      // Overwrite corrupt file
    }
  }

  // If any existing aide hook command no longer runs, strip all aide hooks
  // so the block below regenerates them with the freshly resolved prefix.
  let hooksRepaired = false;
  if (existingHooks?.hooks) {
    const aideCommands = Object.values(existingHooks.hooks)
      .flat()
      .flatMap((m) => m.hooks ?? [])
      .map((h) => h.command)
      .filter(isAideHookCommand);
    if (aideCommands.length > 0 && aideCommands.some((c) => !isHookCommandRunnable(c))) {
      hooksRepaired = true;
      for (const [event, matchers] of Object.entries(existingHooks.hooks)) {
        existingHooks.hooks[event] = matchers.filter(
          (m) => !m.hooks?.some((h) => isAideHookCommand(h.command)),
        );
        if (existingHooks.hooks[event].length === 0) {
          delete existingHooks.hooks[event];
        }
      }
    }
  }

  const hasAideHook = (event: string) =>
    existingHooks?.hooks?.[event]?.some((m) =>
      m.hooks?.some((h) => isAideHookCommand(h.command)),
    ) ?? false;
  const hasAideHooks = hasAideHook("SessionStart") && hasAideHook("Stop");

  if (!hasAideHooks) {
    const dir = dirname(hooksPath);
    mkdirSync(dir, { recursive: true });

    if (existingHooks?.hooks) {
      // Merge: add aide hooks to existing hooks.json
      const aideHooks = generateHooksJson(resolved.hookPrefix).hooks;
      for (const [event, matchers] of Object.entries(aideHooks)) {
        if (!existingHooks.hooks[event]) {
          existingHooks.hooks[event] = matchers;
        } else {
          // Append aide matchers to existing event
          existingHooks.hooks[event].push(...matchers);
        }
      }
      writeFileSync(
        hooksPath,
        JSON.stringify(existingHooks, null, 2) + "\n",
      );
    } else {
      writeFileSync(
        hooksPath,
        JSON.stringify(generateHooksJson(resolved.hookPrefix), null, 2) + "\n",
      );
    }
    hooksWritten = true;
  }

  // Plugin-managed installs get skills from the plugin cache (namespaced
  // aide:<name>); loose copies in .agents/skills would duplicate them.
  const skills: CodexSkillsResult = pluginManaged
    ? {
        installed: [],
        updated: [],
        removed: uninstallCodexSkills(scope),
        skipped: [],
      }
    : installCodexSkills(scope);

  return {
    configWritten,
    hooksWritten,
    mcpRepaired,
    hooksRepaired,
    pluginManaged,
    skills,
  };
}

export function uninstallCodex(scope: "user" | "project"): {
  configRemoved: boolean;
  hooksRemoved: boolean;
  skillsRemoved: string[];
} {
  const configPath = getCodexConfigTomlPath(scope);
  const hooksPath = getCodexHooksJsonPath(scope);
  let configRemoved = false;
  let hooksRemoved = false;

  // Remove aide MCP server from config.toml
  if (existsSync(configPath)) {
    const config = readCodexToml(configPath);
    const mcpServers = (config.mcp_servers || {}) as Record<
      string,
      Record<string, unknown>
    >;

    if (mcpServers[MCP_SERVER_NAME]) {
      delete mcpServers[MCP_SERVER_NAME];
      if (Object.keys(mcpServers).length === 0) {
        delete config.mcp_servers;
      } else {
        config.mcp_servers = mcpServers;
      }
      writeCodexToml(configPath, config);
      configRemoved = true;
    }
  }

  // Remove aide hooks from hooks.json
  if (existsSync(hooksPath)) {
    try {
      const hooks = JSON.parse(
        readFileSync(hooksPath, "utf-8"),
      ) as CodexHooksJson;

      if (hooks.hooks) {
        let changed = false;
        for (const [event, matchers] of Object.entries(hooks.hooks)) {
          const filtered = matchers.filter(
            (m) => !m.hooks?.some((h) => isAideHookCommand(h.command)),
          );
          if (filtered.length !== matchers.length) {
            hooks.hooks[event] = filtered;
            changed = true;
          }
          if (hooks.hooks[event].length === 0) {
            delete hooks.hooks[event];
          }
        }
        if (changed) {
          writeFileSync(hooksPath, JSON.stringify(hooks, null, 2) + "\n");
          hooksRemoved = true;
        }
      }
    } catch {
      // Leave corrupt file alone
    }
  }

  const skillsRemoved = uninstallCodexSkills(scope);

  return { configRemoved, hooksRemoved, skillsRemoved };
}

export function isCodexConfigured(scope: "user" | "project"): {
  mcp: boolean;
  hooks: boolean;
} {
  const configPath = getCodexConfigTomlPath(scope);
  const hooksPath = getCodexHooksJsonPath(scope);

  let mcp = false;
  if (existsSync(configPath)) {
    const config = readCodexToml(configPath);
    const mcpServers = (config.mcp_servers || {}) as Record<string, unknown>;
    mcp = MCP_SERVER_NAME in mcpServers;
  }

  let hooks = false;
  if (existsSync(hooksPath)) {
    try {
      const hooksJson = JSON.parse(
        readFileSync(hooksPath, "utf-8"),
      ) as CodexHooksJson;
      hooks =
        hooksJson.hooks?.SessionStart?.some((m) =>
          m.hooks?.some((h) => isAideHookCommand(h.command)),
        ) ?? false;
    } catch {
      // Corrupt file
    }
  }

  return { mcp, hooks };
}
