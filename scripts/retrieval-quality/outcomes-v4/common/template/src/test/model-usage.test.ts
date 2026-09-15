import { describe, expect, it } from "vitest";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  claudeUsageEvent,
  codexUsageEvent,
  openCodeUsageEvent,
  collectTranscriptUsage,
  createOpenCodeUsageRecorder,
} from "../core/model-usage.js";

const claude = (usage: unknown) => ({
  type: "assistant",
  sessionId: "s",
  timestamp: "2026-09-08T12:00:00Z",
  message: { id: "msg-1", model: "m", usage },
});
describe("host model usage observations", () => {
  it.each([
    "2026-02-30T12:00:00Z",
    "2026-09-08T12:00:00",
    "2026-09-08T24:00:00Z",
    "2026-09-08T12:00:00+24:00",
  ])("rejects malformed explicit source time %s", (timestamp) => {
    const observed = claudeUsageEvent(
      { ...claude({ input_tokens: 1 }), timestamp },
      "s",
    )!;
    expect(observed.attrs?.usage_invalid).toBe("1");
    expect(observed.attrs?.usage_source_time).toBeUndefined();
    expect(observed.ts).toBeUndefined();
  });
  it("preserves valid offsets and submillisecond source precision", () => {
    const timestamp = "2024-02-29T23:59:59.123456789+05:30";
    const observed = claudeUsageEvent(
      { ...claude({ input_tokens: 1 }), timestamp },
      "s",
    )!;
    expect(observed.ts).toBe(timestamp);
    expect(observed.attrs?.usage_source_time).toBe(timestamp);
    expect(observed.attrs?.usage_invalid).toBeUndefined();
  });
  it("retries failed OpenCode writes and retains conflicting revisions", () => {
    const batches: unknown[] = [];
    const record = createOpenCodeUsageRecorder((_binary, _cwd, events) => {
      batches.push(events);
      return batches.length !== 1;
    });
    const part = {
      id: "p",
      sessionID: "s",
      messageID: "m",
      type: "step-finish",
      tokens: { input: 1 },
    };
    record("bin", "/cwd", part); // failed
    record("bin", "/cwd", part); // retry succeeds
    record("bin", "/cwd", part); // same snapshot skipped
    record("bin", "/cwd", { ...part, tokens: { input: 2 } }); // conflict reaches store
    expect(batches).toHaveLength(3);
  });
  it("uses Codex per-response counters and ignores cumulative records", () => {
    const row = {
      type: "token_usage_record",
      payload: {
        thread_id: "s",
        response_id: "resp",
        usage: {
          input_tokens: 12,
          output_tokens: 8,
          reasoning_output_tokens: 3,
          total_tokens: 20,
          cached_input_tokens: 5,
        },
        thread_token_usage: { input_tokens: 999 },
      },
    };
    expect(codexUsageEvent(row, "s")?.attrs).toMatchObject({
      input_tokens: "12",
      output_tokens: "8",
      reasoning_output_tokens: "3",
      total_tokens: "20",
      cache_read_input_tokens: "5",
    });
    expect(codexUsageEvent(row, "other")).toBeNull();
    expect(codexUsageEvent({ ...row, type: "event_msg" }, "s")).toBeNull();
  });
  it("normalizes Claude disjoint inputs but omits placeholder output", () => {
    const e = claudeUsageEvent(
      claude({
        input_tokens: 10,
        cache_read_input_tokens: 20,
        cache_creation_input_tokens: 0,
        output_tokens: 1,
      }),
      "s",
    )!;
    expect(e.attrs).toMatchObject({
      input_tokens: "30",
      uncached_input_tokens: "10",
      cache_write_input_tokens: "0",
      usage_id: "msg-1",
      usage_time_basis: "source",
    });
    expect(e.attrs).not.toHaveProperty("output_tokens");
    expect(e.ts).toBe("2026-09-08T12:00:00Z");
  });
  it("does not substitute missing or invalid fields with zero", () => {
    const e = claudeUsageEvent(
      claude({
        input_tokens: 10,
        cache_read_input_tokens: -1,
        output_tokens: 0,
      }),
      "s",
    )!;
    expect(e.attrs).not.toHaveProperty("input_tokens");
    expect(e.attrs).not.toHaveProperty("cache_read_input_tokens");
    expect(e.attrs).not.toHaveProperty("cache_write_input_tokens");
    expect(e.attrs?.usage_invalid).toBe("1");
  });
  it("requires matching explicit session and stable response identity", () => {
    expect(claudeUsageEvent(claude({ input_tokens: 1 }), "other")).toBeNull();
    expect(
      claudeUsageEvent(
        { ...claude({ input_tokens: 1 }), message: { usage: {} } },
        "s",
      ),
    ).toBeNull();
    expect(claudeUsageEvent({ ...claude({}), type: "user" }, "s")).toBeNull();
  });
  it("records OpenCode per step with ambiguous output separate", () => {
    const e = openCodeUsageEvent({
      id: "part",
      sessionID: "session",
      messageID: "message",
      type: "step-finish",
      tokens: {
        input: 3,
        output: 6,
        reasoning: 2,
        cache: { read: 4, write: 0 },
        total: 15,
      },
    })!;
    expect(e.session).toBe("session");
    expect(e.attrs).toMatchObject({
      usage_id: "part",
      input_tokens: "7",
      reported_output_tokens: "6",
      reasoning_output_tokens: "2",
      usage_time_basis: "observed",
    });
    expect(e.attrs).not.toHaveProperty("output_tokens");
    expect(e.attrs).not.toHaveProperty("total_tokens");
    expect(
      openCodeUsageEvent({ type: "text", id: "p", sessionID: "s" }),
    ).toBeNull();
  });
  it("omits unsafe sums and invalid source timestamps", () => {
    const e = claudeUsageEvent(
      {
        ...claude({
          input_tokens: Number.MAX_SAFE_INTEGER,
          cache_read_input_tokens: 1,
          cache_creation_input_tokens: 0,
        }),
        timestamp: "bad",
      },
      "s",
    )!;
    expect(e.attrs).not.toHaveProperty("input_tokens");
    expect(e.ts).toBeUndefined();
    expect(e.attrs?.usage_time_basis).toBe("observed");
    expect(e.attrs?.usage_invalid).toBe("1");
  });
  it("bounds explicit transcript scanning and reports malformed records", () => {
    const dir = mkdtempSync(join(tmpdir(), "aide-usage-"));
    try {
      const path = join(dir, "transcript.jsonl");
      const line = JSON.stringify(claude({ input_tokens: 1 }));
      writeFileSync(path, line + "\nnot-json\n" + line + "\n");
      const all = collectTranscriptUsage(path, "claude-code", "s");
      expect(all.events).toHaveLength(2); // preserve revisions; store deduplicates identities
      expect(all.malformed).toBe(1);
      expect(all.status).toBe("partial");
      const limited = collectTranscriptUsage(path, "claude-code", "s", {
        maxBytes: Buffer.byteLength(line) + 1,
      });
      expect(limited.events).toHaveLength(1);
      expect(limited.limited).toBe(true);
      expect(limited.status).toBe("partial");
      expect(collectTranscriptUsage(dir, "claude-code", "s").status).toBe(
        "unavailable",
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
