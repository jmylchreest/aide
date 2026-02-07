#!/usr/bin/env node
/**
 * Session Start Hook (SessionStart)
 *
 * Initializes aide state and configuration on session start.
 * - Creates .aide directories if needed
 * - Loads config files
 * - Initializes HUD state
 * - Cleans up stale state from previous sessions
 *
 * Debug logging: Set AIDE_DEBUG=1 to enable startup tracing
 * Logs written to: .aide/_logs/startup.log
 */

import { existsSync, readFileSync, writeFileSync, mkdirSync, readdirSync, unlinkSync, statSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';
import { execSync } from 'child_process';
import { Logger, debug, setDebugCwd } from '../lib/logger.js';
import { readStdin, updateSessionHeartbeat, getSessionHeartbeats, runAide } from '../lib/hook-utils.js';

const SOURCE = 'session-start';
debug(SOURCE, `Hook started (AIDE_DEBUG=${process.env.AIDE_DEBUG || 'unset'})`);

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  continue: boolean;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

interface MemoryConfig {
  injection: {
    static: { enabled: boolean; categories: string[] };
    dynamic: { enabled: boolean; defaultCount: number; category: string };
  };
  modeOverrides: Record<string, { dynamicCount?: number }>;
}

interface AideConfig {
  tiers: Record<string, string>;
  aliases: Record<string, string>;
  hud: {
    enabled: boolean;
    elements: string[];
  };
  memory?: MemoryConfig;
}

interface SessionState {
  sessionId: string;
  startedAt: string;
  cwd: string;
  activeMode: string | null;
  agentCount: number;
  taskCount: number;
}

const DEFAULT_CONFIG: AideConfig = {
  tiers: {
    fast: 'Cheapest/fastest model',
    balanced: 'Good cost/capability balance',
    smart: 'Most capable model',
  },
  aliases: {
    opus: 'smart',
    sonnet: 'balanced',
    haiku: 'fast',
    cheap: 'fast',
    quick: 'fast',
    thorough: 'smart',
    best: 'smart',
  },
  hud: {
    enabled: true,
    elements: ['mode', 'model', 'agents', 'tasks'],
  },
  memory: {
    injection: {
      static: {
        enabled: true,
        categories: ['global', 'decision'],
      },
      dynamic: {
        enabled: true,
        defaultCount: 3,
        category: 'session',
      },
    },
    modeOverrides: {
      autopilot: { dynamicCount: 5 },
      ralph: { dynamicCount: 5 },
      eco: { dynamicCount: 1 },
    },
  },
};

/**
 * Find the aide binary path
 * @param cwd - Optional project directory to check for local binary
 */
function findAideBinary(cwd?: string): string | null {
  // Check CLAUDE_PLUGIN_ROOT first (set by Claude Code when running plugin)
  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT;
  if (pluginRoot) {
    const pluginBinary = join(pluginRoot, 'bin', 'aide');
    if (existsSync(pluginBinary)) {
      return pluginBinary;
    }
  }

  // Check project-local bin directory (for development)
  if (cwd) {
    const localBinary = join(cwd, 'bin', 'aide');
    if (existsSync(localBinary)) {
      return localBinary;
    }
  }

  // Check home directory
  const homeBinary = join(homedir(), '.aide', 'bin', 'aide');
  if (existsSync(homeBinary)) {
    return homeBinary;
  }

  // Check if in PATH
  try {
    execSync('aide --help', { stdio: 'ignore', timeout: 2000 });
    return 'aide';
  } catch {
    // Not in PATH
  }

  return null;
}

/**
 * Read the plugin version from package.json.
 * Tries CLAUDE_PLUGIN_ROOT first, then relative to this script.
 */
function getPluginVersion(): string | null {
  const candidates: string[] = [];

  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT;
  if (pluginRoot) {
    candidates.push(join(pluginRoot, 'package.json'));
  }

  // Relative to compiled dist/hooks/session-start.js → ../../package.json
  try {
    const scriptDir = new URL('.', import.meta.url).pathname;
    candidates.push(join(scriptDir, '..', '..', 'package.json'));
  } catch {
    // import.meta.url not available
  }

  for (const candidate of candidates) {
    try {
      if (existsSync(candidate)) {
        const pkg = JSON.parse(readFileSync(candidate, 'utf-8'));
        if (pkg.version && pkg.version !== '0.0.0') {
          return pkg.version;
        }
      }
    } catch {
      // skip invalid files
    }
  }

  return null;
}

/**
 * Get the download URL for the current platform.
 * Pins to the version in package.json when available,
 * falls back to latest release otherwise.
 */
function getDownloadUrl(): string {
  const platform = process.platform; // 'darwin', 'linux', 'win32'
  const arch = process.arch; // 'x64', 'arm64'

  const goos = platform === 'win32' ? 'windows' : platform;
  const goarch = arch === 'x64' ? 'amd64' : arch;
  const ext = platform === 'win32' ? '.exe' : '';

  const binaryName = `aide-${goos}-${goarch}${ext}`;

  const version = getPluginVersion();
  if (version) {
    return `https://github.com/jmylchreest/aide/releases/download/v${version}/${binaryName}`;
  }

  // Fallback to latest if version can't be determined
  return `https://github.com/jmylchreest/aide/releases/latest/download/${binaryName}`;
}

/**
 * Download aide binary from GitHub releases
 */
async function downloadAideBinary(destDir: string, log: Logger): Promise<string | null> {
  const url = getDownloadUrl();
  const ext = process.platform === 'win32' ? '.exe' : '';
  const destPath = join(destDir, `aide${ext}`);

  log.info(`Downloading aide from ${url}...`);

  try {
    // Create bin directory
    if (!existsSync(destDir)) {
      mkdirSync(destDir, { recursive: true });
    }

    // Use curl to download (available on all platforms)
    execSync(`curl -fsSL "${url}" -o "${destPath}"`, {
      stdio: 'pipe',
      timeout: 60000, // 60 second timeout
    });

    // Make executable
    if (process.platform !== 'win32') {
      execSync(`chmod +x "${destPath}"`, { stdio: 'pipe' });
    }

    log.info(`Downloaded aide to ${destPath}`);
    return destPath;
  } catch (err) {
    log.warn(`Failed to download aide: ${err}`);
    return null;
  }
}

/**
 * Ensure aide binary is present, download if missing
 * @param cwd - Project directory to check for local binary
 * @returns Object with binary path (if found) and any error message to show user
 */
async function ensureAideBinary(cwd: string, log: Logger): Promise<{ binary: string | null; error: string | null }> {
  log.start('ensureAideBinary');

  // Check if already exists (including project-local bin directory)
  let binary = findAideBinary(cwd);
  if (binary) {
    log.end('ensureAideBinary', { found: true, path: binary });
    return { binary, error: null };
  }

  // Try to download
  log.info('aide binary not found, attempting download...');

  // Determine install location
  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT;
  const installDir = pluginRoot
    ? join(pluginRoot, 'bin')
    : join(homedir(), '.aide', 'bin');

  binary = await downloadAideBinary(installDir, log);

  if (binary) {
    log.end('ensureAideBinary', { found: true, path: binary, downloaded: true });
    return { binary, error: null };
  }

  log.warn('aide binary not found and download failed - some features may be unavailable');
  log.end('ensureAideBinary', { found: false });

  // Return error message for user
  const errorMsg = `**AIDE Setup Required**

The aide binary could not be found or downloaded automatically.

**To fix this, choose one option:**

1. **Build from source** (if you cloned the repo):
   \`\`\`bash
   cd ${pluginRoot || '~/aide'}/aide && go build -o ../bin/aide ./cmd/aide
   \`\`\`

2. **Download manually** from GitHub releases:
   https://github.com/jmylchreest/aide/releases

   Place the binary at: \`${installDir}/aide\`

Some AIDE features (memory, code search, decisions) will be unavailable until resolved.`;

  return { binary: null, error: errorMsg };
}

/**
 * Reset aide state for new session
 * Resets global session state - preserves per-agent state
 */
function resetAideState(cwd: string, log: Logger): void {
  log.start('resetAideState');

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug('aide binary not found, skipping state reset');
    log.end('resetAideState', { skipped: true, reason: 'no-binary' });
    return;
  }

  const dbPath = join(cwd, '.aide', 'memory', 'store.db');
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  // Global state keys to reset at session start
  // Per-agent state (prefixed with agent:) is preserved
  const keysToReset = ['mode', 'startedAt', 'lastToolUse', 'toolCalls', 'taskCount', 'agentCount', 'lastTool'];

  try {
    for (const key of keysToReset) {
      try {
        execSync(`"${binary}" state delete ${key}`, { env, stdio: 'pipe', timeout: 5000 });
      } catch {
        // Key might not exist, that's fine
      }
    }
    log.debug(`Reset global state keys: ${keysToReset.join(', ')}`);

    log.debug('State reset complete');

    log.end('resetAideState', { success: true });
  } catch (err) {
    // Non-fatal - state reset is nice-to-have
    log.warn('Failed to reset aide state', err);
    log.end('resetAideState', { success: false, error: String(err) });
  }
}

