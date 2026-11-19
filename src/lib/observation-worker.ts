/** Private stdin-only worker. No host output or network fetches at startup. */
import { execFileSync } from "node:child_process";
import { trackToolUse, updateToolStats } from "../core/tool-tracking.js";
import {
  storePartialMemory,
  gatherPartials,
  buildSummaryFromPartials,
  cleanupPartials,
} from "../core/partial-memory.js";
import {
  buildSessionSummaryFromState,
  getSessionCommits,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import { startHookBudget, hookCommandTimeout } from "./hook-budget.js";
import { setSessionContext } from "./anchor.js";
import { debug, setDebugCwd } from "./logger.js";
import type { ObservationTask } from "./background-observability.js";

// Drain the pipe before doing backend work. Executing the first line while
// more input is pending would make a large batch block its producer on stdin.
const chunks: Buffer[] = [];
let bytes = 0;
for await (const chunk of process.stdin) {
  bytes += chunk.length;
  if (bytes > 8 * 1024 * 1024) process.exit(1);
  chunks.push(chunk);
}
const deadline = Date.now() + 10_000;
for (const line of Buffer.concat(chunks).toString("utf8").trim().split("\n")) {
  if (!line) continue;
  if (Date.now() >= deadline) {
    debug(
      "observability",
      "Background batch deadline reached; remaining observations dropped",
    );
    break;
  }
  try {
    const { task, env } = JSON.parse(line) as {
      task: ObservationTask;
      env: Record<string, string>;
    };
    for (const key of Object.keys(process.env)) {
      if (/^(AIDE_|CLAUDE_|CODEX_)/.test(key)) delete process.env[key];
    }
    Object.assign(process.env, env);
    process.env.AIDE_HOOK_DEADLINE_MS = String(deadline);
    startHookBudget("observation-worker");
    setSessionContext(
      process.env.AIDE_SESSION_ID || process.env.CLAUDE_SESSION_ID || "",
    );
    setDebugCwd(task.cwd);
    const started = Date.now();
    if (task.kind === "command") {
      execFileSync(task.binary, task.args, {
        cwd: task.cwd,
        input: task.input,
        stdio: ["pipe", "pipe", "pipe"],
        timeout: hookCommandTimeout(10_000),
        env: process.env,
        windowsHide: true,
      });
    } else if (task.kind === "tool-start") {
      trackToolUse(task.binary, task.cwd, task.info);
    } else if (task.kind === "tool-stats") {
      updateToolStats(
        task.binary,
        task.cwd,
        task.tool,
        task.session,
        task.agent,
      );
    } else if (task.kind === "partial-memory") {
      storePartialMemory(task.binary, task.cwd, task.info);
    } else if (task.kind === "session-summary") {
      const partials = gatherPartials(task.binary, task.cwd, task.session);
      const summary =
        (partials.length
          ? buildSummaryFromPartials(
              partials,
              getSessionCommits(task.cwd, task.startedAt),
              [],
            )
          : null) || buildSessionSummaryFromState(task.cwd, task.startedAt);
      // A timeout can leave the summary unwritten. Keep its source partials
      // until a later attempt acknowledges that the summary was saved.
      if (summary && storeSessionSummary(task.binary, task.cwd, task.session, summary))
        cleanupPartials(task.binary, task.cwd, task.session);
    }
    const elapsed = Date.now() - started;
    if (elapsed >= 500) {
      // Log the operation, never its arguments (which may contain user text).
      const operation =
        task.kind === "command" ? task.args.slice(0, 2).join(" ") : task.kind;
      debug(
        "observability",
        `Slow backend operation ${operation}: ${elapsed}ms`,
      );
    }
  } catch (error) {
    debug(
      "observability",
      `Background operation failed: ${error instanceof Error ? error.name : "error"}`,
    );
  }
}
