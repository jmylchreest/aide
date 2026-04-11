/**
 * MCP Sync — cross-assistant MCP server configuration synchronization.
 *
 * Reads MCP server definitions from a canonical aide config file and
 * all known assistant config formats, merges them, and writes the result
 * back to the current assistant's config files.
 *
 * Supports:
 *   - Claude Code: ~/.claude.json (user), .mcp.json (project) (reads legacy ~/.mcp.json)
 *   - OpenCode:    ~/.config/opencode/opencode.json (user), ./opencode.json (project)
 *   - Codex CLI:   ~/.codex/config.toml (user), .codex/config.toml (project)
 *   - Aide canonical: ~/.aide/config/mcp.json (user), .aide/config/mcp.json (project)
 *
 * Uses a journal to detect intentional deletions from the aide canonical file,
 * preventing re-import of removed servers from assistant configs.
 */

import {
  existsSync,
  readFileSync,
  readdirSync,
  writeFileSync,
  mkdirSync,
  statSync,
} from "fs";
import { join, dirname } from "path";
import { homedir } from "os";
import * as TOML from "smol-toml";

// =============================================================================
// Types
// =============================================================================

/** Platform identifier for the current assistant. */
export type McpPlatform = "claude-code" | "opencode" | "codex";

/**
 * Discover MCP server names managed by installed Claude Code plugins.
 *
 * Scans plugin cache and marketplace directories for plugin.json files
 * and returns the set of MCP server names they define. These servers
 * must not be synced from assistant configs — doing so would override
 * the plugin's own definition and bypass CLAUDE_PLUGIN_ROOT.
 */
function getPluginManagedServers(): Set<string> {
  const names = new Set<string>();
  const pluginsDir = join(homedir(), ".claude", "plugins");

  // Scan both cache (installed plugins) and marketplaces (git-cloned plugins)
  const searchDirs = [
    join(pluginsDir, "cache"),
    join(pluginsDir, "marketplaces"),
  ];

  for (const baseDir of searchDirs) {
    if (!existsSync(baseDir)) continue;

    try {
      // Walk up to 3 levels deep to find .claude-plugin/plugin.json
      // cache: <marketplace>/<plugin>/<version>/.claude-plugin/plugin.json
      // marketplaces: <name>/.claude-plugin/plugin.json
      const findPluginJsons = (dir: string, depth: number): string[] => {
        if (depth > 3) return [];
        const results: string[] = [];
        const pluginJson = join(dir, ".claude-plugin", "plugin.json");
        if (existsSync(pluginJson)) results.push(pluginJson);

        try {
          for (const entry of readdirSync(dir, { withFileTypes: true })) {
            if (entry.isDirectory() && !entry.name.startsWith(".")) {
              results.push(...findPluginJsons(join(dir, entry.name), depth + 1));
            }
          }
        } catch {
          // Permission or read error — skip
        }
        return results;
      };

      for (const pluginJsonPath of findPluginJsons(baseDir, 0)) {
        try {
          const content = JSON.parse(readFileSync(pluginJsonPath, "utf-8"));
          if (content.mcpServers && typeof content.mcpServers === "object") {
            for (const serverName of Object.keys(content.mcpServers)) {
              names.add(serverName);
            }
          }
        } catch {
          // Skip unparseable plugin.json files
        }
      }
    } catch {
      // Directory read error — skip
    }
  }

  return names;
}

/** Scope level for config files. */
export type McpScope = "user" | "project";

/**
 * Canonical (aide-internal) MCP server definition.
 * Superset of all fields from Claude Code and OpenCode formats.
 */
