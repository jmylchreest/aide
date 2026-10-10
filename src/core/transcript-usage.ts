/** Bounded catch-up for explicitly supplied host transcripts; never discovers files. */
import { execFileSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { isAbsolute } from "node:path";
import { statSync } from "node:fs";
import { collectTranscriptUsage, recordModelUsage } from "./model-usage.js";
import type { ObserveBatchEvent } from "./read-tracking.js";

type Host = "claude-code" | "codex";
const MAX_PATHS = 32;
const MAX_SCAN_PATHS = 4;
const MAX_EVENTS = 256;
const MAX_BYTES = 128 * 1024;
const MAX_STATE_OUTPUT = 512 * 1024;
type Run = (
  binary: string,
  cwd: string,
  args: string[],
  timeout: number,
) => string | null;
interface Dependencies {
  run: Run;
  write: typeof recordModelUsage;
  now: () => number;
}
const run: Run = (binary, cwd, args, timeout) => {
  try {
    return execFileSync(binary, args, {
      cwd,
      timeout,
      maxBuffer: MAX_STATE_OUTPUT,
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
      env: process.env,
    });
  } catch {
    return null;
  }
};
const defaults: Dependencies = { run, write: recordModelUsage, now: Date.now };
function validSession(session: string): boolean {
  return (
    typeof session === "string" &&
    session.trim().length > 0 &&
    session !== "unknown" &&
    session.length <= 1024
  );
}
function validPath(path: unknown): path is string {
  return (
    typeof path === "string" &&
    path.length <= 4096 &&
    !path.includes("\0") &&
    isAbsolute(path)
  );
}
export function usageTranscriptNamespace(host: Host, session: string): string {
  return `usage-transcripts:${createHash("sha256")
    .update(JSON.stringify([host, session]))
    .digest("hex")}`;
}

/** Immutable completion registrations avoid lost concurrent updates and stale deletes.
 * The store enforces the namespace cap atomically. Unsupported binaries fail closed.
 * Register even when the file has not been flushed yet; never infer a replacement path.
 */
export function registerChildTranscript(
  binary: string,
  cwd: string,
  host: Host,
  session: string,
  path: unknown,
  deps: Dependencies = defaults,
): boolean {
  if (host !== "claude-code" || !validSession(session) || !validPath(path))
    return false;
  const agent = usageTranscriptNamespace(host, session);
  const key = randomUUID();
  const value = JSON.stringify({
    version: 1,
    host,
    session,
    path,
    registeredAt: deps.now(),
  });
  const output = deps.run(
    binary,
    cwd,
    [
      "state",
      "init-bounded",
      key,
      value,
      `--agent=${agent}`,
      `--max-agent-entries=${MAX_PATHS}`,
      "--json",
    ],
    500,
  );
  if (output === null) return false;
  try {
    const ack = JSON.parse(output);
    return (
      ack?.key === `agent:${agent}:${key}` &&
      ack.agent === agent &&
      ack.value === value
    );
  } catch {
    return false;
  }
}

interface Registration {
  key: string;
  path: string;
  registeredAt: number;
}
function fileVersion(path: string): string | null {
  try {
    const stat = statSync(path);
    return stat.isFile()
      ? JSON.stringify([
          stat.dev,
          stat.ino,
          stat.size,
          stat.mtimeMs,
          stat.ctimeMs,
        ])
      : null;
  } catch {
    return null;
  }
}
export interface CatchUpResult {
  status: "partial" | "unavailable";
  paths: number;
  records: number;
  retired: number;
  limited: boolean;
  malformed: number;
  acknowledged: boolean;
  elapsedMs: number;
}
/** One bounded batch at a later real lifecycle boundary. No cursor advances before ack.
 * A complete child file with matching usage may retire only the selected registrations;
 * a concurrent new completion for that path has a different immutable key.
 * Main paths are supplied anew by the host; coverage always remains partial.
 */
export function catchUpTranscriptUsage(
  binary: string,
  cwd: string,
  host: Host,
  session: string,
  mainPath?: unknown,
  options: { budgetMs?: number } = {},
  deps: Dependencies = defaults,
): CatchUpResult {
  const started = deps.now();
  const deadline =
    started + Math.max(1, Math.min(2000, options.budgetMs ?? 2000));
  const remaining = () => Math.max(0, deadline - deps.now());
  const result: CatchUpResult = {
    status: "unavailable",
    paths: 0,
    records: 0,
    retired: 0,
    limited: false,
    malformed: 0,
    acknowledged: false,
    elapsedMs: 0,
  };
  const finish = () => {
    result.elapsedMs = deps.now() - started;
    return result;
  };
  if (!validSession(session) || (host !== "claude-code" && host !== "codex"))
    return finish();
  const agent = usageTranscriptNamespace(host, session);
  const registrations: Registration[] = [];
  // Codex has no registered Claude children and never searches their namespace.
  if (host === "claude-code") {
    const listTimeout = Math.min(300, remaining());
    if (listTimeout <= 0) {
      result.limited = true;
      return finish();
    }
    const output = deps.run(
      binary,
      cwd,
      ["state", "list", `--agent=${agent}`, "--json"],
      listTimeout,
    );
    try {
      const states: unknown = output === null ? undefined : JSON.parse(output);
      if (states !== null && !Array.isArray(states))
        throw new Error("invalid state list");
      const entries = (states ?? []) as Record<string, unknown>[];
      if (entries.length > MAX_PATHS) result.limited = true;
      for (const state of entries.slice(0, MAX_PATHS)) {
        if (
          state?.agent !== agent ||
          typeof state.key !== "string" ||
          typeof state.value !== "string"
        )
          continue;
        const prefix = `agent:${agent}:`;
        const key = state.key.startsWith(prefix)
          ? state.key.slice(prefix.length)
          : "";
        if (!/^[a-f0-9-]{36}$/.test(key)) continue;
        try {
          const value = JSON.parse(state.value);
          if (
            value.version === 1 &&
            value.host === host &&
            value.session === session &&
            validPath(value.path) &&
            Number.isSafeInteger(value.registeredAt) &&
            value.registeredAt < started
          )
            registrations.push({
              key,
              path: value.path,
              registeredAt: value.registeredAt,
            });
        } catch {
          result.limited = true;
        }
      }
    } catch {
      result.limited = true;
    }
  }
  const byPath = new Map<string, Registration[]>();
  for (const registration of registrations.sort(
    (a, b) => a.registeredAt - b.registeredAt || a.key.localeCompare(b.key),
  )) {
    const group = byPath.get(registration.path) ?? [];
    group.push(registration);
    byPath.set(registration.path, group);
  }
  const main = validPath(mainPath) ? mainPath : undefined;
  const paths = [...byPath.keys()].filter((path) => path !== main);
  // Reserve the main transcript's slot before rotating the remaining children.
  // Otherwise four retained children plus main always skip the fourth child.
  const childSlots = MAX_SCAN_PATHS - (main ? 1 : 0);
  const rotation =
    paths.length > childSlots ? Math.floor(started / 2000) % paths.length : 0;
  const selected = [...paths.slice(rotation), ...paths.slice(0, rotation)];
  if (main) selected.unshift(main);
  if (selected.length > MAX_SCAN_PATHS) result.limited = true;
  const events: ObserveBatchEvent[] = [];
  const retire: (Registration & { fileVersion: string })[] = [];
  const scanPaths = selected.slice(0, MAX_SCAN_PATHS);
  for (const [index, path] of scanPaths.entries()) {
    if (remaining() <= 0 || events.length >= MAX_EVENTS) {
      result.limited = true;
      break;
    }
    const before = fileVersion(path);
    const scan = collectTranscriptUsage(path, host, session, {
      maxBytes: MAX_BYTES,
      // Reserve capacity for every remaining path so a dense main transcript
      // cannot consume the whole batch before registered children are scanned.
      maxEvents: Math.floor(
        (MAX_EVENTS - events.length) / (scanPaths.length - index),
      ),
    });
    result.paths++;
    result.malformed += scan.malformed;
    result.limited ||= scan.limited;
    if (scan.status === "partial") result.status = "partial";
    events.push(...scan.events);
    if (
      scan.status !== "unavailable" &&
      !scan.limited &&
      !scan.malformed &&
      scan.events.length &&
      before !== null &&
      before === fileVersion(path)
    )
      retire.push(
        ...(byPath.get(path) ?? []).map((registration) => ({
          ...registration,
          fileVersion: before,
        })),
      );
  }
  result.records = events.length;
  const writeTimeout = Math.min(1000, remaining());
  if (events.length && writeTimeout > 0) {
    try {
      result.acknowledged = deps.write(binary, cwd, events, {
        timeout: writeTimeout,
      });
    } catch {
      result.acknowledged = false;
    }
  }
  if (result.acknowledged)
    for (const registration of retire) {
      const deleteTimeout = Math.min(150, remaining());
      if (deleteTimeout <= 0) {
        result.limited = true;
        break;
      }
      // A final row may flush during the write. Keep that registration retryable.
      if (fileVersion(registration.path) !== registration.fileVersion) {
        result.limited = true;
        continue;
      }
      const output = deps.run(
        binary,
        cwd,
        ["state", "delete", registration.key, `--agent=${agent}`],
        deleteTimeout,
      );
      if (output !== null) result.retired++;
      else result.limited = true;
    }
  if (remaining() <= 0) result.limited = true;
  return finish();
}