/**
 * Reset HUD state file for clean session start
 * Clears the HUD display so it starts fresh
 */
function resetHudState(cwd: string, log: Logger): void {
  log.start('resetHudState');

  const hudPath = join(cwd, '.aide', 'state', 'hud.txt');

  try {
    if (existsSync(hudPath)) {
      // Write empty/idle HUD state
      writeFileSync(hudPath, 'mode:idle');
      log.debug('HUD state reset to idle');
    }
    log.end('resetHudState', { success: true });
  } catch (err) {
    // Non-fatal
    log.warn('Failed to reset HUD state', err);
    log.end('resetHudState', { success: false, error: String(err) });
  }
}

/**
 * Ensure all .aide directories exist
 */
function ensureDirectories(cwd: string, log: Logger): void {
  const dirs = [
    join(cwd, '.aide'),
    join(cwd, '.aide', 'skills'),
    join(cwd, '.aide', 'config'),
    join(cwd, '.aide', 'state'),
    join(cwd, '.aide', 'memory'),
    join(cwd, '.aide', 'worktrees'),
    join(cwd, '.aide', '_logs'),  // Log directory
    join(homedir(), '.aide'),
    join(homedir(), '.aide', 'skills'),
    join(homedir(), '.aide', 'config'),
  ];

  log.start('ensureDirectories');
  let created = 0;
  let existed = 0;

  for (const dir of dirs) {
    if (!existsSync(dir)) {
      try {
        mkdirSync(dir, { recursive: true });
        created++;
        log.debug(`Created directory: ${dir}`);
      } catch (err) {
        log.warn(`Failed to create directory: ${dir}`, err);
      }
    } else {
      existed++;
    }
  }

  // Ensure .gitignore exists in .aide directory
  const gitignorePath = join(cwd, '.aide', '.gitignore');
  if (!existsSync(gitignorePath)) {
    try {
      const gitignoreContent = `# AIDE runtime files - do not commit
_logs/
state/
*.db
*.bleve/
`;
      writeFileSync(gitignorePath, gitignoreContent);
      log.debug(`Created .gitignore: ${gitignorePath}`);
    } catch (err) {
      log.warn(`Failed to create .gitignore: ${gitignorePath}`, err);
    }
  }

  log.end('ensureDirectories', { total: dirs.length, created, existed });
}