export interface CanonicalMcpServer {
  /** Display/config name */
  name: string;
  /** "local" (stdio) or "remote" (http/sse) */
  type: "local" | "remote";
  /** Remote transport (http/sse) */
  transport?: "http" | "sse";
  /** Command to run (for local servers) */
  command?: string;
  /** Arguments (for local servers) */
  args?: string[];
  /** URL (for remote servers) */
  url?: string;
  /** Environment variables */
  env?: Record<string, string>;
  /** HTTP headers (for remote servers) */
  headers?: Record<string, string>;
  /** Whether the server is enabled (defaults to true) */
  enabled?: boolean;
}

/** The aide canonical MCP config file format. */
export interface AideMcpConfig {
  mcpServers: Record<string, Omit<CanonicalMcpServer, "name">>;
}

/** Per-server, per-source state entry in the journal. */
export interface ServerSourceState {
  /** Whether the server was present in this source at last check */
  present: boolean;
  /** Mtime of the source file when this state transition was detected */
  mtime: number;
  /** ISO timestamp for human readability */
  ts: string;
  /** Hash of the server config (to detect changes while present) */
  configHash?: string;
}

/**
 * Journal v2: per-server, per-source state tracking.
 *
 * Tracks the last state transition (added/removed/changed) for each
 * MCP server in each config source file, with the file's mtime at the
 * time of that transition. Resolution uses per-server latest-mtime-wins.
 */
export interface McpSyncJournal {
  version: 2;
  /** server name → source file path → state */
  servers: Record<string, Record<string, ServerSourceState>>;
}

/** Result of a sync operation. */
export interface McpSyncResult {
  /** Number of servers written to the current assistant's config */
  serversWritten: number;
  /** Number of new servers imported from other sources */
  imported: number;
  /** Number of servers skipped (in removed list) */
  skipped: number;
  /** Whether any config files were modified */
  modified: boolean;
  /** Servers that were written */
  serverNames: string[];
}

// =============================================================================
// Path helpers
// =============================================================================

function aideUserMcpPath(): string {
  return join(homedir(), ".aide", "config", "mcp.json");
}

function aideProjectMcpPath(cwd: string): string {
  return join(cwd, ".aide", "config", "mcp.json");
}

function journalPath(cwd: string): string {
  return join(cwd, ".aide", "config", "mcp-sync.journal.json");
}

function userJournalPath(): string {
  return join(homedir(), ".aide", "config", "mcp-sync.journal.json");
}

/** Get all config file paths for a given assistant and scope. */
function getAssistantReadPaths(
  platform: McpPlatform,
  scope: McpScope,
  cwd: string,
): string[] {
  if (platform === "claude-code") {
    return scope === "user"
      ? [join(homedir(), ".claude.json"), join(homedir(), ".mcp.json")]
      : [join(cwd, ".mcp.json")];
  }
  if (platform === "opencode") {
    return scope === "user"
      ? [join(homedir(), ".config", "opencode", "opencode.json")]
      : [join(cwd, "opencode.json")];
  }
  if (platform === "codex") {
    return scope === "user"
      ? [join(homedir(), ".codex", "config.toml")]
      : [join(cwd, ".codex", "config.toml")];
  }
  return [];
}

function getAssistantWritePaths(
  platform: McpPlatform,
  scope: McpScope,
  cwd: string,
): string[] {
  if (platform === "claude-code") {
    return scope === "user"
      ? [join(homedir(), ".claude.json")]
      : [join(cwd, ".mcp.json")];
  }
  if (platform === "opencode") {
    return scope === "user"
      ? [join(homedir(), ".config", "opencode", "opencode.json")]
      : [join(cwd, "opencode.json")];
  }
  if (platform === "codex") {
    return scope === "user"
      ? [join(homedir(), ".codex", "config.toml")]
      : [join(cwd, ".codex", "config.toml")];
  }
  return [];
}

// =============================================================================
// Config readers (normalize to canonical format)
// =============================================================================

/**
 * Read the aide canonical MCP config file.
 */
