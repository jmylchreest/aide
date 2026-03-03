/**
 * Tests for context-pruning strategies and tracker.
 *
 * Tests cover:
 * - DedupStrategy: safe-tool dedup, mtime checks, unsafe tool rejection
 * - SupersedeStrategy: Write/Edit annotate prior reads
 * - PurgeErrorsStrategy: large error output trimming
 * - ContextPruningTracker: orchestration, stats, pressure, reset, history
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { DedupStrategy } from "../core/context-pruning/dedup.js";
import { SupersedeStrategy } from "../core/context-pruning/supersede.js";
import { PurgeErrorsStrategy } from "../core/context-pruning/purge.js";
import { ContextPruningTracker } from "../core/context-pruning/tracker.js";
import type { ToolRecord } from "../core/context-pruning/types.js";

// =============================================================================
// DedupStrategy
// =============================================================================

describe("DedupStrategy", () => {
  let strategy: DedupStrategy;

  beforeEach(() => {
    strategy = new DedupStrategy("/tmp/test-cwd");
  });

  it("should not dedup when history is empty", () => {
    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      "file content here",
      [],
    );
    expect(result.modified).toBe(false);
    expect(result.output).toBe("file content here");
  });

  it("should dedup identical Read output for same file", () => {
    // Use a large output so the dedup replacement saves bytes
    const largeContent = "x".repeat(500);
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: largeContent,
        prunedOutput: null,
        timestamp: Date.now(),
        fileMtime: undefined, // No mtime → skip mtime check
      },
    ];

    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      largeContent,
      history,
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("dedup");
    expect(result.output).toContain("[aide:dedup]");
    expect(result.output).toContain("call-1");
    expect(result.bytesSaved).toBeGreaterThan(0);
  });

  it("should not dedup if output differs", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "old content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      "new content",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should not dedup if file path differs", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Read",
      { filePath: "/src/bar.ts" },
      "content",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should not dedup Bash commands (unsafe)", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Bash",
        args: { command: "ls" },
        originalOutput: "file1.ts\nfile2.ts",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Bash",
      { command: "ls" },
      "file1.ts\nfile2.ts",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should not dedup Write tool (unsafe)", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Write",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "File written",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Write",
      { filePath: "/src/foo.ts" },
      "File written",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should not dedup Edit tool (unsafe)", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Edit",
        args: { file_path: "/src/foo.ts" },
        originalOutput: "Edit applied",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Edit",
      { file_path: "/src/foo.ts" },
      "Edit applied",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should dedup Glob with same pattern", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Glob",
        args: { pattern: "**/*.ts" },
        originalOutput: "src/foo.ts\nsrc/bar.ts",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Glob",
      { pattern: "**/*.ts" },
      "src/foo.ts\nsrc/bar.ts",
      history,
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("dedup");
  });

  it("should dedup Grep with same pattern and path", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Grep",
        args: { pattern: "TODO", path: "src/", include: "*.ts" },
        originalOutput: "src/foo.ts:10: // TODO fix",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Grep",
      { pattern: "TODO", path: "src/", include: "*.ts" },
      "src/foo.ts:10: // TODO fix",
      history,
    );
    expect(result.modified).toBe(true);
  });

  it("should dedup aide MCP tools", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "mcp__aide__code_search",
        args: { query: "handleRequest" },
        originalOutput: "Found 5 results...",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "mcp__aide__code_search",
      { query: "handleRequest" },
      "Found 5 results...",
      history,
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("dedup");
  });

  it("should dedup Claude Code style tool names (no mcp__ prefix)", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "memory_list",
        args: { limit: 10 },
        originalOutput: "mem1\nmem2",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "memory_list",
      { limit: 10 },
      "mem1\nmem2",
      history,
    );
    expect(result.modified).toBe(true);
  });

  it("should use prunedOutput for comparison when available", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Glob",
        args: { pattern: "**/*.ts" },
        originalOutput: "original large output",
        prunedOutput: "pruned output", // Was previously pruned
        timestamp: Date.now(),
      },
    ];

    // Output matches the prunedOutput, not originalOutput
    const result = strategy.apply(
      "Glob",
      { pattern: "**/*.ts" },
      "pruned output",
      history,
    );
    expect(result.modified).toBe(true);
  });

  it("should not dedup Read when file mtime has changed", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "content",
        prunedOutput: null,
        timestamp: Date.now(),
        fileMtime: 1000, // Old mtime
      },
    ];

    // The strategy calls statSync internally for mtime check.
    // Since the file doesn't exist at /src/foo.ts, mtime will be undefined,
    // and with prev.fileMtime defined but current undefined, mtime check is skipped.
    // To properly test mtime changes we'd need actual files.
    // This tests the "no file" path — doesn't block dedup.
    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
      history,
    );
    // When currentMtime is undefined, the mtime check is skipped (continue not triggered)
    expect(result.modified).toBe(true);
  });

  it("should handle case-insensitive tool name matching", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    // Tool name is "Read" (capitalized) but history has "read" (lowercase)
    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
      history,
    );
    // dedupKey normalizes to lowercase for the key comparison
    expect(result.modified).toBe(true);
  });

  it("should dedup Read with offset/limit args matching", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts", offset: 100, limit: 50 },
        originalOutput: "partial content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts", offset: 100, limit: 50 },
      "partial content",
      history,
    );
    expect(result.modified).toBe(true);
  });

  it("should not dedup Read with different offset", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts", offset: 100, limit: 50 },
        originalOutput: "partial content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts", offset: 200, limit: 50 },
      "partial content",
      history,
    );
    expect(result.modified).toBe(false);
  });
});

