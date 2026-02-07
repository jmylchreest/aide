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

import { existsSync, mkdirSync, readFileSync, chmodSync, unlinkSync } from 'fs';
import { join, dirname } from 'path';
import { execSync, spawn } from 'child_process';
import { fileURLToPath } from 'url';
import { homedir } from 'os';

export interface DownloadResult {
  success: boolean;
  path: string | null;
  message: string;
  cached: boolean;
}

/**
 * Get the plugin version from package.json
 */
export function getPluginVersion(): string | null {
  const pluginRoot = getPluginRoot();
  if (!pluginRoot) return null;

  try {
    const pkgPath = join(pluginRoot, 'package.json');
    if (existsSync(pkgPath)) {
      const pkg = JSON.parse(readFileSync(pkgPath, 'utf-8'));
      if (pkg.version && pkg.version !== '0.0.0') {
        return pkg.version;
      }
    }
  } catch {
    // skip invalid files
  }

  return null;
}

/**
 * Get the download URL for the current platform
 */
export function getDownloadUrl(): string {
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
 * Get the plugin root directory (where package.json lives)
 */
export function getPluginRoot(): string | null {
  // Check CLAUDE_PLUGIN_ROOT env var first (set by Claude Code)
  const envRoot = process.env.CLAUDE_PLUGIN_ROOT;
  if (envRoot && existsSync(join(envRoot, 'package.json'))) {
    return envRoot;
  }

  // Calculate from this script's location: dist/lib/aide-downloader.js -> ../../
  try {
    const scriptPath = fileURLToPath(import.meta.url);
    const pluginRoot = join(dirname(scriptPath), '..', '..');
    if (existsSync(join(pluginRoot, 'package.json'))) {
      return pluginRoot;
    }
  } catch {
    // import.meta.url not available
  }

  return null;
}

/**
 * Find the aide binary in common locations
 *
 * Priority order:
 * 1. Plugin's bin/aide (installed with plugin via postinstall)
 * 2. Project's .aide/bin/aide (legacy/override)
 * 3. ~/.aide/bin/aide (global install)
 * 4. PATH
 */
export function findAideBinary(cwd?: string): string | null {
  // 1. Check plugin's bin directory (primary location - installed via postinstall)
  const pluginRoot = getPluginRoot();
  if (pluginRoot) {
    const pluginBinary = join(pluginRoot, 'bin', 'aide');
    if (existsSync(pluginBinary)) {
      return pluginBinary;
    }
  }

  // 2. Check project-local .aide/bin directory (legacy/override)
  if (cwd) {
    const projectBinary = join(cwd, '.aide', 'bin', 'aide');
    if (existsSync(projectBinary)) {
      return projectBinary;
    }
  }

  // 3. Check home directory
  const homeBinary = join(homedir(), '.aide', 'bin', 'aide');
  if (existsSync(homeBinary)) {
    return homeBinary;
  }

  // 4. Check if in PATH
  try {
    execSync('which aide', { stdio: 'pipe', timeout: 2000 });
    return 'aide'; // Use from PATH
  } catch {
    // Not in PATH
  }

  return null;
}


/**
 * Download aide binary
 *
 * @param destDir - Directory to install the binary (e.g., plugin's bin/ or project's .aide/bin/)
 * @param options - Download options
 */
export async function downloadAideBinary(
  destDir: string,
  options: { force?: boolean; quiet?: boolean } = {}
): Promise<DownloadResult> {
  const { force = false, quiet = false } = options;

  const ext = process.platform === 'win32' ? '.exe' : '';
  const destPath = join(destDir, `aide${ext}`);

  // Check if already exists (unless force)
  if (!force && existsSync(destPath)) {
    // Verify it works
    try {
      execSync(`"${destPath}" version`, { stdio: 'pipe', timeout: 5000 });
      return {
        success: true,
        path: destPath,
        message: `Using existing aide at ${destPath}`,
        cached: true,
      };
    } catch {
      // Existing binary is broken, re-download
      if (!quiet) console.log('Existing binary is invalid, re-downloading...');
      try {
        unlinkSync(destPath);
      } catch {
        // Ignore delete errors
      }
    }
  }

  const url = getDownloadUrl();

  if (!quiet) {
    console.log(`Downloading aide binary...`);
    console.log(`  Version: ${getPluginVersion() || 'latest'}`);
    console.log(`  URL: ${url}`);
    console.log(`  Dest: ${destPath}`);
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
    const curlArgs = ['-fSL', '--progress-bar', '-o', destPath, url];

    await new Promise<void>((resolve, reject) => {
      const curl = spawn('curl', curlArgs, {
        stdio: quiet ? 'pipe' : ['pipe', 'pipe', 'inherit'], // stderr shows progress
      });

      curl.on('close', (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`curl exited with code ${code}`));
        }
      });

      curl.on('error', reject);
    });

    // Make executable
    if (process.platform !== 'win32') {
      chmodSync(destPath, 0o755);
    }

    // Verify it works
    try {
      execSync(`"${destPath}" version`, { stdio: 'pipe', timeout: 5000 });
    } catch {
      // Version command might not exist, try --help
      execSync(`"${destPath}" --help`, { stdio: 'pipe', timeout: 5000 });
    }

    if (!quiet) {
      console.log(`\n✓ Downloaded aide to ${destPath}`);
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
      console.error(`\n✗ Download failed: ${errorMsg}`);
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
 * Ensure aide binary is present (check only, no download)
 *
 * The binary should be installed via postinstall. This just checks it exists.
 */
export function ensureAideBinary(
  cwd?: string
): { binary: string | null; error: string | null } {
  const existing = findAideBinary(cwd);
  if (existing) {
    return { binary: existing, error: null };
  }

  // Build error message
  const pluginRoot = getPluginRoot();
  const expectedPath = pluginRoot ? join(pluginRoot, 'bin', 'aide') : 'plugin/bin/aide';

  const errorMsg = `**AIDE Binary Not Found**

The aide binary was not installed with the plugin.

**To fix this:**

1. **Reinstall the plugin:**
   \`\`\`bash
   claude plugin uninstall aide
   claude plugin install aide
   \`\`\`

2. **Or download manually** from GitHub releases:
   https://github.com/jmylchreest/aide/releases

   Place the binary at: \`${expectedPath}\``;

  return { binary: null, error: errorMsg };
}

// --- CLI Mode ---
// Run as standalone script for postinstall or manual download

async function main() {
  const args = process.argv.slice(2);

  let destDir: string | null = null;
  let force = false;
  let pluginMode = false;

  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--plugin') {
      pluginMode = true;
    } else if (args[i] === '--cwd' && args[i + 1]) {
      destDir = join(args[i + 1], '.aide', 'bin');
      i++;
    } else if (args[i] === '--dest' && args[i + 1]) {
      destDir = args[i + 1];
      i++;
    } else if (args[i] === '--force') {
      force = true;
    } else if (args[i] === '--help' || args[i] === '-h') {
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
      console.error('Error: Could not determine plugin root directory');
      process.exit(1);
    }
    destDir = join(pluginRoot, 'bin');
  } else if (!destDir) {
    // Default to current directory's .aide/bin
    destDir = join(process.cwd(), '.aide', 'bin');
  }

  const result = await downloadAideBinary(destDir, { force, quiet: false });

  if (result.success) {
    process.exit(0);
  } else {
    process.exit(1);
  }
}

// Run CLI if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((err) => {
    console.error('Error:', err);
    process.exit(1);
  });
}