function readAideConfig(path: string): Record<string, CanonicalMcpServer> {
  if (!existsSync(path)) return {};

  try {
    const parsed: unknown = JSON.parse(readFileSync(path, "utf-8"));
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
      return {};
    const raw = parsed as AideMcpConfig;
    const mcpServers = raw.mcpServers;
    if (
      typeof mcpServers !== "object" ||
      mcpServers === null ||
      Array.isArray(mcpServers)
    )
      return {};
    const servers: Record<string, CanonicalMcpServer> = {};

    for (const [name, def] of Object.entries(mcpServers)) {
      servers[name] = { name, ...def };
    }

    return servers;
  } catch {
    return {};
  }
}

/**
 * Read Claude Code MCP config (~/.mcp.json or .mcp.json).
 *
 * Format:
 * ```json
 * {
 *   "mcpServers": {
 *     "name": {
 *       "type": "stdio",
 *       "command": "/usr/bin/npx",
 *       "args": ["package-name"],
 *       "env": {}
 *     }
 *   }
 * }
 * ```
 */
function readClaudeConfig(path: string): Record<string, CanonicalMcpServer> {
  if (!existsSync(path)) return {};

  try {
    const raw = JSON.parse(readFileSync(path, "utf-8"));
    const servers: Record<string, CanonicalMcpServer> = {};

    for (const [name, def] of Object.entries(
      (raw.mcpServers || {}) as Record<string, Record<string, unknown>>,
    )) {
      const claudeType =
        (def.type as string) ||
        ((def.url as string | undefined) ? "http" : "stdio");
      const isRemote =
        claudeType === "sse" || claudeType === "http" || !!def.url;

      servers[name] = {
        name,
        type: isRemote ? "remote" : "local",
        transport: isRemote
          ? claudeType === "sse"
            ? "sse"
            : "http"
          : undefined,
        command: def.command as string | undefined,
        args: def.args as string[] | undefined,
        url: def.url as string | undefined,
        env: (def.env as Record<string, string>) || undefined,
        headers: def.headers as Record<string, string> | undefined,
        enabled: true,
      };
    }

    return servers;
  } catch {
    return {};
  }
}

/**
 * Read OpenCode MCP config from opencode.json.
 *
 * Format:
 * ```json
 * {
 *   "mcp": {
 *     "name": {
 *       "type": "local",
 *       "command": ["npx", "package-name"],
 *       "environment": {},
 *       "enabled": true
 *     }
 *   }
 * }
 * ```
 */
function readOpenCodeConfig(path: string): Record<string, CanonicalMcpServer> {
  if (!existsSync(path)) return {};

  try {
    const raw = JSON.parse(readFileSync(path, "utf-8"));
    const servers: Record<string, CanonicalMcpServer> = {};

    for (const [name, def] of Object.entries(
      (raw.mcp || {}) as Record<string, Record<string, unknown>>,
    )) {
      const ocType = (def.type as string) || "local";
      const isRemote = ocType === "remote";

      // OpenCode uses "command": ["cmd", "arg1", "arg2"] — split into command + args
      const cmdArray = def.command as string[] | undefined;
      const command = cmdArray?.[0];
      const args = cmdArray?.slice(1);

      servers[name] = {
        name,
        type: isRemote ? "remote" : "local",
        transport: isRemote ? "http" : undefined,
        command,
        args: args?.length ? args : undefined,
        url: def.url as string | undefined,
        env: (def.environment as Record<string, string>) || undefined,
        headers: def.headers as Record<string, string> | undefined,
        enabled: def.enabled !== false,
      };
    }

    return servers;
  } catch {
    return {};
  }
}

/**
 * Read Codex CLI MCP config from config.toml.
 *
 * Format:
 * ```toml
 * [mcp_servers.name]
 * command = "npx"
 * args = ["package-name"]
 *
 * [mcp_servers.name.env]
 * KEY = "value"
 * ```
 */
