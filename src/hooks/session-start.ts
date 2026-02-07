#!/usr/bin/env node
/**
 * Session Start Hook (SessionStart)
 *
 * Initializes aide state and configuration on session start.
 * - Creates .aide directories if needed
 * - Loads config files
 * - Initializes HUD state
 * - Runs `aide session init` (single binary call for state reset, cleanup, memory fetch)
 *
 * Debug logging: Set AIDE_DEBUG=1 to enable startup tracing
 * Logs written to: .aide/_logs/startup.log
 */

import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  readdirSync,
  unlinkSync,
  statSync,
  copyFileSync,
  chmodSync,
} from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { homedir } from "os";
import { execFileSync, execSync } from "child_process";
import { Logger, debug, setDebugCwd } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";
import {
  findAideBinary,
  ensureAideBinary,
} from "../lib/aide-downloader.js";

const SOURCE = "session-start";
debug(SOURCE, `Hook started (AIDE_DEBUG=${process.env.AIDE_DEBUG || "unset"})`);

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

interface AideConfig {
  tiers: Record<string, string>;
  aliases: Record<string, string>;
  hud: {
    enabled: boolean;
    elements: string[];
  };
}

interface SessionState {
  sessionId: string;
  startedAt: string;
  cwd: string;
  activeMode: string | null;
  agentCount: number;
}

const DEFAULT_CONFIG: AideConfig = {
  tiers: {
    fast: "Cheapest/fastest model",
    balanced: "Good cost/capability balance",
    smart: "Most capable model",
  },
  aliases: {
    opus: "smart",
    sonnet: "balanced",
    haiku: "fast",
    cheap: "fast",
    quick: "fast",
    thorough: "smart",
    best: "smart",
  },
  hud: {
    enabled: true,
    elements: ["mode", "model", "agents"],
  },
};

interface BinaryCheckResult {
  binary: string | null;
  error: string | null;
  warning: string | null;
  downloaded: boolean;
}

/**
 * Check for aide binary with logging, version checking, and auto-download
 */
async function checkAideBinary(
  cwd: string,
  log: Logger,
): Promise<BinaryCheckResult> {
  log.start("checkAideBinary");

  const result = await ensureAideBinary(cwd);

  if (result.binary) {
    if (result.downloaded) {
      log.info(`aide binary downloaded successfully to ${result.binary}`);
    }
    if (result.warning) {
      log.info("aide update available");
    }
    log.end("checkAideBinary", {
      found: true,
      path: result.binary,
      downloaded: result.downloaded,
      hasWarning: !!result.warning,
    });
    return result;
  }

  log.warn("aide binary not found and download failed");
  log.end("checkAideBinary", { found: false });

  return result;
}

/**
 * Reset HUD state file for clean session start
 * Clears the HUD display so it starts fresh
 */
function resetHudState(cwd: string, log: Logger): void {
  log.start("resetHudState");

  const hudPath = join(cwd, ".aide", "state", "hud.txt");

  try {
    if (existsSync(hudPath)) {
      // Write empty/idle HUD state
      writeFileSync(hudPath, "mode:idle");
      log.debug("HUD state reset to idle");
    }
    log.end("resetHudState", { success: true });
  } catch (err) {
    // Non-fatal
    log.warn("Failed to reset HUD state", err);
    log.end("resetHudState", { success: false, error: String(err) });
  }
}

/**
 * Get the plugin root directory
 */
function getPluginRoot(): string | null {
  // Check CLAUDE_PLUGIN_ROOT env var first (set by Claude Code)
  const envRoot = process.env.CLAUDE_PLUGIN_ROOT;
  if (envRoot && existsSync(join(envRoot, "package.json"))) {
    return envRoot;
  }

  // Calculate from this script's location: dist/hooks/session-start.js -> ../../
  try {
    const scriptPath = fileURLToPath(import.meta.url);
    const pluginRoot = join(dirname(scriptPath), "..", "..");
    if (existsSync(join(pluginRoot, "package.json"))) {
      return pluginRoot;
    }
  } catch {
    // import.meta.url not available
  }

  return null;
}

/**
 * Install the HUD wrapper script to ~/.claude/bin/
 *
 * This installs a thin wrapper that delegates to the real HUD script in the plugin.
 * The wrapper allows plugin updates to take effect without reinstalling.
 */
