/** Reversible Codex configuration changes used by aide-dev-toggle.sh. */
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import * as TOML from "smol-toml";
import { generateHooksJson, isAideHookCommand } from "./codex-config.js";

type Table = Record<string, any>;
type Hooks = ReturnType<typeof generateHooksJson>;
export interface CodexDevPaths {
  repo: string;
  configDir: string;
  skillsDir: string;
}
interface Snapshot {
  repo: string;
  mcp?: Table;
  plugins: Record<string, { enabled?: boolean; mcp?: { enabled?: boolean } }>;
  hooksFeature?: boolean;
  hooks: Hooks;
  skills: string[];
  devSkills: string[];
  manifest?: string;
}

const STATE_DIR = "aide-dev-toggle";
const MANIFEST = ".aide-skills.json";
const BINARY = process.platform === "win32" ? "aide.exe" : "aide";

function quoteArgument(value: string): string {
  return process.platform === "win32"
    ? `"${value.replace(/"/g, '\\"')}"`
    : `'${value.replace(/'/g, "'\\''")}'`;
}

function isLocalHook(command: string, paths: CodexDevPaths): boolean {
  const cli = join(paths.repo, "src", "cli", "index.ts");
  return command.includes(cli) || command.includes(quoteArgument(cli));
}

function isLocalMcp(mcp: Table, paths: CodexDevPaths): boolean {
  return (
    mcp.command === join(paths.repo, "bin", BINARY) ||
    (mcp.args ?? []).some((arg: string) =>
      [
        join(paths.repo, "bin", "aide-wrapper.ts"),
        join(paths.repo, "src", "cli", "index.ts"),
      ].includes(arg),
    )
  );
}

function readJson<T>(path: string, fallback: T): T {
  return existsSync(path)
    ? (JSON.parse(readFileSync(path, "utf8")) as T)
    : fallback;
}

function readConfig(paths: CodexDevPaths): Table {
  const path = join(paths.configDir, "config.toml");
  return existsSync(path) ? TOML.parse(readFileSync(path, "utf8")) : {};
}

function selectHooks(hooks: Hooks, aide: boolean): Hooks {
  return {
    ...hooks,
    hooks: Object.fromEntries(
      Object.entries(hooks.hooks).flatMap(([event, matchers]) => {
        const selected = matchers
          .map((matcher) => ({
            ...matcher,
            hooks: matcher.hooks.filter(
              (hook) => isAideHookCommand(hook.command) === aide,
            ),
          }))
          .filter((matcher) => matcher.hooks.length > 0);
        return selected.length ? [[event, selected]] : [];
      }),
    ),
  };
}

function mergeHooks(current: Hooks, aide: Hooks): Hooks {
  const result = selectHooks(current, false);
  for (const [event, matchers] of Object.entries(aide.hooks)) {
    result.hooks[event] = [...(result.hooks[event] ?? []), ...matchers];
  }
  return result;
}

function pluginEntries(config: Table): [string, Table][] {
  return Object.entries(config.plugins ?? {}).filter(([key]) =>
    key.startsWith("aide@"),
  ) as [string, Table][];
}

function snapshotPath(paths: CodexDevPaths): string {
  return join(paths.configDir, STATE_DIR, "state.json");
}

function readSnapshot(paths: CodexDevPaths): Snapshot | null {
  const state = readJson<Snapshot | null>(snapshotPath(paths), null);
  if (state && realpathSync(state.repo) !== realpathSync(paths.repo)) {
    throw new Error(
      `Codex dev mode belongs to ${state.repo}; run its prod toggle first`,
    );
  }
  return state;
}

