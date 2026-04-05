#!/usr/bin/env bun
/**
 * aide-wrapper.ts - Ensures aide binary exists before executing
 *
 * Called by an assistant's MCP server configuration.
 * Finds the aide binary, downloads it if missing, then delegates to it.
 *
 * Plugin root resolution order:
 *   1. AIDE_PLUGIN_ROOT  (canonical, platform-agnostic)
 *   2. CLAUDE_PLUGIN_ROOT (set by Claude Code)
 *   3. SCRIPT_DIR/..      (fallback: infer from wrapper location)
 *
 * Lives at: <plugin-root>/bin/aide-wrapper.ts
 * Binary at: <plugin-root>/bin/aide[.exe]
 *
 * Logs written to: .aide/_logs/wrapper.log
 */

import { execFileSync, spawnSync } from "child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  appendFileSync,
  unlinkSync,
  writeFileSync,
  rmdirSync,
} from "fs";
import { dirname, join, resolve } from "path";
import { fileURLToPath } from "url";

const isWindows = process.platform === "win32";
const EXT = isWindows ? ".exe" : "";

// Resolve symlinks so that invoking via node_modules/.bin/aide-wrapper
// (which is a symlink to the real package) gives us the real package dir.
const __filename = fileURLToPath(import.meta.url);
let scriptDir: string;
try {
  scriptDir = dirname(realpathSync(__filename));
} catch {
  scriptDir = dirname(__filename);
}

const PLUGIN_ROOT =
  process.env.AIDE_PLUGIN_ROOT ||
  process.env.CLAUDE_PLUGIN_ROOT ||
  resolve(scriptDir, "..");

const BINARY = join(PLUGIN_ROOT, "bin", `aide${EXT}`);
const BIN_DIR = join(PLUGIN_ROOT, "bin");

// Setup logging
const LOG_DIR = join(PLUGIN_ROOT, ".aide", "_logs");
const LOG_FILE = join(LOG_DIR, "wrapper.log");
try {
  mkdirSync(LOG_DIR, { recursive: true });
} catch {
  // ignore
}

function log(msg: string): void {
  const timestamp = new Date().toISOString().replace("T", " ").replace(/\..+/, "");
  const line = `[${timestamp}] [aide-wrapper] ${msg}`;
  process.stderr.write(line + "\n");
  try {
    appendFileSync(LOG_FILE, line + "\n");
  } catch {
    // ignore log write failures
  }
}

/**
 * Compare semantic versions: returns true if a >= b
 */
function versionGte(a: string, b: string): boolean {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    const va = pa[i] || 0;
    const vb = pb[i] || 0;
    if (va > vb) return true;
    if (va < vb) return false;
  }
  return true;
}

/**
 * Get the version string from the aide binary
 */
function getBinaryVersion(binary: string): string | null {
  try {
    const output = execFileSync(binary, ["version"], {
      stdio: "pipe",
      timeout: 5000,
    })
      .toString()
      .trim();
    const match = output.match(
      /(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.+]+)?)/,
    );
    return match ? match[1] : null;
  } catch {
    return null;
  }
}

/**
 * Read the plugin version from plugin.json or package.json
 */
