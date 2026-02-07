#!/usr/bin/env node
/**
 * AIDE Binary Downloader
 *
 * Downloads the aide binary from GitHub releases.
 *
 * Usage:
 *   node dist/lib/aide-downloader.js --plugin    # Install to plugin's bin/ (postinstall)
 *   node dist/lib/aide-downloader.js --cwd /path # Install to project's .aide/bin/
 *
 * The binary is downloaded from the release matching the plugin version.
 */

import { existsSync, mkdirSync, readFileSync, chmodSync, unlinkSync, realpathSync } from "fs";
import { join, dirname } from "path";
import { execSync, spawn } from "child_process";
import { fileURLToPath } from "url";
// Canonical binary finder — import for local use, re-export for backward compat
import { findAideBinary } from "./hook-utils.js";
export { findAideBinary };

export interface DownloadResult {
  success: boolean;
  path: string | null;
  message: string;
  cached: boolean;
}

export interface EnsureResult {
  binary: string | null;
  error: string | null; // Critical error (can't use AIDE)
  warning: string | null; // Soft warning (update available, etc.)
  downloaded: boolean; // Whether we downloaded the binary this time
}

/**
 * Get the plugin version from package.json
 */
export function getPluginVersion(): string | null {
  const pluginRoot = getPluginRoot();
  if (!pluginRoot) return null;

  try {
    const pkgPath = join(pluginRoot, "package.json");
    if (existsSync(pkgPath)) {
      const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (pkg.version && pkg.version !== "0.0.0") {
        return pkg.version;
      }
    }
  } catch {
    // skip invalid files
  }

  return null;
}

/**
 * Get the version of an installed aide binary
 * Returns full semver including prerelease (e.g., "0.0.5-dev.21+abc1234")
 */
export function getBinaryVersion(binaryPath: string): string | null {
  try {
    const output = execSync(`"${binaryPath}" version`, {
      stdio: "pipe",
      timeout: 5000,
    })
      .toString()
      .trim();
    // Match full semver: 0.0.5-dev.21+abc1234 or plain 0.0.4
    const match = output.match(
      /(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?(?:\+[a-zA-Z0-9.]+)?)/,
    );
    return match ? match[1] : null;
  } catch {
    return null;
  }
}

/**
 * Check if a version string is a dev/prerelease build
 */
export function isDevBuild(version: string): boolean {
  return version.includes("-dev.");
}

/**
 * Extract the base semver (major.minor.patch) from a full version string
 * e.g., "0.0.5-dev.21+abc1234" → "0.0.5"
 */
export function getBaseVersion(version: string): string {
  const dashIdx = version.indexOf("-");
  return dashIdx === -1 ? version : version.substring(0, dashIdx);
}

/**
 * Check the latest release version from GitHub API
 * Returns null if check fails (network error, rate limit, etc.)
 */
export async function getLatestGitHubVersion(): Promise<string | null> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);

    const response = await fetch(
      "https://api.github.com/repos/jmylchreest/aide/releases/latest",
      {
        headers: {
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "aide-plugin",
        },
        signal: controller.signal,
      },
    );

    clearTimeout(timeout);

    if (!response.ok) {
      return null;
    }

    const data = (await response.json()) as { tag_name?: string };
    // tag_name is typically "v0.0.4", strip the "v"
    const tagName = data.tag_name;
    if (tagName && tagName.startsWith("v")) {
      return tagName.slice(1);
    }
    return tagName || null;
  } catch {
    // Network error, timeout, etc. - don't block on this
    return null;
  }
}

/**
 * Compare semantic versions
 * Returns: -1 if a < b, 0 if a == b, 1 if a > b
 */
export function compareVersions(a: string, b: string): number {
  const partsA = a.split(".").map(Number);
  const partsB = b.split(".").map(Number);

  for (let i = 0; i < 3; i++) {
    const numA = partsA[i] || 0;
    const numB = partsB[i] || 0;
    if (numA < numB) return -1;
    if (numA > numB) return 1;
  }
  return 0;
}

/**
 * Get the download URL for the current platform
 */
export function getDownloadUrl(): string {
  const platform = process.platform; // 'darwin', 'linux', 'win32'
  const arch = process.arch; // 'x64', 'arm64'

  const goos = platform === "win32" ? "windows" : platform;
  const goarch = arch === "x64" ? "amd64" : arch;
  const ext = platform === "win32" ? ".exe" : "";

  const binaryName = `aide-${goos}-${goarch}${ext}`;

  const version = getPluginVersion();
  if (version) {
    return `https://github.com/jmylchreest/aide/releases/download/v${version}/${binaryName}`;
  }

  // Fallback to latest if version can't be determined
  return `https://github.com/jmylchreest/aide/releases/latest/download/${binaryName}`;
}

/**
 * Get the plugin root directory (where package.json lives)
 */
