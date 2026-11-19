import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import type { ToolUseInfo } from "../core/types.js";
import type { ToolCompletionInfo } from "../core/partial-memory.js";
import { debug } from "./logger.js";

export type ObservationTask = {
  binary: string;
  cwd: string;
} & (
  | { kind: "command"; args: string[]; input?: string }
  | { kind: "tool-start"; info: ToolUseInfo }
  | { kind: "tool-stats"; tool: string; session: string; agent: string }
  | { kind: "partial-memory"; info: ToolCompletionInfo }
  | { kind: "session-summary"; session: string; startedAt?: string }
);

interface PendingTask {
  task: ObservationTask;
  env: Record<string, string>;
}

const MAX_PENDING_BYTES = 8 * 1024 * 1024;
const MAX_PENDING_TASKS = 128;
let pending: string[] = [];
let pendingBytes = 0;
let running = false;
let scheduled = false;

/** Capture routing when the event arrives, before another session changes it. */
function routingEnv(): Record<string, string> {
  return Object.fromEntries(
    Object.entries(process.env).filter(
      (entry): entry is [string, string] =>
        /^(AIDE_|CLAUDE_|CODEX_)/.test(entry[0]) && entry[1] !== undefined,
    ),
  );
}

/** Best-effort observations never hold up a hook's context or control result.
 * One worker at a time per producer drains bounded batches in submission order.
 * The worker owns backend timeouts; an exiting command hook does not wait for it.
 */
export function enqueueObservation(task: ObservationTask): boolean {
  try {
    const entry: PendingTask = { task, env: routingEnv() };
    const line = JSON.stringify(entry) + "\n";
    const size = Buffer.byteLength(line);
    if (
      pending.length >= MAX_PENDING_TASKS ||
      pendingBytes + size > MAX_PENDING_BYTES
    ) {
      debug("observability", "Background queue full; observation dropped");
      return false;
    }
    pending.push(line);
    pendingBytes += size;
    if (!running && !scheduled) {
      scheduled = true;
      setImmediate(flush);
    }
    return true;
  } catch {
    debug("observability", "Could not queue observation");
    return false;
  }
}

function flush(): void {
  scheduled = false;
  if (running || !pending.length) return;
  const input = pending.join("");
  pending = [];
  pendingBytes = 0;
  running = true;
  try {
    const worker = fileURLToPath(
      new URL("./observation-worker.ts", import.meta.url),
    );
    const child = spawn(process.execPath, [worker], {
      detached: true,
      windowsHide: true,
      stdio: ["pipe", "ignore", "ignore"],
      env: { ...process.env },
    });
    // Never inherit the host's output pipes: that would keep a synchronous
    // context hook alive until the supposedly background worker finishes.
    child.stdin.on("error", () => {});
    child.stdin.once("finish", () => child.stdin.destroy());
    child.stdin.end(input);
    child.on("error", () =>
      debug("observability", "Could not start background worker"),
    );
    child.once("close", (code) => {
      running = false;
      if (code !== 0)
        debug("observability", "Background worker failed; batch not retried");
      flush();
    });
    child.unref();
  } catch {
    running = false;
    debug("observability", "Could not launch background worker");
  }
}