function readCodexConfig(path: string): Record<string, CanonicalMcpServer> {
  if (!existsSync(path)) return {};

  try {
    const raw = TOML.parse(readFileSync(path, "utf-8"));
    const servers: Record<string, CanonicalMcpServer> = {};

    const mcpServers = raw.mcp_servers as
      | Record<string, Record<string, unknown>>
      | undefined;
    if (
      typeof mcpServers !== "object" ||
      mcpServers === null ||
      Array.isArray(mcpServers)
    )
      return {};

    for (const [name, def] of Object.entries(mcpServers)) {
      const hasUrl = typeof def.url === "string";
      const isRemote = hasUrl;

      servers[name] = {
        name,
        type: isRemote ? "remote" : "local",
        transport: isRemote ? "http" : undefined,
        command: def.command as string | undefined,
        args: def.args as string[] | undefined,
        url: def.url as string | undefined,
        env: (def.env as Record<string, string>) || undefined,
        headers: def.headers as Record<string, string> | undefined,
        enabled: true,
      };
    }

    return servers;
  } catch {
    return {};
  }
}

/**
 * Read MCP servers from a specific assistant's config file.
 */
function readAssistantConfig(
  platform: McpPlatform,
  path: string,
): Record<string, CanonicalMcpServer> {
  if (platform === "claude-code") return readClaudeConfig(path);
  if (platform === "opencode") return readOpenCodeConfig(path);
  if (platform === "codex") return readCodexConfig(path);
  return {};
}

// =============================================================================
// Config writers (convert canonical → assistant format)
// =============================================================================

/**
 * Write canonical servers to an aide MCP config file.
 */
function writeAideConfig(
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  const config: AideMcpConfig = { mcpServers: {} };
  for (const [name, server] of Object.entries(servers)) {
    const { name: _name, ...rest } = server;
    config.mcpServers[name] = rest;
  }

  writeFileSync(path, JSON.stringify(config, null, 2) + "\n");
}

/**
 * Write canonical servers to a Claude Code MCP config file.
 */
function writeClaudeConfig(
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Read existing file to preserve non-mcpServers keys
  let existing: Record<string, unknown> = {};
  if (existsSync(path)) {
    try {
      existing = JSON.parse(readFileSync(path, "utf-8"));
    } catch {
      // Start fresh
    }
  }

  const mcpServers: Record<string, Record<string, unknown>> = {};
  for (const [name, server] of Object.entries(servers)) {
    const entry: Record<string, unknown> = {};

    if (server.type === "remote") {
      entry.type = server.transport === "sse" ? "sse" : "http";
      if (server.url) entry.url = server.url;
      if (server.headers) entry.headers = server.headers;
    } else {
      entry.type = "stdio";
      if (server.command) entry.command = server.command;
      if (server.args) entry.args = server.args;
    }

    if (server.env && Object.keys(server.env).length > 0) {
      entry.env = server.env;
    } else if (server.type === "local") {
      entry.env = {};
    }

    mcpServers[name] = entry;
  }

  existing.mcpServers = mcpServers;
  writeFileSync(path, JSON.stringify(existing, null, 2) + "\n");
}

/**
 * Write canonical servers to an OpenCode config file.
 */
function writeOpenCodeConfig(
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Read existing file to preserve non-mcp keys
  let existing: Record<string, unknown> = {};
  if (existsSync(path)) {
    try {
      existing = JSON.parse(readFileSync(path, "utf-8"));
    } catch {
      // Start fresh
    }
  }

  // Ensure $schema is present
  if (!existing.$schema) {
    existing.$schema = "https://opencode.ai/config.json";
  }

  const mcp: Record<string, Record<string, unknown>> = {};
  for (const [name, server] of Object.entries(servers)) {
    const entry: Record<string, unknown> = {};

    if (server.type === "remote") {
      entry.type = "remote";
      if (server.url) entry.url = server.url;
      if (server.headers) entry.headers = server.headers;
    } else {
      entry.type = "local";
      // OpenCode uses "command": ["cmd", "arg1", ...]
      const cmdArray: string[] = [];
      if (server.command) cmdArray.push(server.command);
      if (server.args) cmdArray.push(...server.args);
      if (cmdArray.length > 0) entry.command = cmdArray;
    }

    if (server.env && Object.keys(server.env).length > 0) {
      entry.environment = server.env;
    }

    entry.enabled = server.enabled !== false;
    mcp[name] = entry;
  }

  existing.mcp = mcp;
  writeFileSync(path, JSON.stringify(existing, null, 2) + "\n");
}

