/**
 * Codex CLI configuration generator.
 *
 * Generates .codex/config.toml (MCP server) and .codex/hooks.json
 * for integrating aide with Codex CLI.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
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
          ],
        },
      ],
      PostToolUse: [
        {
          matcher: "*",
          hooks: [
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
// Install / uninstall
// =============================================================================

export function installCodex(scope: "user" | "project"): {
  configWritten: boolean;
  hooksWritten: boolean;
} {
  const configPath = getCodexConfigTomlPath(scope);
  const hooksPath = getCodexHooksJsonPath(scope);
  let configWritten = false;
  let hooksWritten = false;

  // Add aide MCP server to config.toml
  const config = readCodexToml(configPath);
  const mcpServers = (config.mcp_servers || {}) as Record<
    string,
    Record<string, unknown>
  >;

  const resolved = resolvePluginCommand();

  if (!mcpServers[MCP_SERVER_NAME]) {
    const mcpEntry: Record<string, unknown> = {
      command: resolved.mcpCommand,
      args: resolved.mcpArgs,
      env: {
        AIDE_CODE_WATCH: "1",
        AIDE_CODE_WATCH_DELAY: "30s",
        ...(resolved.wrapperEnv || {}),
      },
    };
    mcpServers[MCP_SERVER_NAME] = mcpEntry;
    config.mcp_servers = mcpServers;
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

  return { configWritten, hooksWritten };
}

export function uninstallCodex(scope: "user" | "project"): {
  configRemoved: boolean;
  hooksRemoved: boolean;
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

  return { configRemoved, hooksRemoved };
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