export function getPluginRoot(): string | null {
  // Check CLAUDE_PLUGIN_ROOT env var first (set by Claude Code)
  const envRoot = process.env.CLAUDE_PLUGIN_ROOT;
  if (envRoot && existsSync(join(envRoot, "package.json"))) {
    return envRoot;
  }

  // Calculate from this script's location: dist/lib/aide-downloader.js -> ../../
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

// findAideBinary is imported from hook-utils.ts and re-exported at the top.
// See hook-utils.ts for the canonical implementation.

/**
 * Download aide binary
 *
 * @param destDir - Directory to install the binary (e.g., plugin's bin/ or project's .aide/bin/)
 * @param options - Download options
 */
export async function downloadAideBinary(
  destDir: string,
  options: { force?: boolean; quiet?: boolean; useStderr?: boolean } = {},
): Promise<DownloadResult> {
  const { force = false, quiet = false, useStderr = false } = options;

  // Use stderr for output when called from hooks (stdout reserved for JSON)
  const log = useStderr ? console.error : console.log;

  const ext = process.platform === "win32" ? ".exe" : "";
  const destPath = join(destDir, `aide${ext}`);

  // Check if already exists (unless force)
  if (!force && existsSync(destPath)) {
    // Verify it works
    try {
      execSync(`"${destPath}" version`, { stdio: "pipe", timeout: 5000 });
      return {
        success: true,
        path: destPath,
        message: `Using existing aide at ${destPath}`,
        cached: true,
      };
    } catch {
      // Existing binary is broken, re-download
      if (!quiet) log("[aide] Existing binary is invalid, re-downloading...");
      try {
        unlinkSync(destPath);
      } catch {
        // Ignore delete errors
      }
    }
  }

  const url = getDownloadUrl();

  if (!quiet) {
    log(`[aide] Downloading from: ${url}`);
  }

  try {
    // Create bin directory
    if (!existsSync(destDir)) {
      mkdirSync(destDir, { recursive: true });
    }

    // Use curl with progress
    // -f: fail silently on HTTP errors
    // -S: show errors
    // -L: follow redirects
    // --progress-bar: show progress
    const curlArgs = ["-fSL", "--progress-bar", "-o", destPath, url];

    await new Promise<void>((resolve, reject) => {
      const curl = spawn("curl", curlArgs, {
        stdio: quiet ? "pipe" : ["pipe", "pipe", "inherit"], // stderr shows progress
      });

      curl.on("close", (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`curl exited with code ${code}`));
        }
      });

      curl.on("error", reject);
    });

    // Make executable
    if (process.platform !== "win32") {
      chmodSync(destPath, 0o755);
    }

    // Verify it works
    try {
      execSync(`"${destPath}" version`, { stdio: "pipe", timeout: 5000 });
    } catch {
      // Version command might not exist, try --help
      execSync(`"${destPath}" --help`, { stdio: "pipe", timeout: 5000 });
    }

    if (!quiet) {
      log(`[aide] ✓ Binary installed successfully`);
    }

    return {
      success: true,
      path: destPath,
      message: `Downloaded aide to ${destPath}`,
      cached: false,
    };
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);

    if (!quiet) {
      log(`[aide] ✗ Download failed: ${errorMsg}`);
    }

    return {
      success: false,
      path: null,
      message: `Failed to download aide: ${errorMsg}`,
      cached: false,
    };
  }
}

/**
 * Ensure aide binary is present with version checking and auto-download
 *
 * This function:
 * 1. Checks if binary exists
 * 2. Verifies binary version matches plugin version
 * 3. Auto-downloads if missing or version mismatch
 * 4. Checks GitHub for newer releases (non-blocking)
 * 5. Returns warnings for plugin updates
 */
