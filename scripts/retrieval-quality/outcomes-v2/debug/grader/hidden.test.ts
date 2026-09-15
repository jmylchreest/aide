import { test, expect, afterAll } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync, utimesSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
const trial = process.env.AIDE_TRIAL_ROOT;
if (!trial) throw new Error("AIDE_TRIAL_ROOT is required");
const { ContextPruningTracker } = await import(join(trial, "src/core/context-pruning/tracker.ts"));
const { recoverablePrune } = await import(join(trial, "src/core/context-pruning/recovery.ts"));
const root = mkdtempSync(join(tmpdir(), "aide-debug-hidden-"));
afterAll(() => rmSync(root, { recursive: true, force: true }));
const output = "src/module.ts:78: export const café = '🧭';\n".repeat(220);
const args = { query: "café", language: "typescript" };
function run(tracker: InstanceType<typeof ContextPruningTracker>, call: string, text = output) {
  return tracker.process(call, "mcp__aide__code_search", args, text, recoverablePrune(root, "hidden", call, text));
}

test("repeat pointers skip shortened calls and target retained full output", () => {
  const tracker = new ContextPruningTracker(root);
  expect(run(tracker, "full").modified).toBe(false);
  for (const call of ["duplicate-1", "duplicate-2", "duplicate-3"]) {
    const result = run(tracker, call);
    expect(result.modified).toBe(true);
    expect(result.output).toContain("callId: full");
  }
});

test("history records original and actually emitted shortened outputs separately", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "history-full");
  const second = run(tracker, "history-short");
  const history = tracker.getHistory();
  expect(history[0].originalOutput).toBe(output);
  expect(history[0].prunedOutput).toBeNull();
  expect(history[1].originalOutput).toBe(output);
  expect(history[1].prunedOutput).toBe(second.output);
  expect(history[1].prunedOutput).toContain("[aide:original]");
});

test("evicted full baseline forces a new full result before dedup resumes", () => {
  const tracker = new ContextPruningTracker(root, 2);
  run(tracker, "evict-1");
  run(tracker, "evict-2");
  expect(run(tracker, "evict-3").output).toContain("callId: evict-1");
  const fourth = run(tracker, "evict-4");
  expect(fourth.modified).toBe(false);
  expect(fourth.output).toBe(output);
  expect(run(tracker, "evict-5").output).toContain("callId: evict-4");
});

test("reloading history preserves full baseline choice and byte statistics", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "reload-full");
  run(tracker, "reload-short");
  const restored = new ContextPruningTracker(root);
  restored.loadHistory(tracker.getHistory());
  expect(restored.getStats()).toEqual(tracker.getStats());
  expect(run(restored, "reload-next").output).toContain("callId: reload-full");
});

test("reloading a history window containing only shortened outputs starts fresh", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "window-full");
  run(tracker, "window-short");
  const restored = new ContextPruningTracker(root, 1);
  restored.loadHistory(tracker.getHistory());
  expect(run(restored, "window-next").modified).toBe(false);
  expect(restored.getHistory()[0].originalOutput).toBe(output);
});

test("recovery veto leaves full emitted output as a usable baseline", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "veto-first");
  const veto = tracker.process("veto-second", "mcp__aide__code_search", args, output, recoverablePrune(root, "", "veto-second", output));
  expect(veto).toEqual({ output, modified: false, bytesSaved: 0 });
  expect(tracker.getHistory()[1].prunedOutput).toBeNull();
  expect(tracker.getStats()).toEqual({ totalCalls: 2, prunedCalls: 0, totalBytesSaved: 0, estimatedContextBytes: Buffer.byteLength(output) * 2 });
  expect(run(tracker, "veto-third").output).toContain("callId: veto-second");
});

test("different arguments and different outputs are not deduplicated", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "distinct-first");
  expect(tracker.process("distinct-args", "mcp__aide__code_search", { ...args, query: "other" }, output).modified).toBe(false);
  expect(run(tracker, "distinct-output", output + "changed").modified).toBe(false);
  expect(run(tracker, "distinct-repeat", output + "changed").output).toContain("callId: distinct-output");
});

test("side effecting tools never acquire dedup pointers", () => {
  for (const tool of ["Bash", "Write", "Edit"]) {
    const tracker = new ContextPruningTracker(root);
    tracker.process("unsafe-1", tool, args, output);
    expect(tracker.process("unsafe-2", tool, args, output)).toEqual({ output, modified: false, bytesSaved: 0 });
  }
});

test("retained originals match original bytes and survive further duplicate calls", () => {
  const tracker = new ContextPruningTracker(root);
  run(tracker, "retain-first");
  const second = run(tracker, "retain-second");
  expect(typeof second.recoveryPath).toBe("string");
  expect(readFileSync(second.recoveryPath!, "utf8")).toBe(output);
  run(tracker, "retain-third");
  expect(readFileSync(second.recoveryPath!, "utf8")).toBe(output);
});

test("live accounting measures final emitted UTF-8 bytes", () => {
  const tracker = new ContextPruningTracker(root);
  const results = [run(tracker, "bytes-1"), run(tracker, "bytes-2"), run(tracker, "bytes-3")];
  for (const result of results) expect(result.bytesSaved).toBe(Buffer.byteLength(output) - Buffer.byteLength(result.output));
  expect(tracker.getStats()).toEqual({ totalCalls: 3, prunedCalls: 2, totalBytesSaved: results.reduce((n, r) => n + r.bytesSaved, 0), estimatedContextBytes: results.reduce((n, r) => n + Buffer.byteLength(r.output), 0) });
});

test("changed Read file mtime prevents dedup even for identical text", () => {
  const path = join(root, "mtime.txt");
  writeFileSync(path, output);
  utimesSync(path, 1000000000, 1000000000);
  const tracker = new ContextPruningTracker(root);
  tracker.process("read-first", "Read", { file_path: path }, output);
  utimesSync(path, 1000000010, 1000000010);
  expect(tracker.process("read-second", "Read", { file_path: path }, output).modified).toBe(false);
});