export function codexDevMode(
  paths: CodexDevPaths,
): "dev" | "prod" | "mixed" | "not-installed" {
  readSnapshot(paths);
  const config = readConfig(paths);
  const hooks = readJson<Hooks>(join(paths.configDir, "hooks.json"), {
    hooks: {},
  });
  const commands = Object.values(selectHooks(hooks, true).hooks).flatMap(
    (matchers) =>
      matchers.flatMap((matcher) => matcher.hooks.map((hook) => hook.command)),
  );
  const states: boolean[] = [];
  if (config.mcp_servers?.aide) {
    const mcp = config.mcp_servers.aide;
    states.push(isLocalMcp(mcp, paths));
  }
  for (const [, plugin] of pluginEntries(config)) {
    if (plugin.enabled !== false && plugin.mcp_servers?.aide?.enabled !== false)
      states.push(false);
  }
  states.push(...commands.map((command) => isLocalHook(command, paths)));
  if (states.length === 0) {
    if (existsSync(snapshotPath(paths))) return "mixed";
    return pluginEntries(config).some(([, plugin]) => plugin.enabled !== false)
      ? "prod"
      : "not-installed";
  }
  return states.every(Boolean)
    ? "dev"
    : states.some(Boolean)
      ? "mixed"
      : "prod";
}

function validateSkillNames(names: string[]): void {
  if (!Array.isArray(names) || names.some((name) => !/^[\w-]+$/.test(name))) {
    throw new Error(
      "Invalid aide skills manifest; refusing to change skill directories",
    );
  }
}

// Migrate snapshots from the old toggle, which replaced installed skills with
// loose development copies. New toggles leave skill installation to the installer.
function restoreLegacySkills(paths: CodexDevPaths, state: Snapshot): void {
  validateSkillNames(state.skills);
  validateSkillNames(state.devSkills);
  if (state.skills.length === 0 && state.devSkills.length === 0) return;
  for (const name of new Set([...state.devSkills, ...state.skills])) {
    rmSync(join(paths.skillsDir, name), { recursive: true, force: true });
    const backup = join(paths.configDir, STATE_DIR, "skills", name);
    if (existsSync(backup))
      cpSync(backup, join(paths.skillsDir, name), { recursive: true });
  }
  const manifestPath = join(paths.skillsDir, MANIFEST);
  if (state.manifest === undefined) rmSync(manifestPath, { force: true });
  else writeFileSync(manifestPath, state.manifest);
  state.skills = [];
  state.devSkills = [];
  delete state.manifest;
}