function installHudWrapper(log: Logger): void {
  log.start("installHudWrapper");

  const claudeBinDir = join(homedir(), ".claude", "bin");
  const wrapperDest = join(claudeBinDir, "aide-hud.sh");

  // Check if wrapper already exists
  if (existsSync(wrapperDest)) {
    log.debug("HUD wrapper already installed");
    log.end("installHudWrapper", { skipped: true, reason: "exists" });
    return;
  }

  // Find our wrapper source
  const pluginRoot = getPluginRoot();
  if (!pluginRoot) {
    log.warn("Could not determine plugin root, skipping HUD wrapper install");
    log.end("installHudWrapper", { skipped: true, reason: "no-plugin-root" });
    return;
  }

  const wrapperSrc = join(pluginRoot, "scripts", "aide-hud-wrapper.sh");
  if (!existsSync(wrapperSrc)) {
    log.warn(`HUD wrapper source not found: ${wrapperSrc}`);
    log.end("installHudWrapper", { skipped: true, reason: "no-source" });
    return;
  }

  try {
    // Create ~/.claude/bin if needed
    if (!existsSync(claudeBinDir)) {
      mkdirSync(claudeBinDir, { recursive: true });
    }

    // Copy wrapper script
    copyFileSync(wrapperSrc, wrapperDest);
    chmodSync(wrapperDest, 0o755);

    log.info(`Installed HUD wrapper to ${wrapperDest}`);
    log.end("installHudWrapper", { success: true, path: wrapperDest });
  } catch (err) {
    log.warn("Failed to install HUD wrapper", err);
    log.end("installHudWrapper", { success: false, error: String(err) });
  }
}

/**
 * Ensure all .aide directories exist
 */
function ensureDirectories(cwd: string, log: Logger): void {
  const dirs = [
    join(cwd, ".aide"),
    join(cwd, ".aide", "skills"),
    join(cwd, ".aide", "config"),
    join(cwd, ".aide", "state"),
    join(cwd, ".aide", "memory"),
    join(cwd, ".aide", "worktrees"),
    join(cwd, ".aide", "_logs"), // Log directory
    join(homedir(), ".aide"),
    join(homedir(), ".aide", "skills"),
    join(homedir(), ".aide", "config"),
  ];

  log.start("ensureDirectories");
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
  const gitignorePath = join(cwd, ".aide", ".gitignore");
  const requiredGitignoreContent = `# AIDE runtime files - do not commit
_logs/
state/
bin/
*.db
*.bleve/
`;
  if (!existsSync(gitignorePath)) {
    try {
      writeFileSync(gitignorePath, requiredGitignoreContent);
      log.debug(`Created .gitignore: ${gitignorePath}`);
    } catch (err) {
      log.warn(`Failed to create .gitignore: ${gitignorePath}`, err);
    }
  } else {
    // Check if bin/ is missing and add it
    try {
      const existingContent = readFileSync(gitignorePath, "utf-8");
      if (!existingContent.includes("bin/")) {
        const updatedContent = existingContent.trimEnd() + "\nbin/\n";
        writeFileSync(gitignorePath, updatedContent);
        log.debug(`Updated .gitignore to include bin/`);
      }
    } catch (err) {
      log.warn(`Failed to update .gitignore: ${gitignorePath}`, err);
    }
  }

  log.end("ensureDirectories", { total: dirs.length, created, existed });
}

/**
 * Load or create config file
 */