/**
 * Load or create config file
 */
function loadConfig(cwd: string, log: Logger): AideConfig {
  const configPath = join(cwd, '.aide', 'config', 'aide.json');

  log.start('loadConfig');

  if (existsSync(configPath)) {
    try {
      const content = readFileSync(configPath, 'utf-8');
      log.end('loadConfig', { source: 'existing', path: configPath });
      return { ...DEFAULT_CONFIG, ...JSON.parse(content) };
    } catch (err) {
      log.warn(`Failed to parse config: ${configPath}`, err);
      log.end('loadConfig', { source: 'default', reason: 'parse-error' });
      return DEFAULT_CONFIG;
    }
  }

  // Create default config
  try {
    writeFileSync(configPath, JSON.stringify(DEFAULT_CONFIG, null, 2));
    log.debug(`Created default config: ${configPath}`);
  } catch (err) {
    log.warn(`Failed to write default config: ${configPath}`, err);
  }

  log.end('loadConfig', { source: 'default', reason: 'not-found' });
  return DEFAULT_CONFIG;
}

/**
 * Clean up stale state files older than 24 hours
 */
function cleanupStaleState(cwd: string, log: Logger): void {
  const stateDir = join(cwd, '.aide', 'state');
  if (!existsSync(stateDir)) {
    log.debug('cleanupStaleState: state directory does not exist');
    return;
  }

  log.start('cleanupStaleState');

  const now = Date.now();
  const maxAge = 24 * 60 * 60 * 1000; // 24 hours
  let scanned = 0;
  let deleted = 0;

  try {
    const files = readdirSync(stateDir);
    for (const file of files) {
      // Don't clean up session or config files
      if (file.endsWith('-state.json') || file === 'session.json') {
        scanned++;
        const filePath = join(stateDir, file);
        const stats = statSync(filePath);
        const age = now - stats.mtimeMs;
        if (age > maxAge) {
          try {
            unlinkSync(filePath);
            deleted++;
            log.debug(`Deleted stale file: ${file} (age: ${Math.round(age / 3600000)}h)`);
          } catch (err) {
            log.warn(`Failed to delete stale file: ${file}`, err);
          }
        }
      }
    }
  } catch (err) {
    log.warn('Failed to read state directory', err);
  }

  log.end('cleanupStaleState', { scanned, deleted });
}

