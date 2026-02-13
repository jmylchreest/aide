#!/usr/bin/env npx tsx
/**
 * OpenCode adapter generator.
 *
 * Generates an opencode.json config file and optionally sets up
 * the aide plugin for local development.
 *
 * Usage:
 *   npx tsx adapters/opencode/generate.ts [--plugin-path /path/to/aide]
 *   npx tsx adapters/opencode/generate.ts --npm
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join, parse, resolve } from "path";
import { execSync } from "child_process";

const args = process.argv.slice(2);
const useNpm = args.includes("--npm");
const pluginPathIdx = args.indexOf("--plugin-path");
const pluginPath =
  pluginPathIdx >= 0 && args[pluginPathIdx + 1]
    ? resolve(args[pluginPathIdx + 1])
    : null;

const cwd = process.cwd();

/**
 * Detect if we're running inside the aide repo itself (or a workspace that
 * contains @jmylchreest/aide-plugin as a workspace package). In that case,
 * `npm install @jmylchreest/aide-plugin` would resolve locally via workspaces
 * instead of fetching from the npm registry, causing subtle breakage.
 */
function isInsideAideRepo(): boolean {
  // Walk up from cwd looking for a package.json with workspaces that would
  // resolve @jmylchreest/aide-plugin locally instead of from npm.
  let dir = cwd;
  const { root } = parse(dir);
  while (dir !== root) {
    const pkgPath = join(dir, "package.json");
    if (existsSync(pkgPath)) {
      try {
        const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
        if (
          pkg.workspaces &&
          Array.isArray(pkg.workspaces) &&
          pkg.workspaces.some(
            (ws: string) =>
              ws === "packages/*" || ws === "packages/opencode-plugin",
          )
        ) {
          // Check if the workspace package actually exists
          const pluginPkg = join(
            dir,
            "packages",
            "opencode-plugin",
            "package.json",
          );
          if (existsSync(pluginPkg)) {
            try {
              const sub = JSON.parse(readFileSync(pluginPkg, "utf-8"));
              if (sub.name === "@jmylchreest/aide-plugin") {
                return true;
              }
            } catch {}
          }
        }
      } catch {}
    }
    const parent = resolve(dir, "..");
    if (parent === dir) break;
    dir = parent;
  }
  return false;
}

interface OpenCodeConfig {
  $schema: string;
  plugin?: string[];
  mcp?: Record<
    string,
    {
      type?: string;
      command?: string[];
      environment?: Record<string, string>;
      enabled?: boolean;
    }
  >;
}

/**
 * Find the aide-wrapper.sh path.
 *
 * Resolution order:
 *   1. Local plugin path (--plugin-path)
 *   2. npm-installed @jmylchreest/aide-plugin package
 *   3. Fall back to "aide" in PATH
 */
function findWrapperCommand(): string[] {
  // Local development: use the wrapper from the plugin path
  if (pluginPath) {
    const wrapper = join(pluginPath, "bin", "aide-wrapper.sh");
    if (existsSync(wrapper)) {
      return [wrapper, "mcp"];
    }
  }

  // npm-installed package: use the plugin's built-in mcp subcommand.
  // The -y flag auto-installs if not already present.
  // Uses bunx since the package ships TypeScript with #!/usr/bin/env bun
  if (useNpm) {
    return ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"];
  }

  // No plugin path, not npm: assume aide is in PATH
  return ["aide", "mcp"];
}

function generateConfig(): OpenCodeConfig {
  const config: OpenCodeConfig = {
    $schema: "https://opencode.ai/config.json",
  };

  // Plugin configuration
  if (useNpm) {
    config.plugin = ["@jmylchreest/aide-plugin"];
  }
  // If using local path, user should symlink to .opencode/plugins/

  // MCP configuration for aide tools
  const command = findWrapperCommand();
  const environment: Record<string, string> = {
    AIDE_CODE_WATCH: "1",
    AIDE_CODE_WATCH_DELAY: "30s",
  };

  // Set AIDE_PLUGIN_ROOT so the wrapper knows where to find/download the binary
  if (pluginPath) {
    environment.AIDE_PLUGIN_ROOT = pluginPath;
  }
  // For --npm, AIDE_PLUGIN_ROOT is NOT set in the config — the wrapper resolves
  // its own package root at runtime by following its symlink (see aide-wrapper.sh).

  config.mcp = {
    aide: {
      type: "local",
      command,
      environment,
      enabled: true,
    },
  };

  return config;
}

function main(): void {
  console.log("aide OpenCode adapter generator\n");

  // Guard: detect running inside the aide repo with --npm
  if (useNpm && isInsideAideRepo()) {
    console.error(
      "Error: You appear to be inside the aide repository (or a workspace\n" +
        "containing @jmylchreest/aide-plugin). Using --npm here would cause\n" +
        "npm to resolve the package from the local workspace instead of the\n" +
        "npm registry.\n\n" +
        "For local development, use --plugin-path instead:\n" +
        `  npx tsx adapters/opencode/generate.ts --plugin-path ${process.cwd()}\n`,
    );
    process.exit(1);
  }

  // Generate opencode.json
  const config = generateConfig();
  const configPath = join(cwd, "opencode.json");

  if (existsSync(configPath)) {
    console.log(`opencode.json already exists at ${configPath}`);
    console.log("Skipping config generation. Merge manually if needed.\n");
    console.log("Suggested merge:");
    console.log(JSON.stringify(config, null, 2));
  } else {
    writeFileSync(configPath, JSON.stringify(config, null, 2) + "\n");
    console.log(`Created: ${configPath}`);
  }

  // Set up local plugin if path provided
  if (pluginPath && !useNpm) {
    const pluginDir = join(cwd, ".opencode", "plugins");
    mkdirSync(pluginDir, { recursive: true });

    const symlinkTarget = join(pluginDir, "aide.ts");
    const content = `// aide OpenCode plugin (local development)
// This file re-exports from the aide source tree.
// For production, use: "plugin": ["@jmylchreest/aide-plugin"] in opencode.json
export { AidePlugin as default } from "${pluginPath}/dist/opencode/index.js";
`;

    writeFileSync(symlinkTarget, content);
    console.log(`Created: ${symlinkTarget}`);
    console.log(`  Points to: ${pluginPath}/dist/opencode/`);
  }

  console.log("\nSetup complete. Start OpenCode to use aide integration.");

  if (useNpm) {
    console.log("\nInstall the plugin package:");
    console.log("  npm install @jmylchreest/aide-plugin");
  } else if (!pluginPath) {
    console.log("\nMake sure the aide binary is in your PATH:");
    console.log(
      "  which aide || echo 'aide not found - install from GitHub releases'",
    );
  }
}

main();
