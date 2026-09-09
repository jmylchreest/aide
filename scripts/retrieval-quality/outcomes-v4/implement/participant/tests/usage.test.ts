/** EXPERIMENT VISIBLE REGRESSION TESTS. Run: bun test tests */
import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { claudeUsageEvent, codexUsageEvent, openCodeUsageEvent,
  collectTranscriptUsage } from "../src/core/model-usage.ts";

describe("existing usage behavior", () => {
  test("Claude inputs are disjoint; placeholder output is omitted", () => {
    const result = claudeUsageEvent({ type: "assistant", sessionId: "s",
      timestamp: "2024-02-29T23:59:59.123456789+05:30",
      message: { id: "m", model: "model", usage: { input_tokens: 2,
        cache_read_input_tokens: 3, cache_creation_input_tokens: 0, output_tokens: 99 } } }, "s")!;
    expect(result.attrs).toMatchObject({ input_tokens: "5", uncached_input_tokens: "2", usage_coverage: "partial" });
    expect(result.attrs?.output_tokens).toBeUndefined();
    expect(result.ts).toBe("2024-02-29T23:59:59.123456789+05:30");
  });
  test("Codex rejects cumulative records and mismatched sessions", () => {
    const row = { type: "token_usage_record", payload: { thread_id: "s", response_id: "r",
      usage: { input_tokens: 10 }, thread_token_usage: { input_tokens: 1000 } } };
    expect(codexUsageEvent(row, "s")?.attrs?.input_tokens).toBe("10");
    expect(codexUsageEvent(row, "other")).toBeNull();
    expect(codexUsageEvent({ ...row, type: "event_msg" }, "s")).toBeNull();
  });
  test("OpenCode retains ambiguous raw output without adding it", () => {
    const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m",
      tokens: { input: 2, output: 9, reasoning: 4, cache: { read: 3, write: 1 } } };
    expect(openCodeUsageEvent(part)?.attrs).toMatchObject({ input_tokens: "6", reported_output_tokens: "9", reasoning_output_tokens: "4" });
    expect(openCodeUsageEvent(part)?.attrs?.output_tokens).toBeUndefined();
    expect(openCodeUsageEvent({ ...part, type: "text" })).toBeNull();
  });
  test("transcript scanner skips incomplete trailing lines and stays partial", () => {
    const dir = mkdtempSync(join(tmpdir(), "usage-visible-"));
    try {
      const path = join(dir, "transcript.jsonl");
      writeFileSync(path, JSON.stringify({ type: "assistant", sessionId: "s", message: { id: "m", usage: { input_tokens: 1 } } }) + '\n{"unfinished":');
      const result = collectTranscriptUsage(path, "claude-code", "s");
      expect(result.events).toHaveLength(1);
      expect(result.status).toBe("partial"); expect(result.limited).toBe(true);
      expect(collectTranscriptUsage("relative", "claude-code", "s").status).toBe("unavailable");
    } finally { rmSync(dir, { recursive: true, force: true }); }
  });
});