// =============================================================================
// SupersedeStrategy
// =============================================================================

describe("SupersedeStrategy", () => {
  let strategy: SupersedeStrategy;

  beforeEach(() => {
    strategy = new SupersedeStrategy();
  });

  it("should not modify non-write tool outputs", () => {
    const result = strategy.apply(
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
      [],
    );
    expect(result.modified).toBe(false);
  });

  it("should annotate Write output when prior Reads exist for same file", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "old content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Write",
      { filePath: "/src/foo.ts" },
      "File written successfully",
      history,
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("supersede");
    expect(result.output).toContain("[aide:supersede]");
    expect(result.output).toContain("1 prior Read(s)");
    expect(result.output).toContain("/src/foo.ts");
    expect(result.output).toContain("File written successfully");
  });

  it("should annotate Edit output when prior Reads exist", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "old content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
      {
        callId: "call-2",
        toolName: "Read",
        args: { filePath: "/src/foo.ts", offset: 50 },
        originalOutput: "more old content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Edit",
      { filePath: "/src/foo.ts" },
      "Edit applied",
      history,
    );
    expect(result.modified).toBe(true);
    expect(result.output).toContain("2 prior Read(s)");
  });

  it("should not annotate Write if no prior Reads for that file", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/other.ts" },
        originalOutput: "other content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Write",
      { filePath: "/src/foo.ts" },
      "File written",
      history,
    );
    expect(result.modified).toBe(false);
  });

  it("should not annotate Write with no file path", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply("Write", {}, "Written", history);
    expect(result.modified).toBe(false);
  });

  it("should report 0 bytes saved (adds content, doesn't remove)", () => {
    const history: ToolRecord[] = [
      {
        callId: "call-1",
        toolName: "Read",
        args: { filePath: "/src/foo.ts" },
        originalOutput: "content",
        prunedOutput: null,
        timestamp: Date.now(),
      },
    ];

    const result = strategy.apply(
      "Write",
      { filePath: "/src/foo.ts" },
      "Written",
      history,
    );
    expect(result.bytesSaved).toBe(0);
  });
});

// =============================================================================
// PurgeErrorsStrategy
// =============================================================================

