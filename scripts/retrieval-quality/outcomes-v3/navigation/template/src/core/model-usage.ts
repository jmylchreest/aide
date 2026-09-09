/** Host-reported counters. These are neither text estimates nor savings. */
import {
  openSync,
  closeSync,
  fstatSync,
  statSync,
  readSync,
  constants,
} from "node:fs";
import { isAbsolute } from "node:path";
import { execFileSync } from "node:child_process";
import { debug } from "../lib/logger.js";
import type { ObserveBatchEvent } from "./read-tracking.js";

type Host = "claude-code" | "codex" | "opencode";
type RecordValue = Record<string, unknown>;
function object(value: unknown): RecordValue | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as RecordValue)
    : undefined;
}
function id(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim().length > 0 &&
    value !== "unknown" &&
    value.length <= 1024
  );
}
function count(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function counter(
  attrs: Record<string, string>,
  name: string,
  value: unknown,
): void {
  if (count(value)) attrs[name] = String(value);
  else if (value !== undefined) attrs.usage_invalid = "1";
}
function summedInput(attrs: Record<string, string>): void {
  const fields = [
    "uncached_input_tokens",
    "cache_read_input_tokens",
    "cache_write_input_tokens",
  ];
  if (fields.every((key) => attrs[key] !== undefined)) {
    const total = fields.reduce((sum, key) => sum + Number(attrs[key]), 0);
    if (count(total)) attrs.input_tokens = String(total);
    else attrs.usage_invalid = "1";
  }
}
/** Validate before parsing: Date.parse normalizes impossible dates and accepts local time. */
function sourceTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const parts =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|([+-])(\d{2}):(\d{2}))$/.exec(
      value,
    );
  if (!parts) return false;
  const [
    ,
    yearText,
    monthText,
    dayText,
    hourText,
    minuteText,
    secondText,
    ,
    zoneHourText,
    zoneMinuteText,
  ] = parts;
  const year = Number(yearText),
    month = Number(monthText),
    day = Number(dayText);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= days[month - 1] &&
    Number(hourText) <= 23 &&
    Number(minuteText) <= 59 &&
    Number(secondText) <= 59 &&
    Number(zoneHourText ?? 0) <= 23 &&
    Number(zoneMinuteText ?? 0) <= 59 &&
    Number.isFinite(Date.parse(value))
  );
}

function event(
  host: Host,
  source: string,
  session: string,
  usageId: string,
  timestamp?: unknown,
): ObserveBatchEvent {
  const validTime = sourceTime(timestamp);
  return {
    kind: "session",
    name: "model_usage",
    session,
    ...(validTime ? { ts: timestamp } : {}),
    attrs: {
      model_usage_version: "1",
      host,
      usage_source: source,
      usage_id: usageId,
      usage_time_basis: validTime ? "source" : "observed",
      usage_coverage: "partial",
      ...(validTime
        ? { usage_source_time: timestamp }
        : timestamp !== undefined
          ? { usage_invalid: "1" }
          : {}),
    },
  };
}

export function claudeUsageEvent(
  rowValue: unknown,
  session: string,
): ObserveBatchEvent | null {
  const row = object(rowValue),
    message = object(row?.message),
    usage = object(message?.usage);
  if (
    !id(session) ||
    row?.type !== "assistant" ||
    row.sessionId !== session ||
    !id(message?.id) ||
    !usage
  )
    return null;
  const result = event(
    "claude-code",
    "claude.assistant_usage.v1",
    session,
    message.id,
    row.timestamp,
  );
  const attrs = result.attrs!;
  if (id(message.model)) attrs.model = message.model;
  counter(attrs, "uncached_input_tokens", usage.input_tokens);
  counter(attrs, "cache_read_input_tokens", usage.cache_read_input_tokens);
  counter(attrs, "cache_write_input_tokens", usage.cache_creation_input_tokens);
  summedInput(attrs);
  // Assistant output_tokens can be a message_start placeholder. Do not report it.
  return result;
}

export function codexUsageEvent(
  rowValue: unknown,
  session: string,
): ObserveBatchEvent | null {
  const row = object(rowValue),
    payload = object(row?.payload),
    usage = object(payload?.usage);
  if (
    !id(session) ||
    row?.type !== "token_usage_record" ||
    payload?.thread_id !== session ||
    !id(payload.response_id) ||
    !usage
  )
    return null;
  const result = event(
    "codex",
    "codex.token_usage_record.v1",
    session,
    payload.response_id,
    row.timestamp,
  );
  const attrs = result.attrs!;
  for (const key of [
    "input_tokens",
    "output_tokens",
    "reasoning_output_tokens",
    "total_tokens",
  ])
    counter(attrs, key, usage[key]);
  counter(attrs, "cache_read_input_tokens", usage.cached_input_tokens);
  counter(attrs, "cache_write_input_tokens", usage.cache_write_input_tokens);
  if (
    [
      "input_tokens",
      "cache_read_input_tokens",
      "cache_write_input_tokens",
    ].every((key) => attrs[key] !== undefined)
  ) {
    const uncached =
      Number(attrs.input_tokens) -
      Number(attrs.cache_read_input_tokens) -
      Number(attrs.cache_write_input_tokens);
    if (count(uncached)) attrs.uncached_input_tokens = String(uncached);
    else attrs.usage_invalid = "1";
  }
  // thread_token_usage is cumulative and must never be added to these counters.
  return result;
}

