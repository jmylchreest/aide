#!/usr/bin/env bun
/**
 * aide-wrapper.ts - Ensures aide binary exists before executing
 *
 * Called by an assistant's MCP server configuration.
 * Finds the aide binary, downloads it if missing, then delegates to it.
 *
 * Plugin root is the package containing this wrapper. Inherited root
 * variables may refer to an older install and must not redirect provisioning.
 *
 * Binary resolution order (first hit wins):
 *   1. Bundled via npm optionalDependencies:
 *      @jmylchreest/aide-binary-<platform>-<arch>/bin/aide[.exe], located
 *      with node's resolution algorithm and a node_modules walk-up. Package
 *      managers HOIST optional deps next to the plugin, not inside it, so
 *      <plugin-root>/node_modules/ is the one place it is never found.
 *   2. PLUGIN_ROOT/bin/aide[.exe]  (legacy: downloaded by previous wrapper run)
 *   3. Downloaded fresh into PLUGIN_ROOT/bin/  (fallback when neither exists)
 *
 * The bundled path is populated by `npm install` for OpenCode/Codex users
 * (zero-config, offline-after-install). Claude Code users — whose plugin
 * distribution does not support npm optionalDependencies — fall through
 * to the legacy download path; Claude's per-version cache layout makes
 * that path work correctly.
 *
 * This file is deliberately self-contained: it runs before `bun install`
 * has necessarily populated node_modules, so it duplicates the resolution
 * logic in src/lib/binary-resolve.ts rather than importing it. The two are
 * pinned together by src/test/binary-resolve.test.ts.
 *
 * Lives at: <plugin-root>/bin/aide-wrapper.ts
 * Logs written to: .aide/_logs/wrapper.log
 */

import { execFileSync, spawnSync } from "child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  appendFileSync,
} from "fs";
import { createRequire } from "module";
import { dirname, join, resolve, sep } from "path";
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

// Anchor provisioning to the package that supplied this wrapper. An inherited
// root variable may still name the package from a previous session.
const SELF_ROOT = resolve(scriptDir, "..");

const PLUGIN_ROOT = SELF_ROOT;

const BINARY = join(PLUGIN_ROOT, "bin", `aide${EXT}`);
const BIN_DIR = join(PLUGIN_ROOT, "bin");

// Per-arch npm package holding the bundled binary for this host.
const ARCH_PKG: Record<string, string> = {
  "linux-x64": "linux-amd64",
  "linux-arm64": "linux-arm64",
  "darwin-x64": "darwin-amd64",
  "darwin-arm64": "darwin-arm64",
  "win32-x64": "windows-amd64",
  "win32-arm64": "windows-arm64",
};
const BINARY_PKG = (() => {
  const suffix = ARCH_PKG[`${process.platform}-${process.arch}`];
  return suffix ? `@jmylchreest/aide-binary-${suffix}` : null;
})();

/**
 * Find the binary npm installed via optionalDependencies.
 *
 * Package managers hoist optional deps to the install root, so the binary
 * is a *sibling* of the plugin package:
 *
 *   node_modules/@jmylchreest/aide-plugin/               <- SELF_ROOT
 *   node_modules/@jmylchreest/aide-binary-linux-amd64/bin/aide
 *
 * Node's resolver handles that natively; the walk-up covers layouts where
 * it can't run (bun's ESM cache, Yarn PnP-free installs, dev checkouts).
 */
function resolveBundledBinary(): string | null {
  if (!BINARY_PKG) return null;
  const subPath = join(...BINARY_PKG.split("/"), "bin", `aide${EXT}`);

  try {
    const resolved = createRequire(import.meta.url).resolve(
      `${BINARY_PKG}/bin/aide${EXT}`,
    );
    if (existsSync(resolved)) return resolved;
  } catch {
    // Not resolvable from here — fall through to the filesystem walk.
  }

  let dir = SELF_ROOT;
  for (;;) {
    const nested = join(dir, "node_modules", subPath);
    if (existsSync(nested)) return nested;
    if (dir.endsWith(`${sep}node_modules`)) {
      const hoisted = join(dir, subPath);
      if (existsSync(hoisted)) return hoisted;
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

// Setup logging
const LOG_DIR = join(PLUGIN_ROOT, ".aide", "_logs");
const LOG_FILE = join(LOG_DIR, "wrapper.log");
try {
  mkdirSync(LOG_DIR, { recursive: true });
} catch {
  // ignore
}

function log(msg: string): void {
  const timestamp = new Date()
    .toISOString()
    .replace("T", " ")
    .replace(/\..+/, "");
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
  const pa = a.split(/[+-]/)[0].split(".").map(Number);
  const pb = b.split(/[+-]/)[0].split(".").map(Number);
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
    const match = output.match(/(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.+]+)?)/);
    return match ? match[1] : null;
  } catch {
    return null;
  }
}