/**
 * Write canonical servers to a Codex CLI config.toml file.
 *
 * Preserves all non-mcp_servers content in the file.
 */
function writeCodexConfig(
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Read existing file to preserve non-mcp_servers keys
  let existing: Record<string, unknown> = {};
  if (existsSync(path)) {
    try {
      existing = TOML.parse(readFileSync(path, "utf-8")) as Record<
        string,
        unknown
      >;
    } catch {
      // Start fresh
    }
  }

  const mcpServers: Record<string, Record<string, unknown>> = {};
  for (const [name, server] of Object.entries(servers)) {
    const entry: Record<string, unknown> = {};

    if (server.type === "remote") {
      if (server.url) entry.url = server.url;
      if (server.headers) entry.headers = server.headers;
    } else {
      if (server.command) entry.command = server.command;
      if (server.args) entry.args = server.args;
    }

    if (server.env && Object.keys(server.env).length > 0) {
      entry.env = server.env;
    }

    mcpServers[name] = entry;
  }

  existing.mcp_servers = mcpServers;
  writeFileSync(
    path,
    TOML.stringify(existing as Record<string, unknown>) + "\n",
  );
}

/**
 * Write canonical servers to the current assistant's config file.
 */
function writeAssistantConfig(
  platform: McpPlatform,
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  if (platform === "claude-code") writeClaudeConfig(path, servers);
  else if (platform === "opencode") writeOpenCodeConfig(path, servers);
  else if (platform === "codex") writeCodexConfig(path, servers);
}

// =============================================================================
// Journal (v2: per-server, per-source state tracking)
// =============================================================================

/** Legacy journal format (v1) — only used for migration. */
interface McpSyncJournalV1 {
  entries?: Array<{ ts: string; servers: string[] }>;
  removed?: string[];
}

function readJournal(path: string): McpSyncJournal {
  const empty: McpSyncJournal = { version: 2, servers: {} };
  if (!existsSync(path)) return empty;

  try {
    const parsed: unknown = JSON.parse(readFileSync(path, "utf-8"));
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
      return empty;

    const obj = parsed as Record<string, unknown>;

    // v2 journal
    if (obj.version === 2) return parsed as McpSyncJournal;

    // v1 → v2 migration: preserve the removed list as synthetic removal entries.
    // Use mtime=0 so any real source file with the server will override the
    // migration entry. If no source has the server, absence resolves it as removed.
    const v1 = parsed as McpSyncJournalV1;
    const migrated: McpSyncJournal = { version: 2, servers: {} };
    if (Array.isArray(v1.removed)) {
      for (const name of v1.removed) {
        migrated.servers[name] = {
          "migrated-v1": {
            present: false,
            mtime: 0,
            ts: new Date().toISOString(),
          },
        };
      }
    }
    return migrated;
  } catch {
    return empty;
  }
}

function writeJournal(path: string, journal: McpSyncJournal): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Prune server entries with no remaining sources
  for (const [serverName, sourceMap] of Object.entries(journal.servers)) {
    if (Object.keys(sourceMap).length === 0) {
      delete journal.servers[serverName];
    }
  }

  writeFileSync(path, JSON.stringify(journal, null, 2) + "\n");
}