export function openCodeUsageEvent(
  partValue: unknown,
): ObserveBatchEvent | null {
  const part = object(partValue),
    tokens = object(part?.tokens),
    cache = object(tokens?.cache);
  if (
    part?.type !== "step-finish" ||
    !id(part.sessionID) ||
    !id(part.id) ||
    !id(part.messageID) ||
    !tokens
  )
    return null;
  const result = event(
    "opencode",
    "opencode.step_finish.v1",
    part.sessionID,
    part.id,
  );
  const attrs = result.attrs!;
  counter(attrs, "uncached_input_tokens", tokens.input);
  counter(attrs, "cache_read_input_tokens", cache?.read);
  counter(attrs, "cache_write_input_tokens", cache?.write);
  counter(attrs, "reported_output_tokens", tokens.output);
  counter(attrs, "reasoning_output_tokens", tokens.reasoning);
  summedInput(attrs);
  // Output/reasoning overlap changed across OpenCode versions. Keep raw output separate.
  return result;
}

export interface UsageCollection {
  events: ObserveBatchEvent[];
  status: "partial" | "unavailable";
  limited: boolean;
  malformed: number;
}

/** Read only the explicitly supplied regular file, retaining a bounded tail.
 * No cursor: later Stop hooks can retry failed writes and delayed transcript flushes.
 * Even a complete file scan cannot establish complete provider or subagent coverage.
 */
export function collectTranscriptUsage(
  path: string,
  host: "claude-code" | "codex",
  session: string,
  limits: { maxBytes?: number; maxEvents?: number } = {},
): UsageCollection {
  const result: UsageCollection = {
    events: [],
    status: "unavailable",
    limited: false,
    malformed: 0,
  };
  if (!isAbsolute(path) || !id(session)) return result;
  const maxBytes = Math.min(
    4 * 1024 * 1024,
    Math.max(1, Math.floor(limits.maxBytes ?? 4 * 1024 * 1024)),
  );
  const maxEvents = Math.min(
    1000,
    Math.max(1, Math.floor(limits.maxEvents ?? 1000)),
  );
  let fd: number | undefined;
  try {
    if (!statSync(path).isFile()) return result;
    fd = openSync(path, constants.O_RDONLY | (constants.O_NONBLOCK ?? 0));
    const stat = fstatSync(fd);
    if (!stat.isFile()) return result;
    const start = Math.max(0, stat.size - maxBytes);
    const buffer = Buffer.alloc(Math.min(stat.size, maxBytes));
    const bytes = readSync(fd, buffer, 0, buffer.length, start);
    let text = buffer.subarray(0, bytes).toString("utf8");
    result.limited = start > 0 || bytes !== buffer.length;
    if (start > 0) {
      const previous = Buffer.alloc(1);
      readSync(fd, previous, 0, 1, start - 1);
      if (previous[0] !== 10) {
        const boundary = text.indexOf("\n");
        text = boundary < 0 ? "" : text.slice(boundary + 1);
      }
    }
    const end = text.lastIndexOf("\n");
    if (end !== text.length - 1) result.limited = true;
    text = end < 0 ? "" : text.slice(0, end);
    const lines = text.split("\n");
    result.status = "partial";
    for (let i = lines.length - 1; i >= 0; i--) {
      if (!lines[i].trim()) continue;
      try {
        const row: unknown = JSON.parse(lines[i]);
        const next =
          host === "codex"
            ? codexUsageEvent(row, session)
            : claudeUsageEvent(row, session);
        if (next) {
          if (result.events.length === maxEvents) {
            result.limited = true;
            break;
          }
          result.events.push(next);
        }
      } catch {
        result.malformed++;
      }
    }
    result.events.reverse();
  } catch {
    /* Missing, unreadable or unsupported source stays unavailable. */
  } finally {
    if (fd !== undefined) closeSync(fd);
  }
  return result;
}

/** Acknowledged batch only: older CLI fallback cannot preserve source timestamps. */
export function recordModelUsage(
  binary: string,
  cwd: string,
  events: ObserveBatchEvent[],
): boolean {
  if (!events.length) return true;
  try {
    const output = execFileSync(binary, ["observe", "record", "--stdin"], {
      cwd,
      input: events.map((e) => JSON.stringify(e)).join("\n") + "\n",
      timeout: 10000,
      stdio: ["pipe", "pipe", "pipe"],
    }).toString();
    const ack = /^Recorded (\d+) event\(s\)(?:, skipped (\d+))?\s*$/i.exec(
      output.trim(),
    );
    return (
      ack !== null &&
      Number(ack[1]) === events.length &&
      Number(ack[2] ?? 0) === 0
    );
  } catch (error) {
    debug(
      "model-usage",
      `Usage batch was not acknowledged: ${error instanceof Error ? error.name : "error"}`,
    );
    return false;
  }
}

/** Keep repeated part broadcasts cheap; durable dedup/conflict handling is in Go. */
export function createOpenCodeUsageRecorder(
  write = recordModelUsage,
): (binary: string, cwd: string, part: unknown) => void {
  const acknowledged = new Set<string>();
  return (binary, cwd, part) => {
    const next = openCodeUsageEvent(part);
    if (!next) return;
    const fingerprint = JSON.stringify([cwd, next]);
    if (acknowledged.has(fingerprint) || !write(binary, cwd, [next])) return;
    acknowledged.add(fingerprint);
    if (acknowledged.size > 1024)
      acknowledged.delete(acknowledged.values().next().value!);
  };
}
