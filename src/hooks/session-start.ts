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
  copyFileSync,
  chmodSync,
} from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { homedir } from "os";
import { Logger, debug, setDebugCwd } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary, ensureAideBinary } from "../lib/aide-downloader.js";
import {
  ensureDirectories as coreEnsureDirectories,
  loadConfig as coreLoadConfig,
  initializeSession as coreInitializeSession,
  cleanupStaleStateFiles as coreCleanupStaleStateFiles,
  resetHudState as coreResetHudState,
  getProjectName,
  runSessionInit as coreRunSessionInit,
  buildWelcomeContext as coreBuildWelcomeContext,
  formatTimeAgo,
} from "../core/session-init.js";
import { syncMcpServers } from "../core/mcp-sync.js";
import type {
  AideConfig,
  SessionState,
  SessionInitResult,
  MemoryInjection,
  StartupNotices,
} from "../core/types.js";

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
 * Reset HUD state file for clean session start — delegates to core
 */
function resetHudState(cwd: string, log: Logger): void {
  log.start("resetHudState");
  try {
    coreResetHudState(cwd);
    log.end("resetHudState", { success: true });
  } catch (err) {
    log.warn("Failed to reset HUD state", err);
    log.end("resetHudState", { success: false, error: String(err) });
  }
}

/**
 * Get the plugin root directory
 */
function getPluginRoot(): string | null {
  // Check AIDE_PLUGIN_ROOT or CLAUDE_PLUGIN_ROOT env var
  const envRoot =
    process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
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
 * Ensure all .aide directories exist — delegates to core
 */
function ensureDirectories(cwd: string, log: Logger): void {
  log.start("ensureDirectories");
  const { created, existed } = coreEnsureDirectories(cwd);
  log.end("ensureDirectories", { created, existed });
}

/**
 * Load or create config file — delegates to core
 */
function loadConfig(cwd: string, log: Logger): AideConfig {
  log.start("loadConfig");
  const config = coreLoadConfig(cwd);
  log.end("loadConfig");
  return config;
}

/**
 * Clean up stale state files — delegates to core
 */
function cleanupStaleStateFiles(cwd: string, log: Logger): void {
  log.start("cleanupStaleStateFiles");
  const { scanned, deleted } = coreCleanupStaleStateFiles(cwd);
  log.end("cleanupStaleStateFiles", { scanned, deleted });
}

/**
 * Initialize session state — delegates to core
 */
function initializeSession(
  sessionId: string,
  cwd: string,
  log: Logger,
): SessionState {
  log.start("initializeSession");
  const state = coreInitializeSession(sessionId, cwd);
  log.end("initializeSession", { sessionId: sessionId.slice(0, 8) });
  return state;
}

// getProjectName, runSessionInit, formatTimeAgo, buildWelcomeContext
// are now imported from ../core/session-init.js above.
// The runSessionInit wrapper below adds logging around the core function.

/**
 * Run session init with logging — wraps core function
 */
function runSessionInit(
  cwd: string,
  projectName: string,
  sessionLimit: number,
  log: Logger,
  config?: AideConfig,
): MemoryInjection {
  log.start("sessionInit");

  const binary = findAideBinary(cwd);
  if (!binary) {
    log.debug("aide binary not found, skipping session init");
    log.end("sessionInit", { skipped: true, reason: "no-binary" });
    return {
      static: { global: [], project: [], decisions: [] },
      dynamic: { sessions: [] },
    };
  }

  const result = coreRunSessionInit(
    binary,
    cwd,
    projectName,
    sessionLimit,
    config,
  );

  log.end("sessionInit", {
    globalCount: result.static.global.length,
    projectCount: result.static.project.length,
    decisionCount: result.static.decisions.length,
    sessionCount: result.dynamic.sessions.length,
  });

  return result;
}

/**
 * Build welcome context — wraps core function
 */
function buildWelcomeContext(
  state: SessionState,
  memories: MemoryInjection,
  notices: StartupNotices = {},
): string {
  return coreBuildWelcomeContext(state, memories, notices);
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

    // Sync MCP server configs across assistants (FS only, fast)
    debugLog("mcpSync starting...");
    log.start("mcpSync");
    try {
      const mcpResult = syncMcpServers("claude-code", cwd);
      const totalImported =
        mcpResult.user.imported + mcpResult.project.imported;
      const totalWritten =
        mcpResult.user.serversWritten + mcpResult.project.serversWritten;
      const totalSkipped = mcpResult.user.skipped + mcpResult.project.skipped;
      log.end("mcpSync", {
        userServers: mcpResult.user.serversWritten,
        projectServers: mcpResult.project.serversWritten,
        imported: totalImported,
        skipped: totalSkipped,
      });
      if (totalImported > 0) {
        debugLog(
          `mcp-sync: imported ${totalImported} server(s), ${totalWritten} total`,
        );
      }
    } catch (err) {
      log.warn("MCP sync failed (non-fatal)", err);
      log.end("mcpSync", { success: false, error: String(err) });
    }
    debugLog(`mcpSync complete (${Date.now() - hookStart}ms)`);

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
    const config = loadConfig(cwd, log);
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
    const memories = runSessionInit(cwd, projectName, 3, log, config);
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

    // Log notices via debug (avoids stderr which Claude Code interprets as error)
    if (notices.error) {
      debugLog(`NOTICE ERROR: ${notices.error}`);
    }
    if (notices.warning) {
      debugLog(`NOTICE WARNING: ${notices.warning}`);
    }
    for (const info of notices.info || []) {
      debugLog(`NOTICE INFO: ${info}`);
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