function loadConfig(cwd: string, log: Logger): AideConfig {
  const configPath = join(cwd, ".aide", "config", "aide.json");

  log.start("loadConfig");

  if (existsSync(configPath)) {
    try {
      const content = readFileSync(configPath, "utf-8");
      log.end("loadConfig", { source: "existing", path: configPath });
      return { ...DEFAULT_CONFIG, ...JSON.parse(content) };
    } catch (err) {
      log.warn(`Failed to parse config: ${configPath}`, err);
      log.end("loadConfig", { source: "default", reason: "parse-error" });
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

  log.end("loadConfig", { source: "default", reason: "not-found" });
  return DEFAULT_CONFIG;
}

/**
 * Clean up stale state files older than 24 hours
 */
function cleanupStaleStateFiles(cwd: string, log: Logger): void {
  const stateDir = join(cwd, ".aide", "state");
  if (!existsSync(stateDir)) {
    log.debug("cleanupStaleStateFiles: state directory does not exist");
    return;
  }

  log.start("cleanupStaleStateFiles");

  const now = Date.now();
  const maxAge = 24 * 60 * 60 * 1000; // 24 hours
  let scanned = 0;
  let deleted = 0;

  try {
    const files = readdirSync(stateDir);
    for (const file of files) {
      // Don't clean up session or config files
      if (file.endsWith("-state.json") || file === "session.json") {
        scanned++;
        const filePath = join(stateDir, file);
        const stats = statSync(filePath);
        const age = now - stats.mtimeMs;
        if (age > maxAge) {
          try {
            unlinkSync(filePath);
            deleted++;
            log.debug(
              `Deleted stale file: ${file} (age: ${Math.round(age / 3600000)}h)`,
            );
          } catch (err) {
            log.warn(`Failed to delete stale file: ${file}`, err);
          }
        }
      }
    }
  } catch (err) {
    log.warn("Failed to read state directory", err);
  }

  log.end("cleanupStaleStateFiles", { scanned, deleted });
}

/**
 * Initialize session state
 */
function initializeSession(
  sessionId: string,
  cwd: string,
  log: Logger,
): SessionState {
  log.start("initializeSession");

  const state: SessionState = {
    sessionId,
    startedAt: new Date().toISOString(),
    cwd,
    activeMode: null,
    agentCount: 0,
  };

  const statePath = join(cwd, ".aide", "state", "session.json");
  try {
    writeFileSync(statePath, JSON.stringify(state, null, 2));
    log.debug(`Session state written: ${statePath}`);
  } catch (err) {
    log.warn(`Failed to write session state: ${statePath}`, err);
  }

  log.end("initializeSession", { sessionId: sessionId.slice(0, 8) });
  return state;
}

/**
 * Get project name from git remote or directory name
 */
function getProjectName(cwd: string): string {
  try {
    // Try git remote first
    const remoteUrl = execSync("git config --get remote.origin.url", {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 2000,
    })
      .toString()
      .trim();

    // Extract repo name from URL
    const match = remoteUrl.match(/[/:]([^/]+?)(?:\.git)?$/);
    if (match) return match[1];
  } catch {
    // Not a git repo or no remote
  }

  // Fallback to directory name
  return cwd.split("/").pop() || "unknown";
}

/**
 * Result from `aide session init --format=json`
 */
interface SessionInitResult {
  state_keys_deleted: number;
  stale_agents_cleaned: number;
  global_memories: Array<{ id: string; content: string; category: string; tags: string[] }>;
  project_memories: Array<{ id: string; content: string; category: string; tags: string[] }>;
  decisions: Array<{ topic: string; value: string; rationale?: string }>;
  recent_sessions: Array<{
    session_id: string;
    last_at: string;
    memories: Array<{ content: string; category: string }>;
  }>;
}

interface MemoryInjection {
  static: {
    global: string[]; // scope:global memories
    project: string[]; // project:<name> memories
    decisions: string[]; // project decisions
  };
  dynamic: {
    sessions: string[]; // recent session summaries
  };
}

/**
 * Run `aide session init` — single binary invocation that:
 * 1. Deletes global state keys (mode, startedAt, etc.)
 * 2. Cleans up stale agent state (>30m)
 * 3. Returns global memories, project memories, decisions, recent sessions
 *
 * Replaces 7 separate binary spawns (~35-50s) with 1 (~5s).
 */
function runSessionInit(
  cwd: string,
  projectName: string,
  sessionLimit: number,
  log: Logger,
): MemoryInjection {
  log.start("sessionInit");

  const result: MemoryInjection = {
    static: { global: [], project: [], decisions: [] },
    dynamic: { sessions: [] },
  };

  // Check for disable flag
  if (process.env.AIDE_MEMORY_INJECT === "0") {
    log.info("Memory injection disabled via AIDE_MEMORY_INJECT=0");
  }

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug("aide binary not found, skipping session init");
    log.end("sessionInit", { skipped: true, reason: "no-binary" });
    return result;
  }

  const dbPath = join(cwd, ".aide", "memory", "store.db");

  try {
    const args = [
      "session", "init",
      `--project=${projectName}`,
      `--session-limit=${sessionLimit}`,
    ];

    const output = execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      timeout: 15000,
      env: { ...process.env, AIDE_MEMORY_DB: dbPath },
    }).trim();

    if (!output) {
      log.debug("session init returned empty output");
      log.end("sessionInit", { success: false, reason: "empty-output" });
      return result;
    }

    const data: SessionInitResult = JSON.parse(output);

    log.debug(
      `State: ${data.state_keys_deleted} keys deleted, ${data.stale_agents_cleaned} stale agents cleaned`,
    );

    // Skip memory population if injection disabled
    if (process.env.AIDE_MEMORY_INJECT === "0") {
      log.end("sessionInit", { success: true, memoriesDisabled: true });
      return result;
    }

    // Populate memories from the single response
    result.static.global = data.global_memories.map((m) => m.content);
    result.static.project = data.project_memories.map((m) => m.content);
    result.static.decisions = data.decisions.map(
      (d) =>
        `**${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ""}`,
    );

    // Format recent sessions
    for (const sess of data.recent_sessions) {
      const timeAgo = sess.last_at ? formatTimeAgo(sess.last_at) : "";
      const header = `Session ${sess.session_id}${timeAgo ? ` (${timeAgo})` : ""}`;
      const memories = sess.memories
        .map((m) => `- [${m.category}] ${m.content}`)
        .join("\n");
      result.dynamic.sessions.push(`${header}:\n${memories}`);
    }

    log.end("sessionInit", {
      success: true,
      globalCount: result.static.global.length,
      projectCount: result.static.project.length,
      decisionCount: result.static.decisions.length,
      sessionCount: result.dynamic.sessions.length,
    });
  } catch (err) {
    log.warn("session init failed", err);
    log.end("sessionInit", { success: false, error: String(err) });
  }

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

    const month = dt.toLocaleString("en", { month: "short" });
    return `${dt.getDate()} ${month}`;
  } catch {
    return "";
  }
}

