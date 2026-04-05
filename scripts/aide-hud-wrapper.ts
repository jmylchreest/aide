#!/usr/bin/env bun
/**
 * aide-hud-wrapper.ts - Installed to ~/.claude/bin/aide-hud.ts
 *
 * This is a thin wrapper that delegates to the real HUD script in the aide plugin.
 * This allows the plugin to update without requiring users to reinstall the wrapper.
 *
 * Cross-platform TypeScript HUD wrapper script.
 */

import { existsSync, readdirSync, statSync } from "fs";
import { join } from "path";
import spawn from "cross-spawn";
import { homedir } from "os";

/**
 * Find the newest aide plugin installation's HUD script
 */
function findHudScript(): string | null {
  const cacheDir = join(homedir(), ".claude", "plugins", "cache");

  if (!existsSync(cacheDir)) return null;

  let newest: { path: string; mtime: number } | null = null;

  // Walk the cache directory looking for aide/*/scripts/aide-hud.ts
  try {
    for (const entry of readdirSync(cacheDir)) {
      const entryPath = join(cacheDir, entry);
      try {
        const stat = statSync(entryPath);
        if (!stat.isDirectory()) continue;
      } catch {
        continue;
      }

      // Look for aide directories within
      const aideDir = join(entryPath, "aide");
      if (!existsSync(aideDir)) continue;

      try {
        for (const version of readdirSync(aideDir)) {
          const hudScript = join(aideDir, version, "scripts", "aide-hud.ts");
          if (existsSync(hudScript)) {
            try {
              const stat = statSync(hudScript);
              if (!newest || stat.mtimeMs > newest.mtime) {
                newest = { path: hudScript, mtime: stat.mtimeMs };
              }
            } catch {
              // skip
            }
          }
        }
      } catch {
        // skip unreadable dirs
      }
    }
  } catch {
    return null;
  }

  return newest?.path ?? null;
}

const script = findHudScript();

if (script) {
  const result = spawn.sync("bun", [script, ...process.argv.slice(2)], {
    stdio: "inherit",
  });
  process.exit(result.status ?? 0);
} else {
  console.log("[aide] not installed");
}
