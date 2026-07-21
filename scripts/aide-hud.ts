#!/usr/bin/env bun
/**
 * aide-hud.ts - Status line display for aide plugin
 *
 * Reads HUD state from <project-root>/.aide/state/hud.txt and outputs
 * formatted status. Falls back to minimal display if state doesn't exist.
 *
 * Root resolution ladder (matching the anchor design — this script must
 * not grow its own resolver semantics):
 *   1. session anchor cache (~/.aide/anchors/<session_id>.json) — the
 *      statusline stdin JSON carries session_id + cwd
 *   2. AIDE_PROJECT_ROOT
 *   3. minimal .aide walk from the statusline cwd (last resort only; a
 *      session that ran session-start always hits the anchor cache)
 *
 * Cross-platform TypeScript HUD status line script.
 */

import { existsSync, readFileSync } from "fs";
import { join, dirname, sep } from "path";
import { homedir } from "os";

interface StatuslineInput {
  session_id?: string;
  cwd?: string;
}

function readStatuslineInput(): StatuslineInput {
  try {
    // Manual terminal runs have a TTY on stdin — reading would block
    // until EOF. Statusline invocations pipe JSON and close the pipe.
    if (process.stdin.isTTY) return {};
    const raw = readFileSync(0, "utf-8");
    if (!raw.trim()) return {};
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    return {
      session_id:
        typeof parsed.session_id === "string" ? parsed.session_id : undefined,
      cwd: typeof parsed.cwd === "string" ? parsed.cwd : undefined,
    };
  } catch {
    return {};
  }
}

function readAnchorRoot(sessionId: string, cwd?: string): string | null {
  try {
    if (!/^[a-zA-Z0-9_-]{1,128}$/.test(sessionId)) return null;
    // Same location contract as lib/anchor.ts anchorCacheDirs.
    const dirs: string[] = [];
    const xdg = process.env.XDG_RUNTIME_DIR;
    if (xdg && existsSync(xdg)) dirs.push(join(xdg, "aide", "anchors"));
    dirs.push(join(homedir(), ".aide", "anchors"));
    const p = dirs
      .map((d) => join(d, `${sessionId}.json`))
      .find((f) => existsSync(f));
    if (!p) return null;
    const entry = JSON.parse(readFileSync(p, "utf-8"));
    const root = entry?.anchor?.root;
    if (entry?.anchor?.schemaVersion !== 1 || typeof root !== "string" || !root)
      return null;
    // The statusline cwd may drift below the launch dir; require only that
    // the recorded root still exists and contains (or equals) the cwd.
    if (!existsSync(root)) return null;
    if (cwd && cwd !== root && !cwd.startsWith(root + sep)) return null;
    return root;
  } catch {
    return null;
  }
}

function walkForAide(startDir: string): string | null {
  let dir = startDir;
  while (true) {
    if (existsSync(join(dir, ".aide"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

const input = readStatuslineInput();
const startCwd = input.cwd || process.cwd();

let projectRoot: string | null = null;
if (input.session_id) {
  projectRoot = readAnchorRoot(input.session_id, startCwd);
}
if (!projectRoot) {
  const override = process.env.AIDE_PROJECT_ROOT;
  if (override && existsSync(override)) projectRoot = override;
}
if (!projectRoot) {
  projectRoot = walkForAide(startCwd) ?? startCwd;
}

const hudFile = join(projectRoot, ".aide", "state", "hud.txt");

if (existsSync(hudFile)) {
  try {
    const content = readFileSync(hudFile, "utf-8").trim();
    if (content) {
      console.log(content);
      process.exit(0);
    }
  } catch {
    // fall through to default
  }
}

// No state at all - show minimal status
console.log("[aide] idle");