/** Recursively sort object keys for deterministic serialization. */
function sortDeep(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sortDeep);
  if (v && typeof v === "object") {
    return Object.fromEntries(
      Object.keys(v as object)
        .sort()
        .map((k) => [k, sortDeep((v as Record<string, unknown>)[k])]),
    );
  }
  return v;
}

/**
 * Deterministic hash of a server config for change detection.
 * Normalizes away trivial differences (empty env, undefined fields)
 * and sorts all keys recursively for stable ordering.
 */
function serverConfigHash(server: CanonicalMcpServer): string {
  const { name: _, ...rest } = server;
  const normalized: Record<string, unknown> = {};
  const keys = Object.keys(rest).sort();
  for (const k of keys) {
    const v = (rest as Record<string, unknown>)[k];
    if (v === undefined) continue;
    if (
      typeof v === "object" &&
      v !== null &&
      !Array.isArray(v) &&
      Object.keys(v).length === 0
    )
      continue;
    normalized[k] = v;
  }
  return JSON.stringify(sortDeep(normalized));
}

// =============================================================================
// File modification time
// =============================================================================

function getFileMtime(path: string): number {
  try {
    return statSync(path).mtimeMs;
  } catch {
    return 0;
  }
}

interface McpSource {
  path: string;
  servers: Record<string, CanonicalMcpServer>;
  mtime: number;
}

/**
 * Gather all MCP config source files for a given scope.
 */
function gatherSources(scope: McpScope, cwd: string): McpSource[] {
  const platforms: McpPlatform[] = ["claude-code", "opencode", "codex"];
  const sources: McpSource[] = [];

  const aidePath =
    scope === "user" ? aideUserMcpPath() : aideProjectMcpPath(cwd);
  sources.push({
    path: aidePath,
    servers: readAideConfig(aidePath),
    mtime: getFileMtime(aidePath),
  });

  for (const platform of platforms) {
    const paths = getAssistantReadPaths(platform, scope, cwd);
    for (const p of paths) {
      sources.push({
        path: p,
        servers: readAssistantConfig(platform, p),
        mtime: getFileMtime(p),
      });
    }
  }

  return sources;
}

/**
 * Update the journal with per-server state transitions and resolve the
 * final server set using per-server latest-mtime-wins semantics.
 *
 * For each server in each source file, we track the last state transition
 * (present→removed or absent→present or config changed). The mtime stored
 * is the source file's mtime at the time of that transition. To resolve,
 * for each server we pick the entry with the highest mtime: if present the
 * server is included, if removed it is excluded.
 */