export async function ensureAideBinary(cwd?: string): Promise<EnsureResult> {
  const pluginRoot = getPluginRoot();
  const pluginVersion = getPluginVersion();

  let downloaded = false;
  let warning: string | null = null;

  // Step 1: Check for existing binary
  let binaryPath = findAideBinary(cwd);
  let binaryVersion = binaryPath ? getBinaryVersion(binaryPath) : null;

  // Step 2: Check version match
  // Skip download if the binary is a dev build with base version >= plugin version
  // (i.e., a locally-built binary that's ahead of the released plugin)
  const isNewerDev =
    binaryVersion &&
    pluginVersion &&
    isDevBuild(binaryVersion) &&
    compareVersions(getBaseVersion(binaryVersion), pluginVersion) >= 0;

  const needsDownload =
    !binaryPath ||
    (!isNewerDev &&
      pluginVersion &&
      binaryVersion &&
      binaryVersion !== pluginVersion);

  if (isNewerDev) {
    console.error(
      `[aide] Dev build v${binaryVersion} (base ${getBaseVersion(binaryVersion!)} >= plugin v${pluginVersion}), skipping download`,
    );
  }

  if (needsDownload) {
    // Can't download without plugin root
    if (!pluginRoot) {
      return {
        binary: binaryPath,
        error: binaryPath
          ? null
          : "AIDE binary not found and plugin root could not be determined for download",
        warning: null,
        downloaded: false,
      };
    }

    const destDir = join(pluginRoot, "bin");
    const reason = !binaryPath
      ? "not found"
      : `outdated (v${binaryVersion} → v${pluginVersion})`;

    // Print to stderr so user sees progress (stdout is reserved for JSON hook response)
    console.error(
      `[aide] Binary ${reason}, downloading v${pluginVersion || "latest"}...`,
    );

    // Auto-download
    const result = await downloadAideBinary(destDir, {
      force: true,
      quiet: false,
      useStderr: true,
    });

    if (result.success && result.path) {
      binaryPath = result.path;
      binaryVersion = getBinaryVersion(result.path);
      downloaded = true;
    } else {
      // Download failed - return error with manual instructions
      const platform =
        process.platform === "win32" ? "windows" : process.platform;
      const arch = process.arch === "x64" ? "amd64" : process.arch;
      const ext = process.platform === "win32" ? ".exe" : "";
      const binaryName = `aide-${platform}-${arch}${ext}`;

      const errorMsg = `**AIDE Binary ${!binaryPath ? "Not Found" : "Outdated"}**

Auto-download failed: ${result.message}

**Manual fix:**
\`\`\`bash
# Download the binary
curl -fSL "https://github.com/jmylchreest/aide/releases/download/v${pluginVersion || "latest"}/${binaryName}" -o "${destDir}/aide${ext}"
chmod +x "${destDir}/aide${ext}"
\`\`\`

Or visit: https://github.com/jmylchreest/aide/releases`;

      return {
        binary: null,
        error: errorMsg,
        warning: null,
        downloaded: false,
      };
    }
  }

  // Step 3: Check for newer GitHub release (non-blocking)
  if (pluginVersion) {
    try {
      console.error(`[aide] Checking for updates...`);
      const latestVersion = await getLatestGitHubVersion();
      if (latestVersion && compareVersions(pluginVersion, latestVersion) < 0) {
        warning = `**AIDE Update Available**

A newer version of AIDE is available: **v${latestVersion}** (you have v${pluginVersion})

\`\`\`bash
claude plugin update aide
\`\`\`

Then restart Claude Code to use the new version.`;
      }
    } catch {
      // Ignore - version check is best-effort
    }
  }

  return {
    binary: binaryPath,
    error: null,
    warning,
    downloaded,
  };
}

/**
 * Synchronous version for simple existence check (no download, no version check)
 * Use ensureAideBinary() for full functionality
 */
export function findAideBinarySync(cwd?: string): string | null {
  return findAideBinary(cwd);
}

// --- CLI Mode ---
// Run as standalone script for postinstall or manual download

async function main() {
  const args = process.argv.slice(2);

  let destDir: string | null = null;
  let force = false;
  let pluginMode = false;

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--plugin") {
      pluginMode = true;
    } else if (args[i] === "--cwd" && args[i + 1]) {
      destDir = join(args[i + 1], ".aide", "bin");
      i++;
    } else if (args[i] === "--dest" && args[i + 1]) {
      destDir = args[i + 1];
      i++;
    } else if (args[i] === "--force") {
      force = true;
    } else if (args[i] === "--help" || args[i] === "-h") {
      console.log(`AIDE Binary Downloader

Usage: node aide-downloader.js [options]

Options:
  --plugin      Install to plugin's bin/ directory (used by postinstall)
  --cwd <path>  Install to <path>/.aide/bin/
  --dest <path> Install to specific directory
  --force       Download even if binary exists
  --help        Show this help

Downloads the aide binary from GitHub releases.
`);
      process.exit(0);
    }
  }

  // Determine destination
  if (pluginMode) {
    const pluginRoot = getPluginRoot();
    if (!pluginRoot) {
      console.error("Error: Could not determine plugin root directory");
      process.exit(1);
    }
    destDir = join(pluginRoot, "bin");
  } else if (!destDir) {
    // Default to current directory's .aide/bin
    destDir = join(process.cwd(), ".aide", "bin");
  }

  const result = await downloadAideBinary(destDir, { force, quiet: false });

  if (result.success) {
    process.exit(0);
  } else {
    process.exit(1);
  }
}

// Run CLI if executed directly
// Use realpath to handle symlinks (e.g., src-office -> dtkr4-cnjjf)
const isMain = (() => {
  try {
    const scriptPath = realpathSync(fileURLToPath(import.meta.url));
    const argPath = realpathSync(process.argv[1]);
    return scriptPath === argPath;
  } catch {
    return false;
  }
})();

if (isMain) {
  main().catch((err) => {
    console.error("Error:", err);
    process.exit(1);
  });
}
