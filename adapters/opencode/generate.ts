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

import { existsSync, mkdirSync, writeFileSync } from "fs";
import { join, resolve } from "path";

const args = process.argv.slice(2);
const useNpm = args.includes("--npm");
const pluginPathIdx = args.indexOf("--plugin-path");
const pluginPath =
  pluginPathIdx >= 0 && args[pluginPathIdx + 1]
    ? resolve(args[pluginPathIdx + 1])
    : null;

const cwd = process.cwd();

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

function generateConfig(): OpenCodeConfig {
  const config: OpenCodeConfig = {
    $schema: "https://opencode.ai/config.json",
  };

  // Plugin configuration
  if (useNpm) {
    config.plugin = ["@aide/opencode-plugin"];
  }
  // If using local path, user should symlink to .opencode/plugins/

  // MCP configuration for aide tools
  config.mcp = {
    aide: {
      type: "local",
      command: ["aide", "mcp"],
      environment: {
        AIDE_CODE_WATCH: "1",
      },
      enabled: true,
    },
  };

  return config;
}

function main(): void {
  console.log("aide OpenCode adapter generator\n");

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
// For production, use: "plugin": ["@aide/opencode-plugin"] in opencode.json
export { AidePlugin as default } from "${pluginPath}/dist/opencode/index.js";
`;

    writeFileSync(symlinkTarget, content);
    console.log(`Created: ${symlinkTarget}`);
    console.log(`  Points to: ${pluginPath}/dist/opencode/`);
  }

  console.log("\nSetup complete. Start OpenCode to use aide integration.");

  if (useNpm) {
    console.log("\nInstall the plugin package:");
    console.log("  npm install @aide/opencode-plugin");
  }

  console.log("\nMake sure the aide binary is in your PATH:");
  console.log("  which aide || echo 'aide not found - install from GitHub releases'");
}

main();