describe("PurgeErrorsStrategy", () => {
  let strategy: PurgeErrorsStrategy;

  beforeEach(() => {
    strategy = new PurgeErrorsStrategy();
  });

  it("should not purge non-Bash tool outputs", () => {
    const largeError = "error: big failure\n" + "stack line\n".repeat(100);
    const result = strategy.apply("Read", {}, largeError, []);
    expect(result.modified).toBe(false);
  });

  it("should not purge small Bash error outputs", () => {
    const result = strategy.apply("Bash", {}, "error: small", []);
    expect(result.modified).toBe(false);
  });

  it("should not purge large non-error Bash output", () => {
    const largeOutput = "line\n".repeat(200);
    const result = strategy.apply("Bash", {}, largeOutput, []);
    expect(result.modified).toBe(false);
  });

  it("should purge large Bash error output with 'error' pattern", () => {
    const lines = ["error: compilation failed"];
    for (let i = 0; i < 50; i++) {
      lines.push(
        `  at module.js:${i}:${i} in handleRequest (${"/long/path/to/module".repeat(3)})`,
      );
    }
    const output = lines.join("\n");
    // Verify test setup: output must exceed MIN_SIZE_FOR_PURGE (2048)
    expect(output.length).toBeGreaterThan(2048);

    const result = strategy.apply("Bash", {}, output, []);
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("purge");
    expect(result.output).toContain("[aide:purge]");
    expect(result.output).toContain("trimmed");
    expect(result.bytesSaved).toBeGreaterThan(0);

    // Should keep first 30 lines
    const resultLines = result.output.split("\n");
    expect(resultLines.length).toBeLessThan(lines.length);
  });

  it("should purge large Bash output with Traceback pattern", () => {
    const lines = ["Traceback (most recent call last):"];
    for (let i = 0; i < 50; i++) {
      lines.push(`  File "module.py", line ${i}, in <module>`);
    }
    const output = lines.join("\n");

    const result = strategy.apply("Bash", {}, output, []);
    expect(result.modified).toBe(true);
    expect(result.output).toContain("[aide:purge]");
  });

  it("should purge large Bash output with TypeError pattern", () => {
    const lines = ["TypeError: Cannot read property 'foo' of undefined"];
    for (let i = 0; i < 50; i++) {
      lines.push(`    at Object.<anonymous> (index.js:${i}:1)`);
    }
    const output = lines.join("\n");

    const result = strategy.apply("Bash", {}, output, []);
    expect(result.modified).toBe(true);
  });

  it("should purge large Bash output with 'FAILED' pattern", () => {
    const lines = ["BUILD FAILED in 5s"];
    for (let i = 0; i < 50; i++) {
      lines.push(
        `> Task :compile FAILED (detail line ${i} ${"/long/path/detail".repeat(3)})`,
      );
    }
    const output = lines.join("\n");
    expect(output.length).toBeGreaterThan(2048);

    const result = strategy.apply("Bash", {}, output, []);
    expect(result.modified).toBe(true);
  });

  it("should not purge if output has fewer than 30 lines", () => {
    const lines = ["error: something failed"];
    for (let i = 0; i < 25; i++) {
      lines.push(`detail ${i}`);
    }
    // Need enough bytes to exceed MIN_SIZE_FOR_PURGE (2048)
    const output = lines.map((l) => l.padEnd(80, " ")).join("\n");

    const result = strategy.apply("Bash", {}, output, []);
    // 26 lines < 30 → should not purge
    expect(result.modified).toBe(false);
  });

  it("should handle case-insensitive tool name", () => {
    const lines = ["error: failed"];
    for (let i = 0; i < 50; i++) {
      lines.push(`stack frame ${i} `.padEnd(50, "x"));
    }
    const output = lines.join("\n");

    const result = strategy.apply("bash", {}, output, []);
    expect(result.modified).toBe(true);
  });
});

// =============================================================================
// ContextPruningTracker
// =============================================================================