/**
 * Clean up agent state from dead sessions
 *
 * A session is considered dead if:
 * - Its last heartbeat was more than 30 minutes ago
 * - It has agents in "running" status (orphaned agents)
 *
 * This prevents stale HUD entries when sessions crash without cleanup.
 */
function cleanupDeadSessionAgents(cwd: string, currentSessionId: string, log: Logger): void {
  log.start('cleanupDeadSessionAgents');

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug('aide binary not found, skipping dead session cleanup');
    log.end('cleanupDeadSessionAgents', { skipped: true, reason: 'no-binary' });
    return;
  }

  const dbPath = join(cwd, '.aide', 'memory', 'store.db');
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  // Get all session heartbeats
  const heartbeats = getSessionHeartbeats(cwd);
  const now = Date.now();
  const heartbeatThreshold = 30 * 60 * 1000; // 30 minutes

  // Find dead sessions (no heartbeat or heartbeat too old)
  const deadSessions = new Set<string>();
  for (const [sessionId, lastSeen] of heartbeats) {
    if (sessionId !== currentSessionId && (now - lastSeen) > heartbeatThreshold) {
      deadSessions.add(sessionId);
      log.debug(`Session ${sessionId.slice(0, 8)} is dead (last seen ${Math.round((now - lastSeen) / 60000)}m ago)`);
    }
  }

  if (deadSessions.size === 0) {
    log.debug('No dead sessions found');
    log.end('cleanupDeadSessionAgents', { deadSessions: 0, cleaned: 0 });
    return;
  }

  // Get all agent state and find orphaned agents
  const output = runAide(cwd, ['state', 'list']);
  if (!output) {
    log.end('cleanupDeadSessionAgents', { skipped: true, reason: 'state-list-failed' });
    return;
  }

  // Parse agent sessions from state output
  const agentSessions = new Map<string, string>(); // agentId -> sessionId
  for (const line of output.split('\n')) {
    // Match: [agentId] agent:<agentId>:session = <sessionId>
    const match = line.match(/^\[([^\]]+)\]\s+agent:[^:]+:session\s*=\s*(.+)/);
    if (match) {
      agentSessions.set(match[1], match[2].trim());
    }
  }

  // Clean up agents belonging to dead sessions
  let cleaned = 0;
  for (const [agentId, sessionId] of agentSessions) {
    if (deadSessions.has(sessionId)) {
      try {
        execSync(`"${binary}" state clear --agent=${agentId}`, { env, stdio: 'pipe', timeout: 5000 });
        cleaned++;
        log.debug(`Cleaned up orphaned agent ${agentId.slice(0, 8)} from dead session ${sessionId.slice(0, 8)}`);
      } catch (err) {
        log.warn(`Failed to clean up agent ${agentId}`, err);
      }
    }
  }

  // Also clean up the dead session heartbeats
  for (const sessionId of deadSessions) {
    try {
      execSync(`"${binary}" state delete session:${sessionId}:lastSeen`, { env, stdio: 'pipe', timeout: 5000 });
    } catch {
      // Key might not exist
    }
  }

  log.info(`Cleaned up ${cleaned} orphaned agents from ${deadSessions.size} dead sessions`);
  log.end('cleanupDeadSessionAgents', { deadSessions: deadSessions.size, cleaned });
}

