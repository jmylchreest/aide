import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { ContextPruningTracker } from "../core/context-pruning/tracker.js";
import { recoverablePrune } from "../core/context-pruning/recovery.js";
import { replacementTarget } from "../core/context-pruning/replacement.js";
import { processPruningHook } from "../core/context-pruning/claude.js";
import { transformationEvent } from "../core/context-pruning/observation.js";

let cwd: string;
beforeEach(() => {
  cwd = mkdtempSync(join(tmpdir(), "aide-prune-recovery-"));
});
afterEach(() => rmSync(cwd, { recursive: true, force: true }));

describe("recoverable output transformations", () => {
  it("keeps the final error diagnosis as well as the opening context", () => {
    const tracker = new ContextPruningTracker(cwd);
    const original =
      "Traceback (most recent call last):\n" +
      "a long stack frame at module.py:40\n".repeat(100) +
      "TypeError: actual cause at the end";
    const result = tracker.process(
      "error",
      "Bash",
      {},
      original,
      recoverablePrune(cwd, "scope", "error", original),
    );
    expect(result.modified).toBe(true);
    expect(result.output).toContain("Traceback");
    expect(result.output).toContain("TypeError: actual cause at the end");
    expect(result.output).not.toContain("Re-run");
  });
  it("uses tool_response and real invocation identity without crossing context epochs", () => {
    const identity = { host: "claude-code", sessionId: "s", actorId: "a" };
    const window = {
      version: 1 as const,
      id: "epoch",
      status: "active" as const,
      continuity: "new" as const,
      reason: "startup",
    };
    const input = {
      tool_name: "mcp__aide__code_outline",
      tool_response: "é result\n".repeat(500),
      tool_input: { file: "a.ts" },
      tool_use_id: "first",
    };
    expect(processPruningHook(cwd, identity, window, input)).toBeNull();
    const result = processPruningHook(cwd, identity, window, {
      ...input,
      tool_use_id: "second",
    })!;
    expect(result.event?.attrs?.observation_stage).toBe("rewrite_candidate");
    expect(result.event?.attrs?.invocation_id).toBe("second");
    expect(result.event?.attrs?.before_bytes).toBe(
      String(Buffer.byteLength(input.tool_response)),
    );
    expect(result.event?.attrs?.after_bytes).toBe(
      String(Buffer.byteLength(result.replacement as string)),
    );
    expect(
      processPruningHook(cwd, identity, window, {
        ...input,
        tool_use_id: "second",
      }),
    ).toBeNull();
    expect(
      processPruningHook(
        cwd,
        identity,
        { ...window, id: "new" },
        { ...input, tool_use_id: "third" },
      ),
    ).toBeNull();
    expect(
      processPruningHook(
        cwd,
        identity,
        { ...window, status: "pending" },
        { ...input, tool_use_id: "fourth" },
      ),
    ).toBeNull();
    expect(
      processPruningHook(cwd, identity, window, {
        ...input,
        tool_use_id: undefined,
      }),
    ).toBeNull();
  });
  it("retains exact UTF-8 text and measures the replacement including its recovery pointer", () => {
    const tracker = new ContextPruningTracker(cwd);
    const original = "é😀 output\n".repeat(500);
    tracker.process("one", "Grep", {}, original);
    const result = tracker.process(
      "two",
      "Grep",
      {},
      original,
      recoverablePrune(cwd, "scope", "two", original),
    );
    expect(result.modified).toBe(true);
    expect(readFileSync(result.recoveryPath!, "utf8")).toBe(original);
    expect(result.output).toContain(result.recoveryPath);
    expect(result.bytesSaved).toBe(
      Buffer.byteLength(original) - Buffer.byteLength(result.output),
    );
    expect(tracker.getStats().totalBytesSaved).toBe(result.bytesSaved);
    expect(tracker.getStats().estimatedContextBytes).toBe(
      Buffer.byteLength(original) + Buffer.byteLength(result.output),
    );
  });
  it("keeps full output if originals cannot be retained", () => {
    writeFileSync(join(cwd, ".aide"), "not a directory");
    const tracker = new ContextPruningTracker(cwd);
    const original = "content\n".repeat(500);
    tracker.process("one", "Grep", {}, original);
    const result = tracker.process(
      "two",
      "Grep",
      {},
      original,
      recoverablePrune(cwd, "scope", "two", original),
    );
    expect(result.modified).toBe(false);
    expect(result.output).toBe(original);
    expect(tracker.getHistory().at(-1)?.prunedOutput).toBeNull();
  });
  it("includes added bytes as negative adapter evidence without claiming final delivery", () => {
    const event = transformationEvent(
      { host: "opencode", sessionId: "s", actorId: "s" },
      null,
      "call",
      "Edit",
      "ok",
      "ok\nnotice",
      "adapter_change",
    );
    expect(event?.attrs?.before_bytes).toBe("2");
    expect(event?.attrs?.after_bytes).toBe("9");
    expect(event?.attrs?.context_status).toBe("unknown");
    expect(event?.saved).toBeUndefined();
    expect(event?.tokens).toBeUndefined();
  });
  it("preserves signed annotation overhead and restores UTF-8 stats from history", () => {
    const tracker = new ContextPruningTracker(cwd);
    tracker.process("read", "Read", { file_path: "é.ts" }, "é");
    const r = tracker.process("edit", "Edit", { file_path: "é.ts" }, "ok");
    expect(r.bytesSaved).toBe(2 - Buffer.byteLength(r.output));
    const restored = new ContextPruningTracker(cwd);
    restored.loadHistory(tracker.getHistory());
    expect(restored.getStats()).toEqual(tracker.getStats());
  });
  it("preserves Bash channels and MCP block metadata, refusing opaque or unsupported payloads", () => {
    const bash = {
      stdout: "long output",
      stderr: "warning",
      interrupted: false,
      isImage: false,
    };
    const target = replacementTarget("Bash", bash)!;
    expect(target.replace("short")).toEqual({ ...bash, stdout: "short" });
    const mcp = {
      content: [{ type: "text", text: "abc", annotations: { priority: 1 } }],
      isError: false,
    };
    expect(
      replacementTarget("mcp__aide__code_outline", mcp)?.replace("short"),
    ).toEqual({ ...mcp, content: [{ ...mcp.content[0], text: "short" }] });
    expect(
      replacementTarget("mcp__aide__code_outline", {
        content: [...mcp.content, { type: "image", data: "opaque" }],
      }),
    ).toBeNull();
    expect(
      replacementTarget("Read", { file: { content: "source" } }),
    ).toBeNull();
  });
});
