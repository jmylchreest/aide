import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

vi.mock("child_process", () => ({ execFileSync: vi.fn(() => "") }));
vi.mock("../core/mcp-sync.js", () => ({ syncMcpServers: vi.fn() }));
vi.mock("../core/session-init.js", async (original) => ({
  ...(await original<object>()),
  ensureDirectories: vi.fn(),
  loadConfig: () => ({}),
  cleanupStaleStateFiles: vi.fn(),
  resetHudState: vi.fn(),
  runSessionInit: () => null,
  getProjectName: () => "test",
}));
vi.mock("../core/aide-client.js", async (original) => ({
  ...(await original<object>()),
  findAideBinary: () => "aide",
  getState: () => null,
  setState: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", async (original) => ({
  ...(await original<object>()),
  codeWatchEnabled: vi.fn(() => true),
}));
vi.mock("../core/read-tracking.js", async (original) => ({
  ...(await original<object>()),
  getPreviousRead: vi.fn(() => null),
  checkFileReadFreshness: vi.fn(() => null),
  recordFileRead: vi.fn(),
}));
import { execFileSync } from "child_process";
import { codeWatchEnabled } from "../lib/hook-utils.js";
import {
  getPreviousRead,
  checkFileReadFreshness,
  recordFileRead,
} from "../core/read-tracking.js";
import { createHooks } from "../opencode/hooks.js";
import type { OpenCodeClient, OpenCodeToolResult } from "../opencode/types.js";

let cwd: string;
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(codeWatchEnabled).mockReturnValue(true);
  vi.mocked(getPreviousRead).mockReturnValue(null);
  vi.mocked(checkFileReadFreshness).mockReturnValue(null);
  cwd = mkdtempSync(join(tmpdir(), "aide-opencode-hints-"));
  writeFileSync(join(cwd, "large.ts"), "export const value = 1;\n".repeat(400));
  writeFileSync(join(cwd, "small.ts"), "export const value = 1;\n");
  vi.mocked(execFileSync).mockImplementation((_binary, args) => {
    if (args?.[0] === "code" && args[1] === "search") {
      return JSON.stringify([
        { name: "authenticate", kind: "function", file: "auth.ts", start: 12 },
      ]);
    }
    return "[]";
  });
});
afterEach(() => rmSync(cwd, { recursive: true, force: true }));

async function execute(
  tool: string,
  args: Record<string, unknown>,
  result?: OpenCodeToolResult,
) {
  const hooks = await createHooks(cwd, cwd, {} as OpenCodeClient);
  const input = { tool, sessionID: "session", callID: "call" };
  const before = structuredClone(args);
  await hooks["tool.execute.before"]!(input, { args });
  expect(args).toEqual(before);
  const output = result ?? {
    output: "original é",
    title: "tool title",
    metadata: { args, custom: true },
  };
  await hooks["tool.execute.after"]!({ ...input, args }, output);
  return output;
}

