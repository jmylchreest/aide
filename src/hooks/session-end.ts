#!/usr/bin/env node
/**
 * Session End Hook (SessionEnd)
 *
 * Delegates cleanup to `aide session end` — single binary invocation.
 *
 * CONSTRAINTS:
 * - No ES module imports (hoisted resolution adds ~3s)
 * - Output {"continue": true} before require() resolves
 * - Cleanup via detached spawn to avoid blocking exit
 * - MUST complete well within Claude Code's ~1.5s hook timeout
 *
 * STDIN:
 * Claude Code pipes JSON via stdin and closes the pipe. The documented
 * payload includes session_id, but in practice Claude Code often sends
 * just `{}`. Bun's `for await` async iterator on stdin hangs even when
 * the pipe is closed — use synchronous readFileSync(0) instead.
 */

const T0 = performance.now();

// Output continue IMMEDIATELY — before require(), before anything.
console.log(JSON.stringify({ continue: true }));

const { execFileSync, spawn } = require("child_process") as typeof import("child_process");
const { existsSync, realpathSync, appendFileSync, mkdirSync, readFileSync } = require("fs") as typeof import("fs");
const { join } = require("path") as typeof import("path");

const SESSION_ID_RE = /^[a-zA-Z0-9_-]{1,128}$/;

/** Elapsed ms since T0. */
function ms(): string {
  return `+${(performance.now() - T0).toFixed(0)}ms`;
}

/**
 * Always log to .aide/_logs/session-end.log (NOT gated on AIDE_DEBUG).
 * This hook has historically been invisible when it fails — always log.
 */
function log(cwd: string, msg: string): void {
  try {
    const logDir = join(cwd, ".aide", "_logs");
    if (!existsSync(logDir)) mkdirSync(logDir, { recursive: true });
    const line = `[${new Date().toISOString()}] [session-end] ${ms()} ${msg}\n`;
    appendFileSync(join(logDir, "session-end.log"), line);
  } catch { /* best effort */ }
}

/** Find aide binary — inline, no external module imports. */
function findBinary(cwd?: string): string | null {
  const pluginRoot = process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
  let resolvedRoot = pluginRoot;
  if (resolvedRoot) {
    try { resolvedRoot = realpathSync(resolvedRoot); } catch { /* symlink may not resolve */ }
    const p = join(resolvedRoot, "bin", "aide");
    if (existsSync(p)) return p;
  }
  if (cwd) {
    const p = join(cwd, ".aide", "bin", "aide");
    if (existsSync(p)) return p;
  }
  try {
    return execFileSync("which", ["aide"], { stdio: "pipe", timeout: 2000 })
      .toString().trim() || null;
  } catch { return null; }
}

function main(): void {
  const cwd = process.cwd();
  log(cwd, "started");

  try {
    // Read stdin synchronously — avoids bun's broken async iterator.
    // Claude Code closes the pipe so this returns immediately.
    let input = "";
    try {
      input = readFileSync(0, "utf-8");
    } catch {
      log(cwd, "stdin not readable (not a pipe?)");
    }
    log(cwd, `stdin (${input.length} bytes): ${input.trim().slice(0, 200)}`);

    // Parse session_id from stdin JSON
    let sessionId = "";
    if (input.trim()) {
      try {
        const data = JSON.parse(input);
        sessionId = data.session_id || "";
      } catch {
        log(cwd, "stdin is not valid JSON");
      }
    }

    if (!sessionId) {
      log(cwd, "no session_id in payload, skipping cleanup");
      return;
    }

    if (!SESSION_ID_RE.test(sessionId)) {
      log(cwd, `invalid session_id: ${sessionId}`);
      return;
    }

    const binary = findBinary(cwd);
    log(cwd, `binary: ${binary}`);
    if (!binary) {
      log(cwd, "no binary found, skipping cleanup");
      return;
    }

    log(cwd, `spawning cleanup: session end --session=${sessionId}`);
    const child = spawn(binary, ["session", "end", `--session=${sessionId}`], {
      cwd, detached: true, stdio: "ignore",
    });
    child.unref();
    log(cwd, "cleanup spawned (detached)");
  } catch (error) {
    log(cwd, `error: ${error}`);
  }

  log(cwd, `done ${ms()}`);
}

// On SIGINT/SIGTERM, exit cleanly (continue already output).
process.on("SIGINT", () => process.exit(0));
process.on("SIGTERM", () => process.exit(0));

main();
process.exit(0);