function getPluginVersion(): string | null {
  for (const relPath of [
    ".claude-plugin/plugin.json",
    "package.json",
  ]) {
    try {
      const content = readFileSync(join(PLUGIN_ROOT, relPath), "utf-8");
      const match = content.match(/"version"\s*:\s*"(\d+\.\d+\.\d+)/);
      if (match) return match[1];
    } catch {
      // skip
    }
  }
  return null;
}

/**
 * Check if the binary exists and is executable
 */
function binaryExists(): boolean {
  if (!existsSync(BINARY)) return false;
  // On Windows, existence is sufficient (no execute bit)
  if (isWindows) return true;
  try {
    execFileSync(BINARY, ["version"], { stdio: "pipe", timeout: 5000 });
    return true;
  } catch {
    // Binary exists but can't execute — might need re-download
    return false;
  }
}

/**
 * Simple cross-platform file lock.
 * Uses mkdir as an atomic operation (works on all platforms).
 * Returns a cleanup function to release the lock.
 */
function acquireLock(lockPath: string, timeoutMs: number = 60000): () => void {
  const start = Date.now();
  const pollMs = 200;

  while (true) {
    try {
      mkdirSync(lockPath);
      // Write our PID for debugging
      try {
        writeFileSync(join(lockPath, "pid"), String(process.pid));
      } catch {
        // ignore
      }
      return () => {
        try {
          unlinkSync(join(lockPath, "pid"));
        } catch { /* ignore */ }
        try {
          // rmdir only works on empty dirs — that's what we want
          rmdirSync(lockPath);
        } catch { /* ignore */ }
      };
    } catch (err: unknown) {
      if (
        err &&
        typeof err === "object" &&
        "code" in err &&
        (err as { code: string }).code === "EEXIST"
      ) {
        // Lock held by another process
        if (Date.now() - start > timeoutMs) {
          log("ERROR: Timed out waiting for download lock");
          // Force-remove stale lock and try once more
          try {
            unlinkSync(join(lockPath, "pid"));
          } catch { /* ignore */ }
          try {
            rmdirSync(lockPath);
          } catch { /* ignore */ }
          throw new Error("Timed out waiting for download lock");
        }
        // Poll — use Bun.sleepSync for cross-platform millisecond sleep
        Bun.sleepSync(pollMs);
        continue;
      }
      throw err;
    }
  }
}

/**
 * Download the aide binary using the TypeScript downloader
 */
function downloadBinary(): void {
  const LOCKDIR = join(BIN_DIR, ".aide-download.lock");

  // Resolve the downloader script — prefer src/ (dev) but fall back to dist/ (npm install)
  let downloader: string;
  if (existsSync(join(PLUGIN_ROOT, "src", "lib", "aide-downloader.ts"))) {
    downloader = join(PLUGIN_ROOT, "src", "lib", "aide-downloader.ts");
  } else if (existsSync(join(PLUGIN_ROOT, "dist", "lib", "aide-downloader.js"))) {
    downloader = join(PLUGIN_ROOT, "dist", "lib", "aide-downloader.js");
  } else {
    log(`ERROR: Cannot find aide-downloader in src/ or dist/ under ${PLUGIN_ROOT}`);
    process.exit(1);
  }

  // Remove stale/outdated binary before locking
  if (existsSync(BINARY)) {
    log("Removing outdated binary before download");
    try {
      unlinkSync(BINARY);
    } catch {
      // ignore
    }
  }

  const releaseLock = acquireLock(LOCKDIR);
  try {
    // Re-check after acquiring lock — another process may have finished the download
    if (existsSync(BINARY)) {
      log("Binary appeared while waiting for lock (downloaded by another process)");
      return;
    }

    log("Downloading binary...");
    log(`Using downloader: ${downloader}`);

    // Determine runner based on file extension
    let runner: string;
    let runnerArgs: string[];

    if (downloader.endsWith(".ts")) {
      runner = "bun";
      runnerArgs = [downloader, "--dest", BIN_DIR];
    } else {
      runner = "node";
      runnerArgs = [downloader, "--dest", BIN_DIR];
    }

    // Use cross-spawn for cross-platform compatibility (lazy import to
    // survive missing node_modules during bootstrap).
    const crossSpawn = require("cross-spawn");
    const result = crossSpawn.sync(runner, runnerArgs, {
      stdio: ["ignore", "inherit", "inherit"],
      timeout: 120000,
    });

    if (result.status !== 0) {
      log("ERROR: Downloader failed");
      process.exit(1);
    }

    if (!existsSync(BINARY)) {
      log("ERROR: Binary not found after download");
      process.exit(1);
    }
  } finally {
    releaseLock();
  }

  log(`Binary ready at ${BINARY}`);
}

// --- Ensure dependencies ---

// After a Claude Code marketplace autoUpdate (git pull), node_modules/
// may be missing since it's gitignored. Detect and self-heal before any
// imports that depend on npm packages (e.g. 'which', 'cross-spawn').
function ensureDependencies(): void {
  const nodeModules = join(PLUGIN_ROOT, "node_modules");
  if (!existsSync(nodeModules)) {
    log("node_modules missing — running bun install to restore dependencies");
    try {
      const result = spawnSync("bun", ["install", "--frozen-lockfile"], {
        cwd: PLUGIN_ROOT,
        stdio: ["ignore", "pipe", "pipe"],
        timeout: 60000,
      });
      if (result.status === 0) {
        log("bun install completed successfully");
      } else {
        const stderr = result.stderr?.toString().trim();
        log(`WARNING: bun install failed (status ${result.status}): ${stderr}`);
      }
    } catch (err) {
      log(`WARNING: bun install error: ${err}`);
    }
  }
}

ensureDependencies();

// --- Main ---

log(
  `Starting wrapper (pid=${process.pid}, args=${process.argv.slice(2).join(" ")})`,
);
log(`PLUGIN_ROOT=${PLUGIN_ROOT}`);
log(`BINARY=${BINARY}`);

let needsDownload = false;

if (!binaryExists()) {
  needsDownload = true;
  log("Binary not found or not executable");
} else {
  const binaryVersion = getBinaryVersion(BINARY);
  log(`Binary version: ${binaryVersion ?? "unknown"}`);

  if (binaryVersion && binaryVersion.includes("-dev.")) {
    // Dev build — check base version against plugin version
    const baseVersion = binaryVersion.split("-")[0];
    const pluginVersion = getPluginVersion();

    if (pluginVersion && versionGte(baseVersion, pluginVersion)) {
      log(
        `Dev build v${binaryVersion} (base ${baseVersion} >= plugin v${pluginVersion}), using local build`,
      );
    } else {
      needsDownload = true;
      log(
        `Dev build v${binaryVersion} is older than plugin v${pluginVersion ?? "unknown"}, re-downloading`,
      );
    }
  } else {
    const pluginVersion = getPluginVersion();
    if (pluginVersion && binaryVersion && !versionGte(binaryVersion, pluginVersion)) {
      needsDownload = true;
      log(
        `Release binary v${binaryVersion} is older than plugin v${pluginVersion}, re-downloading`,
      );
    } else {
      log(`Release binary v${binaryVersion ?? "unknown"} (plugin v${pluginVersion ?? "unknown"})`);
    }
  }
}

if (needsDownload) {
  downloadBinary();
}

// Execute the aide binary, replacing this process
log(`Executing: ${BINARY} ${process.argv.slice(2).join(" ")}`);

const result = spawnSync(BINARY, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});

// Forward the exit code
process.exit(result.status ?? 1);