function updateJournalAndResolve(
  jrnlPath: string,
  sources: McpSource[],
): { resolved: Record<string, CanonicalMcpServer>; journal: McpSyncJournal } {
  const journal = readJournal(jrnlPath);
  const now = new Date().toISOString();
  const activePaths = new Set(sources.map((s) => s.path));

  // Capture servers already in the journal before we process state
  // transitions. The bootstrap phase should only backfill absence for
  // these pre-existing servers — not for servers discovered for the
  // first time in this run — to avoid treating "never existed in
  // another platform's config" as "intentionally removed".
  const preExistingServers = new Set(Object.keys(journal.servers));

  // Detect state transitions per server per source
  for (const source of sources) {
    const currentNames = new Set(Object.keys(source.servers));

    // Servers currently present in this source
    for (const [name, server] of Object.entries(source.servers)) {
      if (!journal.servers[name]) journal.servers[name] = {};
      const existing = journal.servers[name][source.path];
      const hash = serverConfigHash(server);

      if (!existing || !existing.present || existing.configHash !== hash) {
        // State transition: newly added, re-added, or config changed
        journal.servers[name][source.path] = {
          present: true,
          mtime: source.mtime,
          ts: now,
          configHash: hash,
        };
      }
      // If already present with same config: no transition, keep existing mtime
    }

    // Servers previously tracked in this source but now gone
    for (const [serverName, sourceMap] of Object.entries(journal.servers)) {
      const entry = sourceMap[source.path];
      if (entry?.present && !currentNames.has(serverName)) {
        sourceMap[source.path] = {
          present: false,
          mtime: source.mtime,
          ts: now,
        };
      }
    }
  }

  // Bootstrap: for servers that were already tracked in the journal
  // before this run, record "not present" entries for sources that
  // have never been tracked for that server. Without this, a server
  // deleted from one file before the v2 journal existed would have
  // no removal event to counterbalance presence in other files.
  // We only bootstrap pre-existing servers to avoid treating "never
  // existed in another platform's config" as an intentional removal.
  for (const serverName of preExistingServers) {
    const sourceMap = journal.servers[serverName];
    for (const source of sources) {
      if (!sourceMap[source.path]) {
        // This source has never been tracked for this server.
        // If the file exists (mtime > 0), record its absence.
        if (source.mtime > 0) {
          sourceMap[source.path] = {
            present: false,
            mtime: source.mtime,
            ts: now,
          };
        }
      }
    }
  }

  // Clean up journal entries for source files that no longer exist
  for (const sourceMap of Object.values(journal.servers)) {
    for (const sourcePath of Object.keys(sourceMap)) {
      if (sourcePath !== "migrated-v1" && !activePaths.has(sourcePath)) {
        delete sourceMap[sourcePath];
      }
    }
  }

  // Resolve: per server, latest mtime wins
  const resolved: Record<string, CanonicalMcpServer> = {};

  const allNames = new Set<string>();
  for (const source of sources) {
    for (const name of Object.keys(source.servers)) allNames.add(name);
  }
  for (const name of Object.keys(journal.servers)) allNames.add(name);

  for (const serverName of allNames) {
    const sourceMap = journal.servers[serverName];
    if (!sourceMap) continue;

    let bestMtime = -1;
    let bestPresent = false;
    let bestPath = "";

    for (const [sourcePath, state] of Object.entries(sourceMap)) {
      if (
        state.mtime > bestMtime ||
        (state.mtime === bestMtime && state.present && !bestPresent)
      ) {
        bestMtime = state.mtime;
        bestPresent = state.present;
        bestPath = sourcePath;
      }
    }

    if (bestPresent) {
      // Prefer the winning source; fall back to any source that has the server
      const source = sources.find((s) => s.path === bestPath);
      const server =
        source?.servers[serverName] ??
        sources.find((s) => s.servers[serverName])?.servers[serverName];
      if (server) resolved[serverName] = server;
    }
  }

  writeJournal(jrnlPath, journal);
  return { resolved, journal };
}

/**
 * Run MCP sync for a specific scope level.
 *
 * Gathers servers from all sources, updates the per-server journal to
 * track state transitions, resolves using latest-mtime-wins per server,
 * and writes the result to the aide canonical config and current assistant.
 */
