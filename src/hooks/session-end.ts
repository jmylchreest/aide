#!/usr/bin/env node
/**
 * Session End Hook (SessionEnd)
 *
 * Delegates cleanup to `aide session end` — single binary invocation.
 *
 * When invoked from Codex CLI's Stop hook (which has no separate SessionEnd),
 * checks whether autopilot mode is active and skips cleanup if so — the
 * session is continuing, not ending.
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

const { spawn, execFileSync } =
  require("child_process") as typeof import("child_process");
const {
  existsSync,
  realpathSync,
  appendFileSync,
  mkdirSync,
  readFileSync,
  statSync,
} = require("fs") as typeof import("fs");
const { join, dirname, basename } = require("path") as typeof import("path");
const whichSync = (require("which") as typeof import("which")).sync;

/**
 * Inline walk-up for project root — mirrors lib/project-root.ts semantics
 * (closest VCS root wins, linked worktrees resolve to the main repo,
 * submodule checkouts anchor themselves; .aide/-only as fallback). Inlined
 * here to keep this hook's startup cheap (no extra ES imports); replaced by
 * the anchor reader once `aide anchor` lands.
 */
function resolveRoot(cwd: string): string {
  const override = process.env.AIDE_PROJECT_ROOT;
  if (override) {
    try {
      if (statSync(override).isDirectory()) return override;
    } catch {
      /* fall through */
    }
  }

  // "gitdir: <main>/.git/worktrees/<wt>" → main repo root; submodule
  // gitdirs (.git/modules/) return null so the checkout anchors itself.
  function resolveGitFile(gitFilePath: string): string | null {
    try {
      const content = readFileSync(gitFilePath, "utf-8").trim();
      if (!content.startsWith("gitdir:")) return null;
      let gitdir = content.slice("gitdir:".length).trim();
      if (!gitdir.startsWith("/")) {
        gitdir = join(dirname(gitFilePath), gitdir);
      }
      const parts = gitdir.split(/[\\/]+/);
      for (let i = 0; i < parts.length - 1; i++) {
        if (parts[i] === ".git" && parts[i + 1] === "modules") return null;
      }
      let candidate = gitdir;
      for (;;) {
        const parent = dirname(candidate);
        if (parent === candidate) break;
        if (basename(candidate) === ".git") return parent;
        candidate = parent;
      }
      return null;
    } catch {
      return null;
    }
  }

  const candidates: { dir: string; hasAide: boolean; vcsRoot: string }[] = [];
  let dir = cwd;
  for (;;) {
    const hasAide = existsSync(join(dir, ".aide"));
    let vcsRoot = "";
    const gitPath = join(dir, ".git");
    try {
      const st = statSync(gitPath);
      vcsRoot = st.isFile() ? resolveGitFile(gitPath) || dir : dir;
    } catch {
      for (const m of [".hg", ".svn", ".bzr", ".fossil"]) {
        if (existsSync(join(dir, m))) {
          vcsRoot = dir;
          break;
        }
      }
    }
    if (hasAide || vcsRoot) candidates.push({ dir, hasAide, vcsRoot });
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  for (const c of candidates) if (c.vcsRoot) return c.vcsRoot;
  for (const c of candidates) if (c.hasAide) return c.dir;
  return cwd;
}

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
    const logDir = join(resolveRoot(cwd), ".aide", "_logs");
    if (!existsSync(logDir)) mkdirSync(logDir, { recursive: true });
    const line = `[${new Date().toISOString()}] [session-end] ${ms()} ${msg}\n`;
    appendFileSync(join(logDir, "session-end.log"), line);
  } catch {
    /* best effort */
  }
}

/** Find aide binary — inline, no external module imports. */
function findBinary(cwd?: string): string | null {
  const pluginRoot =
    process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
  let resolvedRoot = pluginRoot;
  if (resolvedRoot) {
    try {
      resolvedRoot = realpathSync(resolvedRoot);
    } catch {
      /* symlink may not resolve */
    }
    const p = join(resolvedRoot, "bin", "aide");
    if (existsSync(p)) return p;
  }
  if (cwd) {
    const p = join(resolveRoot(cwd), ".aide", "bin", "aide");
    if (existsSync(p)) return p;
  }
  try {
    return whichSync("aide", { nothrow: true });
  } catch {
    return null;
  }
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

    // Parse session_id, event name, and optional duration (ms) from stdin
    let sessionId = "";
    let eventName = "";
    let durationMs = 0;
    if (input.trim()) {
      try {
        const data = JSON.parse(input);
        sessionId = data.session_id || "";
        eventName = data.hook_event_name || "";
        // Clamp to a finite safe integer — JSON.parse can yield Infinity
        // (1e999) and template serialization of huge numbers ("1e+21")
        // would fail Go-side strconv parsing.
        if (
          typeof data.duration === "number" &&
          Number.isFinite(data.duration) &&
          data.duration > 0 &&
          data.duration <= Number.MAX_SAFE_INTEGER
        ) {
          durationMs = Math.round(data.duration);
        }
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

    // Event guard: Codex has no SessionEnd event — its hooks.json maps the
    // per-turn Stop event to this hook, and running real teardown there would
    // clear counters and broadcast a "Session ended" message on EVERY turn.
    // Only genuine session ends may proceed (including the lifecycle record
    // below — a per-turn "session-end" trigger would be misleading telemetry).
    // (AIDE_PLATFORM=codex is set by the `aide-plugin hook` dispatcher.)
    if (eventName === "Stop" || process.env.AIDE_PLATFORM === "codex") {
      log(
        cwd,
        `per-turn event (event=${eventName || "?"} platform=${process.env.AIDE_PLATFORM || "?"}), skipping session teardown`,
      );
      return;
    }

    // Emit a lifecycle trigger so SessionEnd is traceable in the dashboard,
    // symmetric with session-start and subagent-start/stop. Inlined (no ES
    // import) to keep this hook's startup cheap. Fire-and-forget.
    try {
      execFileSync(
        binary,
        [
          "observe",
          "record",
          "--kind=session",
          "--name=session-end",
          "--category=lifecycle",
          `--session=${sessionId}`,
        ],
        { cwd, timeout: 3000, stdio: ["pipe", "pipe", "pipe"] },
      );
    } catch {
      // Non-fatal telemetry — never block session end.
    }

    // Mode guard: skip teardown while autopilot is active. Mode is global
    // by design (sessionless writers — see core/aide-client.ts). On a
    // genuine SessionEnd this is conservative: teardown is skipped even if
    // the autopilot belongs to a different concurrent session; session init
    // hygiene covers the leftovers.
    // NOTE: `state get` prints "key = value", so the value must be parsed
    // out — comparing the raw output to "autopilot" (as this guard
    // originally did) never matches.
    try {
      const out = execFileSync(binary, ["state", "get", "mode"], {
        cwd,
        timeout: 500,
        encoding: "utf-8",
      });
      const m = out.match(/=\s*(.+)$/m);
      if ((m ? m[1].trim() : "") === "autopilot") {
        log(
          cwd,
          "autopilot mode active, skipping cleanup (session continuing)",
        );
        return;
      }
    } catch {
      // Binary may not support 'state get' or state may not exist — proceed
    }

    const endArgs = ["session", "end", `--session=${sessionId}`];
    if (durationMs > 0) endArgs.push(`--duration=${durationMs}`);
    log(cwd, `spawning cleanup: ${endArgs.join(" ")}`);
    const child = spawn(binary, endArgs, {
      cwd,
      detached: true,
      stdio: "ignore",
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
