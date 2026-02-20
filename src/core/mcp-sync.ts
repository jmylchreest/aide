/**
 * MCP Sync — cross-assistant MCP server configuration synchronization.
 *
 * Reads MCP server definitions from a canonical aide config file and
 * all known assistant config formats, merges them, and writes the result
 * back to the current assistant's config files.
 *
 * Supports:
 *   - Claude Code: ~/.mcp.json (user), .mcp.json (project)
 *   - OpenCode:    ~/.config/opencode/opencode.json (user), ./opencode.json (project)
 *   - Aide canonical: ~/.aide/config/mcp.json (user), .aide/config/mcp.json (project)
 *
 * Uses a journal to detect intentional deletions from the aide canonical file,
 * preventing re-import of removed servers from assistant configs.
 */

import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  statSync,
} from "fs";
import { join, dirname } from "path";
import { homedir } from "os";

// =============================================================================
// Types
// =============================================================================

/** Platform identifier for the current assistant. */
export type McpPlatform = "claude-code" | "opencode";

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

/** Journal tracking server presence across sync runs. */
export interface McpSyncJournal {
  /** Chronological entries of known server sets */
  entries: Array<{
    ts: string;
    servers: string[];
  }>;
  /** Server names intentionally removed from aide canonical config */
  removed: string[];
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
function getAssistantPaths(
  platform: McpPlatform,
  scope: McpScope,
  cwd: string,
): string[] {
  if (platform === "claude-code") {
    return scope === "user"
      ? [join(homedir(), ".mcp.json")]
      : [join(cwd, ".mcp.json")];
  }
  if (platform === "opencode") {
    return scope === "user"
      ? [join(homedir(), ".config", "opencode", "opencode.json")]
      : [join(cwd, "opencode.json")];
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
    const raw = JSON.parse(readFileSync(path, "utf-8")) as AideMcpConfig;
    const servers: Record<string, CanonicalMcpServer> = {};

    for (const [name, def] of Object.entries(raw.mcpServers || {})) {
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
      const claudeType = (def.type as string) || "stdio";
      const isRemote = claudeType === "sse" || !!def.url;

      servers[name] = {
        name,
        type: isRemote ? "remote" : "local",
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
 * Read MCP servers from a specific assistant's config file.
 */
function readAssistantConfig(
  platform: McpPlatform,
  path: string,
): Record<string, CanonicalMcpServer> {
  if (platform === "claude-code") return readClaudeConfig(path);
  if (platform === "opencode") return readOpenCodeConfig(path);
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
      entry.type = "sse";
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
 * Write canonical servers to the current assistant's config file.
 */
function writeAssistantConfig(
  platform: McpPlatform,
  path: string,
  servers: Record<string, CanonicalMcpServer>,
): void {
  if (platform === "claude-code") writeClaudeConfig(path, servers);
  else if (platform === "opencode") writeOpenCodeConfig(path, servers);
}

// =============================================================================
// Journal
// =============================================================================

function readJournal(path: string): McpSyncJournal {
  if (!existsSync(path)) {
    return { entries: [], removed: [] };
  }

  try {
    return JSON.parse(readFileSync(path, "utf-8")) as McpSyncJournal;
  } catch {
    return { entries: [], removed: [] };
  }
}

function writeJournal(path: string, journal: McpSyncJournal): void {
  const dir = dirname(path);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Keep only last 20 entries to avoid unbounded growth
  if (journal.entries.length > 20) {
    journal.entries = journal.entries.slice(-20);
  }

  writeFileSync(path, JSON.stringify(journal, null, 2) + "\n");
}

/**
 * Update the journal with the current set of aide canonical servers.
 * Detects deletions by comparing against the previous entry.
 *
 * Returns the set of server names that should be considered removed.
 */
function updateJournal(
  jrnlPath: string,
  currentAideServers: string[],
): string[] {
  const journal = readJournal(jrnlPath);
  const sorted = [...currentAideServers].sort();

  // Get previous entry's server list
  const prevEntry =
    journal.entries.length > 0
      ? journal.entries[journal.entries.length - 1]
      : null;
  const prevServers = prevEntry
    ? new Set(prevEntry.servers)
    : new Set<string>();

  // Detect deletions: servers in previous entry but not in current aide config
  const currentSet = new Set(sorted);
  for (const prev of prevServers) {
    if (!currentSet.has(prev)) {
      // Server was removed from aide config since last run
      if (!journal.removed.includes(prev)) {
        journal.removed.push(prev);
      }
    }
  }

  // If a server is now in the aide config AND in the removed list, un-remove it
  journal.removed = journal.removed.filter((r) => !currentSet.has(r));

  // Only write a new entry if the server list changed
  const prevSorted = prevEntry ? [...prevEntry.servers].sort() : [];
  const changed =
    sorted.length !== prevSorted.length ||
    sorted.some((s, i) => s !== prevSorted[i]);

  if (changed || !prevEntry) {
    journal.entries.push({
      ts: new Date().toISOString(),
      servers: sorted,
    });
  }

  writeJournal(jrnlPath, journal);

  return journal.removed;
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

// =============================================================================
// Core sync logic
// =============================================================================

/**
 * Collect all MCP servers from all sources for a given scope.
 *
 * Priority order (last-modified wins for conflicts):
 * 1. Aide canonical config (always included as base)
 * 2. Each assistant's config (sorted by file mtime, oldest first)
 */
function collectServers(
  scope: McpScope,
  cwd: string,
): Record<string, CanonicalMcpServer> {
  const platforms: McpPlatform[] = ["claude-code", "opencode"];

  // Collect all sources with their modification times
  const sources: Array<{
    label: string;
    servers: Record<string, CanonicalMcpServer>;
    mtime: number;
  }> = [];

  // Aide canonical config (always lowest priority as base)
  const aidePath =
    scope === "user" ? aideUserMcpPath() : aideProjectMcpPath(cwd);
  sources.push({
    label: `aide:${scope}`,
    servers: readAideConfig(aidePath),
    mtime: getFileMtime(aidePath),
  });

  // Assistant configs
  for (const platform of platforms) {
    const paths = getAssistantPaths(platform, scope, cwd);
    for (const p of paths) {
      const servers = readAssistantConfig(platform, p);
      if (Object.keys(servers).length > 0) {
        sources.push({
          label: `${platform}:${scope}:${p}`,
          servers,
          mtime: getFileMtime(p),
        });
      }
    }
  }

  // Sort by mtime (oldest first, so newest overwrites)
  sources.sort((a, b) => a.mtime - b.mtime);

  // Merge: later entries override earlier ones for same server name
  const merged: Record<string, CanonicalMcpServer> = {};
  for (const source of sources) {
    for (const [name, server] of Object.entries(source.servers)) {
      merged[name] = server;
    }
  }

  return merged;
}

/**
 * Run MCP sync for a specific scope level.
 *
 * 1. Reads the aide canonical config
 * 2. Updates the journal to detect deletions
 * 3. Collects servers from all sources
 * 4. Filters out removed servers
 * 5. Writes merged result to the aide canonical config
 * 6. Writes the result (in assistant-native format) to the current assistant's config
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

  // Step 1: Read current aide canonical config
  const aidePath =
    scope === "user" ? aideUserMcpPath() : aideProjectMcpPath(cwd);
  const aideServers = readAideConfig(aidePath);
  const aideServerNames = Object.keys(aideServers);

  // Step 2: Update journal and get removed list
  const jrnlPath = scope === "user" ? userJournalPath() : journalPath(cwd);
  const removed = updateJournal(jrnlPath, aideServerNames);
  const removedSet = new Set(removed);

  // Step 3: Collect all servers from all sources
  const allServers = collectServers(scope, cwd);

  // Step 4: Filter out removed servers
  const finalServers: Record<string, CanonicalMcpServer> = {};
  for (const [name, server] of Object.entries(allServers)) {
    if (removedSet.has(name)) {
      result.skipped++;
      continue;
    }
    finalServers[name] = server;
  }

  // Step 5: Count imports (servers not previously in aide config)
  for (const name of Object.keys(finalServers)) {
    if (!aideServers[name]) {
      result.imported++;
    }
  }

  // Step 6: Write to aide canonical config (if changed)
  const aideChanged =
    JSON.stringify(aideServers) !==
    JSON.stringify(
      Object.fromEntries(Object.entries(finalServers).map(([k, v]) => [k, v])),
    );

  if (aideChanged) {
    writeAideConfig(aidePath, finalServers);
    result.modified = true;
  }

  // Step 7: Write to current assistant's config
  const assistantPaths = getAssistantPaths(platform, scope, cwd);
  for (const p of assistantPaths) {
    // Read existing assistant config to check if it needs updating
    const existingAssistant = readAssistantConfig(platform, p);
    const assistantChanged =
      JSON.stringify(existingAssistant) !== JSON.stringify(finalServers);

    if (assistantChanged) {
      writeAssistantConfig(platform, p, finalServers);
      result.modified = true;
    }
  }

  result.serversWritten = Object.keys(finalServers).length;
  result.serverNames = Object.keys(finalServers);

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
 * @param platform - The current assistant platform ("claude-code" or "opencode")
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
 */
export function getRemovedServers(cwd: string): {
  user: string[];
  project: string[];
} {
  const userJournal = readJournal(userJournalPath());
  const projectJournal = readJournal(journalPath(cwd));

  return {
    user: userJournal.removed,
    project: projectJournal.removed,
  };
}