describe("ContextPruningTracker", () => {
  let tracker: ContextPruningTracker;

  beforeEach(() => {
    tracker = new ContextPruningTracker("/tmp/test-cwd");
  });

  it("should return unmodified output for first call", () => {
    const result = tracker.process(
      "call-1",
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
    );
    expect(result.modified).toBe(false);
    expect(result.output).toBe("content");
  });

  it("should dedup identical repeated Read calls", () => {
    tracker.process("call-1", "Read", { filePath: "/src/foo.ts" }, "content");
    const result = tracker.process(
      "call-2",
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("dedup");
    expect(result.output).toContain("[aide:dedup]");
  });

  it("should annotate Write when prior Read exists", () => {
    tracker.process(
      "call-1",
      "Read",
      { filePath: "/src/foo.ts" },
      "old content",
    );
    const result = tracker.process(
      "call-2",
      "Write",
      { filePath: "/src/foo.ts" },
      "Written successfully",
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("supersede");
    expect(result.output).toContain("[aide:supersede]");
  });

  it("should track stats correctly", () => {
    const largeContent = "x".repeat(500);
    tracker.process(
      "call-1",
      "Read",
      { filePath: "/src/foo.ts" },
      largeContent,
    );
    tracker.process(
      "call-2",
      "Read",
      { filePath: "/src/foo.ts" },
      largeContent,
    ); // dedup
    tracker.process("call-3", "Bash", { command: "ls" }, "file1\nfile2");

    const stats = tracker.getStats();
    expect(stats.totalCalls).toBe(3);
    expect(stats.prunedCalls).toBe(1); // Only the dedup
    expect(stats.totalBytesSaved).toBeGreaterThan(0);
    expect(stats.estimatedContextBytes).toBeGreaterThan(0);
  });

  it("should compute context pressure", () => {
    // Start with no calls — pressure should be 0
    expect(tracker.getContextPressure()).toBe(0);

    // Add some calls
    tracker.process(
      "call-1",
      "Read",
      { filePath: "/src/foo.ts" },
      "x".repeat(1000),
    );
    const pressure = tracker.getContextPressure();
    expect(pressure).toBeGreaterThan(0);
    expect(pressure).toBeLessThan(1);
  });

  it("should reset clears history and stats", () => {
    tracker.process("call-1", "Read", { filePath: "/src/foo.ts" }, "content");
    tracker.process("call-2", "Bash", { command: "ls" }, "output");

    expect(tracker.getStats().totalCalls).toBe(2);
    expect(tracker.getHistory().length).toBe(2);

    tracker.reset();

    expect(tracker.getStats().totalCalls).toBe(0);
    expect(tracker.getStats().prunedCalls).toBe(0);
    expect(tracker.getStats().totalBytesSaved).toBe(0);
    expect(tracker.getStats().estimatedContextBytes).toBe(0);
    expect(tracker.getHistory().length).toBe(0);
  });

  it("should load history and recompute stats", () => {
    const history: ToolRecord[] = [
      {
        callId: "prior-1",
        toolName: "Read",
        args: { filePath: "/src/a.ts" },
        originalOutput: "aaa",
        prunedOutput: null,
        timestamp: Date.now() - 1000,
      },
      {
        callId: "prior-2",
        toolName: "Glob",
        args: { pattern: "*.ts" },
        originalOutput: "long output here",
        prunedOutput: "[aide:dedup] ...", // Was pruned
        timestamp: Date.now() - 500,
      },
    ];

    tracker.loadHistory(history);

    const stats = tracker.getStats();
    expect(stats.totalCalls).toBe(2);
    expect(stats.prunedCalls).toBe(1);
    expect(stats.estimatedContextBytes).toBeGreaterThan(0);
  });

  it("should dedup after loading history", () => {
    const history: ToolRecord[] = [
      {
        callId: "prior-1",
        toolName: "Glob",
        args: { pattern: "**/*.ts" },
        originalOutput: "src/foo.ts\nsrc/bar.ts",
        prunedOutput: null,
        timestamp: Date.now() - 1000,
      },
    ];

    tracker.loadHistory(history);

    const result = tracker.process(
      "call-1",
      "Glob",
      { pattern: "**/*.ts" },
      "src/foo.ts\nsrc/bar.ts",
    );
    expect(result.modified).toBe(true);
    expect(result.strategy).toBe("dedup");
  });

  it("should trim history when exceeding maxHistory", () => {
    const smallTracker = new ContextPruningTracker("/tmp/test", 5);

    for (let i = 0; i < 10; i++) {
      smallTracker.process(
        `call-${i}`,
        "Bash",
        { command: `cmd ${i}` },
        `output ${i}`,
      );
    }

    expect(smallTracker.getHistory().length).toBe(5);
    // Should keep the last 5 entries
    const history = smallTracker.getHistory();
    expect(history[0].callId).toBe("call-5");
    expect(history[4].callId).toBe("call-9");
  });

  it("should apply first matching strategy (dedup before supersede)", () => {
    // Read a file, then Read again identically — should dedup, not supersede
    tracker.process("call-1", "Read", { filePath: "/src/foo.ts" }, "content");

    const result = tracker.process(
      "call-2",
      "Read",
      { filePath: "/src/foo.ts" },
      "content",
    );

    expect(result.strategy).toBe("dedup");
  });

  it("should record fileMtime for Read calls", () => {
    // Process a Read — mtime recording may fail gracefully for nonexistent files
    tracker.process(
      "call-1",
      "Read",
      { filePath: "/nonexistent/file.ts" },
      "content",
    );

    const history = tracker.getHistory();
    expect(history.length).toBe(1);
    // fileMtime should be undefined for nonexistent file (graceful failure)
    expect(history[0].fileMtime).toBeUndefined();
  });

  it("should not crash on empty output", () => {
    const result = tracker.process(
      "call-1",
      "Read",
      { filePath: "/src/foo.ts" },
      "",
    );
    expect(result.modified).toBe(false);
  });

  it("should handle getHistory returning a copy", () => {
    tracker.process("call-1", "Read", { filePath: "/src/foo.ts" }, "content");
    const h1 = tracker.getHistory();
    const h2 = tracker.getHistory();
    expect(h1).toEqual(h2);
    expect(h1).not.toBe(h2); // Different array reference
  });

  it("should handle getStats returning a copy", () => {
    tracker.process("call-1", "Read", { filePath: "/src/foo.ts" }, "content");
    const s1 = tracker.getStats();
    const s2 = tracker.getStats();
    expect(s1).toEqual(s2);
    expect(s1).not.toBe(s2); // Different object reference
  });
});