/**
 * Clean up stale agent state based on TTL (3 hours)
 *
 * Removes agent state that hasn't been updated in 3 hours, regardless of session.
 * This prevents accumulation of old agent data in the database.
 */
function cleanupStaleAgentsByTTL(cwd: string, currentSessionId: string, log: Logger): void {
  log.start('cleanupStaleAgentsByTTL');

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug('aide binary not found, skipping TTL cleanup');
    log.end('cleanupStaleAgentsByTTL', { skipped: true, reason: 'no-binary' });
    return;
  }

  const dbPath = join(cwd, '.aide', 'memory', 'store.db');
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  const output = runAide(cwd, ['state', 'list']);
  if (!output) {
    log.end('cleanupStaleAgentsByTTL', { skipped: true, reason: 'state-list-failed' });
    return;
  }

  const now = Date.now();
  const ttlMs = 3 * 60 * 60 * 1000; // 3 hours

  // Parse agent timestamps from state output
  // Track: agentId -> { startedAt, endedAt, session }
  const agentData = new Map<string, { startedAt?: number; endedAt?: number; session?: string }>();

  for (const line of output.split('\n')) {
    // Match: [agentId] agent:<agentId>:<key> = <value>
    const match = line.match(/^\[([^\]]+)\]\s+agent:[^:]+:(\w+)\s*=\s*(.+)/);
    if (match) {
      const [, agentId, key, value] = match;
      if (!agentData.has(agentId)) {
        agentData.set(agentId, {});
      }
      const data = agentData.get(agentId)!;

      if (key === 'startedAt') {
        data.startedAt = new Date(value.trim()).getTime();
      } else if (key === 'endedAt') {
        data.endedAt = new Date(value.trim()).getTime();
      } else if (key === 'session') {
        data.session = value.trim();
      }
    }
  }

  // Find and clean stale agents
  let cleaned = 0;
  for (const [agentId, data] of agentData) {
    // Skip current session's agents
    if (data.session === currentSessionId) {
      continue;
    }

    // Use endedAt if available, otherwise startedAt
    const lastUpdate = data.endedAt || data.startedAt;
    if (!lastUpdate) {
      // No timestamp - consider stale
      try {
        execSync(`"${binary}" state clear --agent=${agentId}`, { env, stdio: 'pipe', timeout: 5000 });
        cleaned++;
        log.debug(`Cleaned up agent ${agentId.slice(0, 8)} (no timestamp)`);
      } catch {
        // Ignore errors
      }
      continue;
    }

    const age = now - lastUpdate;
    if (age > ttlMs) {
      try {
        execSync(`"${binary}" state clear --agent=${agentId}`, { env, stdio: 'pipe', timeout: 5000 });
        cleaned++;
        log.debug(`Cleaned up stale agent ${agentId.slice(0, 8)} (age: ${Math.round(age / 3600000)}h)`);
      } catch {
        // Ignore errors
      }
    }
  }

  if (cleaned > 0) {
    log.info(`Cleaned up ${cleaned} stale agents (TTL: 3h)`);
  }
  log.end('cleanupStaleAgentsByTTL', { scanned: agentData.size, cleaned });
}

/**
 * Initialize session state
 */
function initializeSession(sessionId: string, cwd: string, log: Logger): SessionState {
  log.start('initializeSession');

  const state: SessionState = {
    sessionId,
    startedAt: new Date().toISOString(),
    cwd,
    activeMode: null,
    agentCount: 0,
    taskCount: 0,
  };

  const statePath = join(cwd, '.aide', 'state', 'session.json');
  try {
    writeFileSync(statePath, JSON.stringify(state, null, 2));
    log.debug(`Session state written: ${statePath}`);
  } catch (err) {
    log.warn(`Failed to write session state: ${statePath}`, err);
  }

  log.end('initializeSession', { sessionId: sessionId.slice(0, 8) });
  return state;
}

/**
 * Get project name from git remote or directory name
 */