interface StartupNotices {
  error?: string | null; // Critical error (blocks aide)
  warning?: string | null; // Soft warning (update available, etc.)
  info?: string[]; // Informational notices (binary downloaded, tasks cleared, etc.)
}

/**
 * Build welcome context with proper memory injection
 */
function buildWelcomeContext(
  state: SessionState,
  memories: MemoryInjection,
  notices: StartupNotices = {},
): string {
  const lines = ["<aide-context>", ""];

  // Show setup error prominently at the top
  if (notices.error) {
    lines.push("## Setup Issue");
    lines.push("");
    lines.push(notices.error);
    lines.push("");
  }

  // Show update warning (less prominent than errors)
  if (notices.warning) {
    lines.push("## Update Available");
    lines.push("");
    lines.push(notices.warning);
    lines.push("");
  }

  // Show informational notices
  if (notices.info && notices.info.length > 0) {
    lines.push("## Startup");
    lines.push("");
    for (const info of notices.info) {
      lines.push(`- ${info}`);
    }
    lines.push("");
  }

  lines.push("## Session");
  lines.push("");
  lines.push(`ID: ${state.sessionId.slice(0, 8)}`);
  lines.push(`Project: ${getProjectName(state.cwd)}`);
  lines.push("");

  // STATIC: Global preferences (scope:global)
  if (memories.static.global.length > 0) {
    lines.push("## Preferences (Global)");
    lines.push("");
    lines.push("User preferences that apply across all projects:");
    lines.push("");
    for (const mem of memories.static.global) {
      lines.push(`- ${mem}`);
    }
    lines.push("");
  }

  // STATIC: Project memories (project:<name>)
  if (memories.static.project.length > 0) {
    lines.push("## Project Context");
    lines.push("");
    lines.push("Memories specific to this project:");
    lines.push("");
    for (const mem of memories.static.project) {
      lines.push(`- ${mem}`);
    }
    lines.push("");
  }

  // STATIC: Project decisions
  if (memories.static.decisions.length > 0) {
    lines.push("## Project Decisions");
    lines.push("");
    lines.push("Architectural decisions for this project. Follow these:");
    lines.push("");
    for (const decision of memories.static.decisions) {
      lines.push(`- ${decision}`);
    }
    lines.push("");
  }

  // DYNAMIC: Recent sessions
  if (memories.dynamic.sessions.length > 0) {
    lines.push("## Recent Sessions");
    lines.push("");
    for (const session of memories.dynamic.sessions) {
      // Sessions may contain multi-line content, indent properly
      const sessionLines = session.split("\n");
      lines.push(`### ${sessionLines[0]}`);
      if (sessionLines.length > 1) {
        lines.push("");
        lines.push(...sessionLines.slice(1));
      }
      lines.push("");
    }
  }

  lines.push("## Available Modes");
  lines.push("");
  lines.push("- **autopilot**: Full autonomous execution");
  lines.push("- **eco**: Token-efficient mode");
  lines.push("- **ralph**: Persistence until verified complete");
  lines.push("- **swarm**: Parallel agents with shared memory");
  lines.push("- **plan**: Planning interview workflow");
  lines.push("");
  lines.push("</aide-context>");

  return lines.join("\n");
}

// Debug helper - writes to debug.log (not stderr)
function debugLog(msg: string): void {
  debug(SOURCE, msg);
}

// Ensure we always output valid JSON, even on catastrophic errors
function outputContinue(): void {
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    // Last resort - raw JSON string
    console.log('{"continue":true}');
  }
}

// Global error handlers to prevent hook crashes without JSON output
process.on("uncaughtException", (err) => {
  debugLog(`UNCAUGHT EXCEPTION: ${err}`);
  outputContinue();
  process.exit(0);
});