function syncScope(
  platform: McpPlatform,
  scope: McpScope,
  cwd: string,
): McpSyncResult {
  const result: McpSyncResult = {
    serversWritten: 0,
    imported: 0,
    skipped: 0,
    modified: false,
    serverNames: [],
  };

  const aidePath =
    scope === "user" ? aideUserMcpPath() : aideProjectMcpPath(cwd);
  const aideServers = readAideConfig(aidePath);

  const jrnlPath = scope === "user" ? userJournalPath() : journalPath(cwd);
  const sources = gatherSources(scope, cwd);
  const { resolved: finalServers, journal } = updateJournalAndResolve(
    jrnlPath,
    sources,
  );

  // Count imports (servers not previously in aide config)
  for (const name of Object.keys(finalServers)) {
    if (!aideServers[name]) {
      result.imported++;
    }
  }

  // Count skipped (servers whose latest journal state is a removal)
  for (const [serverName, sourceMap] of Object.entries(journal.servers)) {
    if (finalServers[serverName]) continue;
    let bestMtime = -1;
    let bestPresent = false;
    for (const state of Object.values(sourceMap)) {
      if (state.mtime > bestMtime) {
        bestMtime = state.mtime;
        bestPresent = state.present;
      }
    }
    if (!bestPresent) result.skipped++;
  }

  // Write to aide canonical config (if changed) — includes ALL servers
  const sortedStringify = (obj: object) =>
    JSON.stringify(obj, Object.keys(obj).sort());
  const aideChanged =
    sortedStringify(aideServers) !== sortedStringify(finalServers);

  if (aideChanged) {
    writeAideConfig(aidePath, finalServers);
    result.modified = true;
  }

  // For Claude Code, exclude servers managed by plugins — these are
  // defined in plugin.json and must not be written to assistant configs
  // (doing so overrides the plugin's definition and bypasses
  // CLAUDE_PLUGIN_ROOT). The aide canonical config keeps them so they
  // can still sync to non-plugin assistants like OpenCode.
  let assistantServers = finalServers;
  if (platform === "claude-code") {
    const pluginManaged = getPluginManagedServers();
    if (pluginManaged.size > 0) {
      assistantServers = { ...finalServers };
      for (const name of pluginManaged) {
        delete assistantServers[name];
      }
    }
  }

  // Write to current assistant's config
  const assistantPaths = getAssistantWritePaths(platform, scope, cwd);
  for (const p of assistantPaths) {
    const existingAssistant = readAssistantConfig(platform, p);
    const assistantChanged =
      sortedStringify(existingAssistant) !== sortedStringify(assistantServers);

    if (assistantChanged) {
      writeAssistantConfig(platform, p, assistantServers);
      result.modified = true;
    }
  }

  result.serversWritten = Object.keys(assistantServers).length;
  result.serverNames = Object.keys(assistantServers);

  return result;
}

// =============================================================================
// Public API
// =============================================================================

/**
 * Run MCP sync at both user and project scope levels.
 *
 * Call this during session-start. It reads MCP server definitions from
 * the aide canonical config and all known assistant configs, merges them,
 * handles intentional deletions via a journal, and writes the result to
 * both the aide canonical config and the current assistant's config files.
 *
 * @param platform - The current assistant platform ("claude-code", "opencode", or "codex")
 * @param cwd - The project working directory
 * @returns Combined sync results for both user and project scopes
 */
export function syncMcpServers(
  platform: McpPlatform,
  cwd: string,
): { user: McpSyncResult; project: McpSyncResult } {
  const user = syncScope(platform, "user", cwd);
  const project = syncScope(platform, "project", cwd);

  return { user, project };
}

/**
 * Get the list of currently synced MCP servers (for display/logging).
 */
export function listSyncedServers(cwd: string): {
  user: string[];
  project: string[];
} {
  const userServers = readAideConfig(aideUserMcpPath());
  const projectServers = readAideConfig(aideProjectMcpPath(cwd));

  return {
    user: Object.keys(userServers),
    project: Object.keys(projectServers),
  };
}

/**
 * Get the current removed (blocked) server names.
 * Derived from the v2 journal: a server is "removed" if its latest
 * mtime across all sources corresponds to a removal event.
 */
export function getRemovedServers(cwd: string): {
  user: string[];
  project: string[];
} {
  function deriveRemoved(jrnlPath: string): string[] {
    const journal = readJournal(jrnlPath);
    const removed: string[] = [];
    for (const [serverName, sourceMap] of Object.entries(journal.servers)) {
      let bestMtime = -1;
      let bestPresent = false;
      for (const state of Object.values(sourceMap)) {
        if (state.mtime > bestMtime) {
          bestMtime = state.mtime;
          bestPresent = state.present;
        }
      }
      if (!bestPresent) removed.push(serverName);
    }
    return removed;
  }

  return {
    user: deriveRemoved(userJournalPath()),
    project: deriveRemoved(journalPath(cwd)),
  };
}