export function switchCodexDev(
  paths: CodexDevPaths,
  mode: "dev" | "prod",
): string {
  const config = readConfig(paths);
  const hooksPath = join(paths.configDir, "hooks.json");
  const hooks = readJson<Hooks>(hooksPath, { hooks: {} });
  const backupDir = join(paths.configDir, STATE_DIR);
  let state = readSnapshot(paths);
  if (!state && codexDevMode(paths) === "not-installed")
    return "not installed - skipping";
  if (mode === "prod" && !state) {
    if (codexDevMode(paths) === "prod") return "already in prod mode";
  }

  const manifestPath = join(paths.skillsDir, MANIFEST);
  const owned = readJson<{ skills: string[] }>(manifestPath, {
    skills: [],
  }).skills;
  validateSkillNames(owned);

  if (mode === "dev" && !existsSync(join(paths.repo, "bin", BINARY))) {
    throw new Error(`Build bin/${BINARY} before enabling Codex dev mode`);
  }
  if (!state) {
    // Older installs could already point at this checkout, without a backup.
    // Give those local commands a published counterpart for the prod toggle.
    const prodHooks = selectHooks(hooks, true);
    for (const matchers of Object.values(prodHooks.hooks)) {
      for (const matcher of matchers) {
        matcher.hooks = matcher.hooks.map((hook) =>
          isLocalHook(hook.command, paths)
            ? {
                ...hook,
                command: hook.command.replace(
                  /^.*?\s+hook(?=\s|$)/,
                  "bunx -y @jmylchreest/aide-plugin hook",
                ),
              }
            : hook,
        );
      }
    }
    let prodMcp = config.mcp_servers?.aide;
    if (prodMcp && isLocalMcp(prodMcp, paths)) {
      prodMcp = {
        ...prodMcp,
        command: "bunx",
        args: ["-y", "@jmylchreest/aide-plugin", "mcp"],
        env: { ...prodMcp.env },
      };
      delete prodMcp.env.AIDE_PLUGIN_ROOT;
      delete prodMcp.env.CLAUDE_PLUGIN_ROOT;
    }
    state = {
      repo: realpathSync(paths.repo),
      mcp: prodMcp,
      plugins: Object.fromEntries(
        pluginEntries(config).map(([key, value]) => [
          key,
          {
            enabled: value.enabled,
            mcp: { enabled: value.mcp_servers?.aide?.enabled },
          },
        ]),
      ),
      hooksFeature: config.features?.hooks,
      hooks: prodHooks,
      skills: [],
      devSkills: [],
    };
    mkdirSync(backupDir, { recursive: true, mode: 0o700 });
    writeFileSync(snapshotPath(paths), JSON.stringify(state, null, 2) + "\n", {
      mode: 0o600,
    });
  }
  restoreLegacySkills(paths, state);
  if (mode === "dev") {
    for (const [key, plugin] of pluginEntries(config)) {
      if (!(key in state.plugins))
        state.plugins[key] = {
          enabled: plugin.enabled,
          mcp: { enabled: plugin.mcp_servers?.aide?.enabled },
        };
      const saved = state.plugins[key];
      if (!saved.mcp) {
        saved.mcp = { enabled: plugin.mcp_servers?.aide?.enabled };
      }
      // Old toggles disabled the entire plugin. Reapply the saved setting on
      // every run so migration also recovers if interrupted after saving state.
      if (saved.enabled === undefined) delete plugin.enabled;
      else plugin.enabled = saved.enabled;
      plugin.mcp_servers ??= {};
      plugin.mcp_servers.aide ??= {};
      plugin.mcp_servers.aide.enabled = false;
    }
    // Call the build directly: the published wrapper prefers npm's bundled binary.
    config.mcp_servers ??= {};
    config.mcp_servers.aide = {
      ...config.mcp_servers.aide,
      command: join(paths.repo, "bin", BINARY),
      args: ["mcp"],
      enabled: true,
      env: { ...config.mcp_servers.aide?.env, AIDE_PLUGIN_ROOT: paths.repo },
    };
    delete config.mcp_servers.aide.url;
    config.features ??= {};
    config.features.hooks = true;
    writeFileSync(snapshotPath(paths), JSON.stringify(state, null, 2) + "\n");
    writeFileSync(
      join(paths.configDir, "config.toml"),
      TOML.stringify(config) + "\n",
    );
    const prefix = `bun ${quoteArgument(join(paths.repo, "src", "cli", "index.ts"))} hook`;
    writeFileSync(
      hooksPath,
      JSON.stringify(mergeHooks(hooks, generateHooksJson(prefix)), null, 2) +
        "\n",
    );
    return "dev mode (local binary and hooks; installed skills unchanged)";
  }

  if (!state) throw new Error("Missing Codex dev snapshot");
  validateSkillNames(state.skills);
  validateSkillNames(state.devSkills);
  config.mcp_servers ??= {};
  if (state.mcp) config.mcp_servers.aide = state.mcp;
  else delete config.mcp_servers.aide;
  if (Object.keys(config.mcp_servers).length === 0) delete config.mcp_servers;
  for (const [key, saved] of Object.entries(state.plugins)) {
    const plugin = config.plugins?.[key];
    if (!plugin) continue;
    if (saved.enabled === undefined) delete plugin.enabled;
    else plugin.enabled = saved.enabled;
    if (saved.mcp && plugin.mcp_servers?.aide) {
      if (saved.mcp.enabled === undefined)
        delete plugin.mcp_servers.aide.enabled;
      else plugin.mcp_servers.aide.enabled = saved.mcp.enabled;
      if (Object.keys(plugin.mcp_servers.aide).length === 0)
        delete plugin.mcp_servers.aide;
      if (Object.keys(plugin.mcp_servers).length === 0)
        delete plugin.mcp_servers;
    }
  }
  config.features ??= {};
  if (state.hooksFeature === undefined) delete config.features.hooks;
  else config.features.hooks = state.hooksFeature;
  if (Object.keys(config.features).length === 0) delete config.features;
  writeFileSync(
    join(paths.configDir, "config.toml"),
    TOML.stringify(config) + "\n",
  );
  writeFileSync(
    hooksPath,
    JSON.stringify(mergeHooks(hooks, state.hooks), null, 2) + "\n",
  );
  rmSync(backupDir, { recursive: true });
  return "prod mode (saved setup restored)";
}