process.on("unhandledRejection", (reason) => {
  debugLog(`UNHANDLED REJECTION: ${reason}`);
  outputContinue();
  process.exit(0);
});

async function main(): Promise<void> {
  let log: Logger | null = null;
  const hookStart = Date.now();

  debugLog(`Hook started at ${new Date().toISOString()}`);

  try {
    debugLog("Reading stdin...");
    const input = await readStdin();
    debugLog(`Stdin read complete (${Date.now() - hookStart}ms)`);

    if (!input.trim()) {
      debugLog("Empty input, exiting");
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Switch debug logging to project-local logs
    setDebugCwd(cwd);

    debugLog(`Parsed input: cwd=${cwd}, sessionId=${sessionId.slice(0, 8)}`);

    // Initialize logger
    log = new Logger("session-start", cwd);
    log.info(`Session starting: ${sessionId.slice(0, 8)}`);
    log.start("total");

    debugLog(`Logger initialized, enabled=${log.isEnabled()}`);

    // Initialize directories (FS only, fast)
    debugLog("ensureDirectories starting...");
    ensureDirectories(cwd, log);
    debugLog(`ensureDirectories complete (${Date.now() - hookStart}ms)`);

    // Install HUD wrapper script if not present (FS only, fast)
    debugLog("installHudWrapper starting...");
    installHudWrapper(log);
    debugLog(`installHudWrapper complete (${Date.now() - hookStart}ms)`);

    // Check that aide binary is available (auto-downloads if missing/outdated)
    debugLog("checkAideBinary starting...");
    const {
      error: binaryError,
      warning: binaryWarning,
      downloaded: binaryDownloaded,
    } = await checkAideBinary(cwd, log);
    if (binaryDownloaded) {
      debugLog(`aide binary was downloaded`);
    }
    debugLog(`checkAideBinary complete (${Date.now() - hookStart}ms)`);

    // Reset HUD state for clean session start (FS only, fast)
    debugLog("resetHudState starting...");
    resetHudState(cwd, log);
    debugLog(`resetHudState complete (${Date.now() - hookStart}ms)`);

    // Load config (FS only, fast)
    debugLog("loadConfig starting...");
    loadConfig(cwd, log);
    debugLog(`loadConfig complete (${Date.now() - hookStart}ms)`);

    // Cleanup stale state files on disk (FS only, fast)
    debugLog("cleanupStaleStateFiles starting...");
    cleanupStaleStateFiles(cwd, log);
    debugLog(`cleanupStaleStateFiles complete (${Date.now() - hookStart}ms)`);

    // Initialize session state file (FS only, fast)
    debugLog("initializeSession starting...");
    const state = initializeSession(sessionId, cwd, log);
    debugLog(`initializeSession complete (${Date.now() - hookStart}ms)`);

    // Single aide binary call: reset state + cleanup agents + fetch all memories
    // Replaces 7 separate binary spawns (~35-50s) with 1 (~5s)
    const projectName = getProjectName(cwd);
    debugLog("sessionInit starting...");
    const memories = runSessionInit(cwd, projectName, 3, log);
    debugLog(`sessionInit complete (${Date.now() - hookStart}ms)`);

    // Build startup notices
    const notices: StartupNotices = {
      error: binaryError,
      warning: binaryWarning,
      info: [],
    };
    if (binaryDownloaded) {
      notices.info!.push("aide binary downloaded");
    }

    // Output notices to stderr so user sees them in the console
    if (notices.error) {
      console.error(`[aide] ${notices.error}`);
    }
    if (notices.warning) {
      console.error(`[aide] ${notices.warning}`);
    }
    for (const info of notices.info || []) {
      console.error(`[aide] ${info}`);
    }

    // Build welcome context with injected memories
    debugLog("buildWelcomeContext starting...");
    log.start("buildWelcomeContext");
    const context = buildWelcomeContext(state, memories, notices);
    log.end("buildWelcomeContext");
    debugLog(`buildWelcomeContext complete (${Date.now() - hookStart}ms)`);

    log.end("total");
    log.info("Session start complete");
    debugLog(`Flushing logs to ${log.getLogFile()}...`);
    log.flush();
    debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);

    const output: HookOutput = {
      continue: true,
      hookSpecificOutput: {
        hookEventName: "SessionStart",
        additionalContext: context,
      },
    };

    console.log(JSON.stringify(output));
  } catch (error) {
    debugLog(`ERROR: ${error}`);
    // Log error if logger is available
    if (log) {
      log.error("Session start failed", error);
      log.flush();
    }
    // On error, allow continuation without context
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
