#!/usr/bin/env bun
/**
 * aide-hud-wrapper.ts - Installed to ~/.claude/bin/aide-hud.ts
 *
 * This is a thin wrapper that delegates to the real HUD script in the aide plugin.
 * This allows the plugin to update without requiring users to reinstall the wrapper.
 *
 * Locating the plugin is the whole job here. The statusline is a
 * user-settings command rather than a plugin hook, so the harness sets no
 * CLAUDE_PLUGIN_ROOT when it runs us. session-start does know the root, and
 * records it in the pointer file next to this script; the environment and
 * the marketplace cache are the other two ways it can turn up.
 *
 * The marker below makes the installed copy recognizably aide-managed:
 * session-start upgrades managed copies when the plugin ships a higher
 * version, and never touches files without a marker. Bump it whenever
 * this file changes.
 *
 * aide-wrapper-version: 3
 */

import { existsSync, readdirSync, readFileSync, statSync } from "fs";
import { join } from "path";
import spawn from "cross-spawn";
import { homedir } from "os";

// Kept in step with hudPointerFile() in src/lib/hud.ts.
const POINTER_FILE = join(homedir(), ".claude", "bin", "aide-hud.path");

/** The HUD script a plugin root would hold, if it is really there. */
function hudScriptIn(pluginRoot: string): string | null {
  const script = join(pluginRoot, "scripts", "aide-hud.ts");
  return existsSync(script) ? script : null;
}

/** Set when a hook or OpenCode invokes us rather than the statusline. */
function fromEnv(): string | null {
  const root = process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
  return root ? hudScriptIn(root) : null;
}

/** The root session-start resolved, rewritten on every session start. */
function fromPointer(): string | null {
  try {
    const root = readFileSync(POINTER_FILE, "utf-8").trim();
    return root ? hudScriptIn(root) : null;
  } catch {
    return null;
  }
}

/**
 * Newest marketplace-cache install. Only reachable before the first
 * session-start of a fresh install has written the pointer — and it misses
 * local-path installs entirely, which is why it is the last resort.
 */
function fromCache(): string | null {
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

const script = fromEnv() ?? fromPointer() ?? fromCache();

if (script) {
  const result = spawn.sync("bun", [script, ...process.argv.slice(2)], {
    stdio: "inherit",
  });
  process.exit(result.status ?? 0);
} else {
  // Never "not installed" — the rest of the plugin can be working fine and
  // only this lookup have failed. Name what could not be found.
  console.log("[aide] hud unavailable: no plugin root");
}