function getProjectName(cwd: string): string {
  try {
    // Try git remote first
    const remoteUrl = execSync('git config --get remote.origin.url', {
      cwd,
      stdio: ['pipe', 'pipe', 'pipe'],
      timeout: 2000,
    }).toString().trim();

    // Extract repo name from URL
    const match = remoteUrl.match(/[/:]([^/]+?)(?:\.git)?$/);
    if (match) return match[1];
  } catch {
    // Not a git repo or no remote
  }

  // Fallback to directory name
  return cwd.split('/').pop() || 'unknown';
}

interface MemoryInjection {
  static: {
    global: string[];      // scope:global memories
    decisions: string[];   // project decisions
  };
  dynamic: {
    sessions: string[];    // recent session summaries
  };
}

/**
 * Fetch memories for context injection with proper scoping
 *
 * Static (always inject):
 *   - category=global with scope:global tag (cross-project preferences)
 *   - category=decision with project:<name> tag (project decisions)
 *
 * Dynamic (recent N sessions):
 *   - All memories from the N most recent sessions for this project
 *
 * Disable with: AIDE_MEMORY_INJECT=0
 */
function fetchMemories(cwd: string, config: AideConfig, log: Logger): MemoryInjection {
  log.start('fetchMemories');

  // Check for disable flag
  if (process.env.AIDE_MEMORY_INJECT === '0') {
    log.info('Memory injection disabled via AIDE_MEMORY_INJECT=0');
    log.end('fetchMemories', { skipped: true, reason: 'disabled' });
    return {
      static: { global: [], decisions: [] },
      dynamic: { sessions: [] },
    };
  }

  const result: MemoryInjection = {
    static: { global: [], decisions: [] },
    dynamic: { sessions: [] },
  };

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug('aide binary not found, skipping memory fetch');
    log.end('fetchMemories', { skipped: true, reason: 'no-binary' });
    return result;
  }

  const dbPath = join(cwd, '.aide', 'memory', 'store.db');
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
  const projectName = getProjectName(cwd);
  const memConfig = config.memory || DEFAULT_CONFIG.memory!;

  // 1. Fetch STATIC: Global memories (scope:global)
  if (memConfig.injection.static.enabled) {
    try {
      const globalOutput = execSync(
        `"${binary}" memory list --category=global --tags=scope:global --format=json`,
        { env, stdio: ['pipe', 'pipe', 'pipe'], timeout: 5000 }
      ).toString().trim();

      if (globalOutput && globalOutput !== '[]') {
        const memories = JSON.parse(globalOutput);
        result.static.global = memories.map((m: { content: string }) => m.content);
        log.debug(`Loaded ${result.static.global.length} global memories`);
      }
    } catch (err) {
      log.debug('No global memories or error fetching', err);
    }

    // 2. Fetch STATIC: Project decisions
    try {
      const decisionsOutput = execSync(
        `"${binary}" decision list --format=json`,
        { env, stdio: ['pipe', 'pipe', 'pipe'], timeout: 5000 }
      ).toString().trim();

      if (decisionsOutput && decisionsOutput !== '[]') {
        const decisions = JSON.parse(decisionsOutput);
        result.static.decisions = decisions.map((d: { topic: string; value: string; rationale?: string }) =>
          `**${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ''}`
        );
        log.debug(`Loaded ${result.static.decisions.length} project decisions`);
      }
    } catch (err) {
      log.debug('No decisions or error fetching', err);
    }
  }

  // 3. Fetch DYNAMIC: Recent sessions (all memories from N most recent sessions)
  if (memConfig.injection.dynamic.enabled) {
    // Allow override via env var: AIDE_MEMORY_INJECT_SESSION_COUNT (default from config, usually 3)
    let sessionCount = memConfig.injection.dynamic.defaultCount;
    const envCount = process.env.AIDE_MEMORY_INJECT_SESSION_COUNT;
    if (envCount) {
      const parsed = parseInt(envCount, 10);
      if (!isNaN(parsed) && parsed >= 0) {
        sessionCount = parsed;
        log.debug(`Session count overridden via env: ${sessionCount}`);
      }
    }

    try {
      const sessionsOutput = execSync(
        `"${binary}" memory sessions --project=${projectName} --limit=${sessionCount} --format=json`,
        { env, stdio: ['pipe', 'pipe', 'pipe'], timeout: 5000 }
      ).toString().trim();

      if (sessionsOutput && sessionsOutput !== '[]') {
        const sessions = JSON.parse(sessionsOutput);
        // Format each session with all its memories
        for (const sess of sessions) {
          const timeAgo = sess.last_at ? formatTimeAgo(sess.last_at) : '';
          const header = `Session ${sess.session_id}${timeAgo ? ` (${timeAgo})` : ''}`;
          const memories = sess.memories.map((m: { content: string; category: string }) =>
            `- [${m.category}] ${m.content}`
          ).join('\n');
          result.dynamic.sessions.push(`${header}:\n${memories}`);
        }
        log.debug(`Loaded ${sessions.length} recent sessions with all memories`);
      }
    } catch (err) {
      log.debug('No session memories or error fetching', err);
    }
  }

  log.end('fetchMemories', {
    globalCount: result.static.global.length,
    decisionCount: result.static.decisions.length,
    sessionCount: result.dynamic.sessions.length,
  });

  return result;
}