/** Read the version of the package supplying this wrapper. */
function getPluginVersion(): string | null {
  for (const relPath of [".claude-plugin/plugin.json", "package.json"]) {
    try {
      const content = readFileSync(join(SELF_ROOT, relPath), "utf-8");
      const match = content.match(/"version"\s*:\s*"(\d+\.\d+\.\d+)/);
      if (match && match[1] !== "0.0.0") return match[1];
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

/** Download through the shared installer, which also serializes hook downloads. */
function downloadBinary(): void {
  const source = join(PLUGIN_ROOT, "src", "lib", "aide-downloader.ts");
  const compiled = join(PLUGIN_ROOT, "dist", "lib", "aide-downloader.js");
  const downloader = existsSync(source) ? source : compiled;
  if (!existsSync(downloader)) {
    throw new Error(
      `Cannot find aide-downloader in src/ or dist/ under ${PLUGIN_ROOT}`,
    );
  }

  const pluginVersion = getPluginVersion();
  const args = [downloader, "--dest", BIN_DIR];
  if (pluginVersion) args.push("--version", pluginVersion);
  log(
    `Downloading binary for plugin v${pluginVersion ?? "unknown"} using ${downloader}`,
  );

  // stdout is the MCP protocol channel. All installer output goes to stderr.
  const crossSpawn = require("cross-spawn");
  const result = crossSpawn.sync(
    downloader.endsWith(".ts") ? "bun" : "node",
    args,
    {
      stdio: ["ignore", 2, "inherit"],
      env: { ...process.env, AIDE_PLUGIN_ROOT: PLUGIN_ROOT },
      // Allow the lock wait plus both download attempts and validation.
      timeout: 300000,
    },
  );
  if (result.status !== 0) {
    throw new Error(
      `Downloader failed (status ${result.status}): ${result.error ?? "see stderr above"}`,
    );
  }

  const installed = getBinaryVersion(BINARY);
  if (
    !installed ||
    (pluginVersion && !versionGte(installed.split("-")[0], pluginVersion))
  ) {
    throw new Error(
      `Downloaded binary v${installed ?? "unknown"} does not satisfy plugin v${pluginVersion ?? "unknown"}`,
    );
  }
  log(`Binary ready at ${BINARY} (v${installed})`);
}

// --- Ensure dependencies ---

// After a Claude Code marketplace autoUpdate (git pull), node_modules/
// may be missing since it's gitignored. Detect and self-heal before any
// imports that depend on npm packages (e.g. 'which', 'cross-spawn').
//
// Resolvability is the test, not the presence of PLUGIN_ROOT/node_modules:
// in an npm install our dependencies are hoisted to the install root, so
// that directory is legitimately absent and running `bun install` there
// would be pure waste (and noisy failures when offline).
function dependenciesResolvable(): boolean {
  try {
    const req = createRequire(import.meta.url);
    req.resolve("cross-spawn");
    return true;
  } catch {
    return false;
  }
}

function ensureDependencies(): void {
  if (dependenciesResolvable()) return;
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
log(`SELF_ROOT=${SELF_ROOT}`);
log(`BINARY=${BINARY}`);

// Bundled binary: installed by npm via optionalDependencies, pinned to the
// exact plugin version, so npm owns its lifecycle (install = upgrade). We
// still verify the version — a hoisted resolution can land on a different
// install's copy, and silently running the wrong binary is the exact bug
// class this whole mechanism exists to kill.
let bundledBinary = resolveBundledBinary();
log(`BUNDLED_BINARY=${bundledBinary ?? "(none — using legacy download path)"}`);

if (bundledBinary) {
  const pluginVersion = getPluginVersion();
  const bundledVersion = getBinaryVersion(bundledBinary);
  if (!bundledVersion) {
    log(
      "WARNING: bundled binary is not executable, falling back to download path",
    );
    bundledBinary = null;
  } else if (
    pluginVersion &&
    !versionGte(bundledVersion.split("-")[0], pluginVersion)
  ) {
    log(
      `WARNING: bundled binary v${bundledVersion} is older than plugin v${pluginVersion} ` +
        `(stale optionalDependency?), falling back to download path`,
    );
    bundledBinary = null;
  } else {
    log(
      `Using bundled binary v${bundledVersion} (plugin v${pluginVersion ?? "unknown"})`,
    );
  }
}

if (!bundledBinary) {
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
      if (
        !binaryVersion ||
        (pluginVersion && !versionGte(binaryVersion, pluginVersion))
      ) {
        needsDownload = true;
        log(
          `Release binary v${binaryVersion} is older than plugin v${pluginVersion}, re-downloading`,
        );
      } else {
        log(
          `Release binary v${binaryVersion ?? "unknown"} (plugin v${pluginVersion ?? "unknown"})`,
        );
      }
    }
  }

  if (needsDownload) {
    downloadBinary();
  }
}

// Execute the aide binary, replacing this process
const execBinary = bundledBinary ?? BINARY;
log(`Executing: ${execBinary} ${process.argv.slice(2).join(" ")}`);

// Args are passed as an argv array, never through a shell.
const result = spawnSync(execBinary, process.argv.slice(2), {
  stdio: "inherit",
  env: { ...process.env, AIDE_PLUGIN_ROOT: PLUGIN_ROOT },
});

// Forward the exit code
process.exit(result.status ?? 1);