describe("OpenCode retrieval hint delivery", () => {
  it("appends a large-read hint to the rendered result and accounts for only the added bytes", async () => {
    const args = { filePath: "large.ts" };
    const content = readFileSync(join(cwd, "large.ts"), "utf8");
    const output = await execute("read", args, {
      output: content,
      title: "tool title",
      metadata: { args, custom: true },
    });
    expect(output.output).toContain(`${content}\n\n[aide:context]`);
    expect(output.output).toContain("code_outline");
    expect(output.metadata).toEqual({ args, custom: true });
    expect(output.title).toBe("tool title");
    const calls = vi.mocked(execFileSync).mock.calls;
    const batches = calls.filter((call) => call[1]?.includes("--stdin"));
    const events = batches.flatMap((call) =>
      String((call[2] as { input: string }).input)
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line)),
    );
    const appended = output.output!.slice(content.length);
    expect(events).toContainEqual(
      expect.objectContaining({
        kind: "injection",
        session: "session",
        attrs: expect.objectContaining({
          host: "opencode",
          actor_id: "session",
          invocation_id: "call",
          content_boundary: "appended_text",
          observation_stage: "aide_context",
          payload_bytes: String(Buffer.byteLength(appended)),
        }),
      }),
    );
    expect(
      calls.some(
        (call) =>
          call[1]?.includes("--name=output-transform") &&
          call[1].includes(
            `--attr=after_bytes=${Buffer.byteLength(output.output!)}`,
          ),
      ),
    ).toBe(true);
    expect(output.output?.match(/\[aide:context\]/g)).toHaveLength(1);
    expect(recordFileRead).toHaveBeenCalledWith("aide", cwd, "large.ts", {
      identity: { host: "opencode", sessionId: "session", actorId: "session" },
      content,
    });
    expect(vi.mocked(getPreviousRead).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(recordFileRead).mock.invocationCallOrder[0],
    );
  });

  it("delivers hints without recording coverage for unverified read output", async () => {
    const output = await execute("read", { filePath: "large.ts" });
    expect(output.output).toContain("original é\n\n[aide:context]");
    expect(recordFileRead).not.toHaveBeenCalled();
  });

  it("delivers indexed symbol matches with the grep result", async () => {
    const output = await execute("grep", { pattern: "authenticate" });
    expect(output.output).toContain("original é\n\n[aide:code-index]");
    expect(output.output).toContain("auth.ts:12");
    expect(output.output).toContain("code_read_symbol");
    expect(
      vi
        .mocked(execFileSync)
        .mock.calls.filter(
          (call) => call[1]?.[0] === "code" && call[1][1] === "search",
        ),
    ).toHaveLength(1);
  });

  it("prefers a single reuse hint for a verified earlier read in this session", async () => {
    vi.mocked(getPreviousRead).mockReturnValue("2026-09-09T00:00:00Z");
    vi.mocked(checkFileReadFreshness).mockReturnValue({
      indexed: true,
      fresh: true,
      symbols: 1,
      outline_available: true,
      estimated_tokens: 100,
    });
    const output = await execute("read", { filePath: "large.ts" });
    expect(output.output).toContain("[aide:smart-read]");
    expect(output.output).not.toContain("[aide:context]");
    expect(getPreviousRead).toHaveBeenCalledWith("aide", cwd, "large.ts", {
      host: "opencode",
      sessionId: "session",
      actorId: "session",
    });
    expect(getPreviousRead).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["read", { filePath: "small.ts" }],
    ["read", { filePath: "large.ts", limit: 20 }],
    ["read", { filePath: "large.ts", offset: 100 }],
    ["grep", { pattern: "auth.*" }],
  ])(
    "keeps %s targeted or non-symbol retrieval unchanged (%j)",
    async (tool, args) => {
      expect((await execute(tool, args)).output).toBe("original é");
    },
  );

  it.each([
    ["grep", { pattern: "authenticate" }],
    ["read", { filePath: "large.ts" }],
  ])("keeps disabled indexing quiet for %s", async (tool, args) => {
    vi.mocked(codeWatchEnabled).mockReturnValue(false);
    expect((await execute(tool, args)).output).toBe("original é");
    expect(
      vi
        .mocked(execFileSync)
        .mock.calls.some((call) => call[1]?.[0] === "code"),
    ).toBe(false);
  });

  it("keeps missing symbol matches and index failures quiet", async () => {
    vi.mocked(execFileSync).mockReturnValue("[]");
    expect((await execute("grep", { pattern: "absent" })).output).toBe(
      "original é",
    );
    vi.mocked(execFileSync).mockImplementation((_binary, args) => {
      if (args?.[0] === "code") throw new Error("index unavailable");
      return "[]";
    });
    expect((await execute("grep", { pattern: "absent" })).output).toBe(
      "original é",
    );
  });

  it.each([
    { output: "failed read", isError: true },
    {
      content: [{ type: "text", text: "unrendered" }],
      _meta: { receipt: "original" },
    },
  ])("preserves failures and unsupported envelopes (%j)", async (result) => {
    const original = structuredClone(result);
    expect(await execute("read", { filePath: "large.ts" }, result)).toEqual(
      original,
    );
  });
});