/**
 * Format a timestamp as relative time
 */
function formatTimeAgo(isoTimestamp: string): string {
  try {
    const dt = new Date(isoTimestamp);
    const now = new Date();
    const seconds = (now.getTime() - dt.getTime()) / 1000;
    const minutes = seconds / 60;
    const hours = seconds / 3600;
    const days = seconds / 86400;

    if (minutes < 60) return `${Math.floor(minutes)}m ago`;
    if (hours < 24) return `${Math.floor(hours)}h ago`;
    if (days < 7) return `${Math.floor(days)}d ago`;

    const month = dt.toLocaleString('en', { month: 'short' });
    return `${dt.getDate()} ${month}`;
  } catch {
    return '';
  }
}

/**
 * Build welcome context with proper memory injection
 */
function buildWelcomeContext(config: AideConfig, state: SessionState, memories: MemoryInjection, setupError?: string | null): string {
  const lines = [
    '<aide-context>',
    '',
  ];

  // Show setup error prominently at the top
  if (setupError) {
    lines.push('## ⚠️ Setup Issue');
    lines.push('');
    lines.push(setupError);
    lines.push('');
  }

  lines.push('## Session');
  lines.push('');
  lines.push(`ID: ${state.sessionId.slice(0, 8)}`);
  lines.push(`Project: ${getProjectName(state.cwd)}`);
  lines.push('');

  // STATIC: Global preferences (scope:global)
  if (memories.static.global.length > 0) {
    lines.push('## Preferences (Global)');
    lines.push('');
    lines.push('User preferences that apply across all projects:');
    lines.push('');
    for (const mem of memories.static.global) {
      lines.push(`- ${mem}`);
    }
    lines.push('');
  }

  // STATIC: Project decisions
  if (memories.static.decisions.length > 0) {
    lines.push('## Project Decisions');
    lines.push('');
    lines.push('Architectural decisions for this project. Follow these:');
    lines.push('');
    for (const decision of memories.static.decisions) {
      lines.push(`- ${decision}`);
    }
    lines.push('');
  }

  // DYNAMIC: Recent sessions
  if (memories.dynamic.sessions.length > 0) {
    lines.push('## Recent Sessions');
    lines.push('');
    for (const session of memories.dynamic.sessions) {
      // Sessions may contain multi-line content, indent properly
      const sessionLines = session.split('\n');
      lines.push(`### ${sessionLines[0]}`);
      if (sessionLines.length > 1) {
        lines.push('');
        lines.push(...sessionLines.slice(1));
      }
      lines.push('');
    }
  }

  lines.push('## Available Modes');
  lines.push('');
  lines.push('- **autopilot**: Full autonomous execution');
  lines.push('- **eco**: Token-efficient mode');
  lines.push('- **ralph**: Persistence until verified complete');
  lines.push('- **swarm**: Parallel agents with shared memory');
  lines.push('- **plan**: Planning interview workflow');
  lines.push('');
  lines.push('</aide-context>');

  return lines.join('\n');
}

// Debug helper - writes to debug.log (not stderr)
function debugLog(msg: string): void {
  debug(SOURCE, msg);
}

async function main(): Promise<void> {
  let log: Logger | null = null;
  const hookStart = Date.now();

  debugLog(`Hook started at ${new Date().toISOString()}`);

  try {
    debugLog('Reading stdin...');
    const input = await readStdin();
    debugLog(`Stdin read complete (${Date.now() - hookStart}ms)`);

    if (!input.trim()) {
      debugLog('Empty input, exiting');
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || 'unknown';

    // Switch debug logging to project-local logs
    setDebugCwd(cwd);

    debugLog(`Parsed input: cwd=${cwd}, sessionId=${sessionId.slice(0, 8)}`);

    // Initialize logger
    log = new Logger('session-start', cwd);
    log.info(`Session starting: ${sessionId.slice(0, 8)}`);
    log.start('total');

    debugLog(`Logger initialized, enabled=${log.isEnabled()}`);

    // Initialize directories
    debugLog('ensureDirectories starting...');
    ensureDirectories(cwd, log);
    debugLog(`ensureDirectories complete (${Date.now() - hookStart}ms)`);

    // Ensure aide binary is available (check local bin first, then download if needed)
    debugLog('ensureAideBinary starting...');
    const { binary: aideBinary, error: binaryError } = await ensureAideBinary(cwd, log);
    debugLog(`ensureAideBinary complete (${Date.now() - hookStart}ms)`);

    // Reset global state for new session (preserves per-agent state)
    debugLog('resetAideState starting...');
    resetAideState(cwd, log);
    debugLog(`resetAideState complete (${Date.now() - hookStart}ms)`);

    // Reset HUD state for clean session start
    debugLog('resetHudState starting...');
    resetHudState(cwd, log);
    debugLog(`resetHudState complete (${Date.now() - hookStart}ms)`);

    // Load config
    debugLog('loadConfig starting...');
    const config = loadConfig(cwd, log);
    debugLog(`loadConfig complete (${Date.now() - hookStart}ms)`);

    // Cleanup stale state files
    debugLog('cleanupStaleState starting...');
    cleanupStaleState(cwd, log);
    debugLog(`cleanupStaleState complete (${Date.now() - hookStart}ms)`);

    // Update session heartbeat (proves this session is alive)
    debugLog('updateSessionHeartbeat starting...');
    updateSessionHeartbeat(cwd, sessionId);
    debugLog(`updateSessionHeartbeat complete (${Date.now() - hookStart}ms)`);

    // Clean up orphaned agents from dead sessions
    debugLog('cleanupDeadSessionAgents starting...');
    cleanupDeadSessionAgents(cwd, sessionId, log);
    debugLog(`cleanupDeadSessionAgents complete (${Date.now() - hookStart}ms)`);

    // Clean up stale agents older than 3 hours (TTL-based)
    debugLog('cleanupStaleAgentsByTTL starting...');
    cleanupStaleAgentsByTTL(cwd, sessionId, log);
    debugLog(`cleanupStaleAgentsByTTL complete (${Date.now() - hookStart}ms)`);

    // Initialize session
    debugLog('initializeSession starting...');
    const state = initializeSession(sessionId, cwd, log);
    debugLog(`initializeSession complete (${Date.now() - hookStart}ms)`);

    // Fetch memories for context injection (static + dynamic)
    debugLog('fetchMemories starting...');
    const memories = fetchMemories(cwd, config, log);
    debugLog(`fetchMemories complete (${Date.now() - hookStart}ms)`);

    // Build welcome context with injected memories
    debugLog('buildWelcomeContext starting...');
    log.start('buildWelcomeContext');
    const context = buildWelcomeContext(config, state, memories, binaryError);
    log.end('buildWelcomeContext');
    debugLog(`buildWelcomeContext complete (${Date.now() - hookStart}ms)`);

    log.end('total');
    log.info('Session start complete');
    debugLog(`Flushing logs to ${log.getLogFile()}...`);
    log.flush();
    debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);

    const output: HookOutput = {
      continue: true,
      hookSpecificOutput: {
        hookEventName: 'SessionStart',
        additionalContext: context,
      },
    };

    console.log(JSON.stringify(output));
  } catch (error) {
    debugLog(`ERROR: ${error}`);
    // Log error if logger is available
    if (log) {
      log.error('Session start failed', error);
      log.flush();
    }
    // On error, allow continuation without context
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
