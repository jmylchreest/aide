import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ObserveBatchEvent } from "../core/read-tracking.js";

const mocks = vi.hoisted(() => ({
  input: "",
  host: "claude-code",
  binary: "aide" as string | null,
  emit: vi.fn(),
  write: vi.fn(
    (_binary: string, _cwd: string, _events: ObserveBatchEvent[]) => true,
  ),
  observe: vi.fn(),
  register: vi.fn(() => true),
}));
vi.mock("child_process", () => ({
  execFileSync: vi.fn(() => Buffer.from("")),
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: async () => mocks.input,
  detectPlatform: () => mocks.host,
  findAideBinary: () => mocks.binary,
  emitHookResult: mocks.emit,
  installHookSafetyNet: vi.fn(),
  setMemoryState: () => true,
  isFalsy: () => true,
}));
vi.mock("../core/model-usage.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  recordModelUsage: mocks.write,
}));
vi.mock("../core/transcript-usage.js", () => ({
  registerChildTranscript: mocks.register,
}));
vi.mock("../core/context-window.js", () => ({
  ensureContextWindow: () => null,
}));
vi.mock("../lib/anchor.js", () => ({ setSessionContext: vi.fn() }));
vi.mock("../core/read-tracking.js", () => ({
  recordObserveEvent: mocks.observe,
  emitInjectionEvent: vi.fn(),
}));
vi.mock("../lib/hud.js", () => ({
  refreshHud: vi.fn(),
  invalidateHudRenderCache: vi.fn(),
}));
vi.mock("../lib/logger.js", () => ({
  debug: vi.fn(),
  Logger: class {
    info() {}
    debug() {}
    start() {}
    end() {}
    warn() {}
    error() {}
    flush() {}
  },
}));

let cwd: string;
function row(id = "child-response", sessionId = "session") {
  return {
    type: "assistant",
    sessionId,
    timestamp: "2026-09-10T09:13:37.911Z",
    message: {
      id,
      model: "claude-opus-5",
      usage: {
        input_tokens: 2,
        cache_read_input_tokens: 3,
        cache_creation_input_tokens: 4,
      },
    },
  };
}
function transcript(name: string, rows: unknown[]) {
  const path = join(cwd, name);
  writeFileSync(path, rows.map((r) => JSON.stringify(r)).join("\n") + "\n");
  return path;
}
async function stop(overrides: Record<string, unknown> = {}) {
  mocks.input = JSON.stringify({
    hook_event_name: "SubagentStop",
    session_id: "session",
    agent_id: "child",
    agent_type: "",
    cwd,
    ...overrides,
  });
  mocks.emit.mockClear();
  vi.resetModules();
  await import("../hooks/subagent-tracker.js");
  await vi.waitFor(() =>
    expect(mocks.emit).toHaveBeenCalledWith({ continue: true }),
  );
}
beforeEach(() => {
  cwd = mkdtempSync(join(tmpdir(), "aide-subagent-usage-"));
  vi.clearAllMocks();
  mocks.write.mockReset().mockReturnValue(true);
  mocks.host = "claude-code";
  mocks.binary = "aide";
});
afterEach(() => rmSync(cwd, { recursive: true, force: true }));

describe("Claude SubagentStop usage", () => {
  it("budgets sequential status, HUD, lifecycle, and usage operations", () => {
    const manifest = JSON.parse(
      readFileSync(
        new URL("../../.claude-plugin/plugin.json", import.meta.url),
        "utf8",
      ),
    );
    const hook = manifest.hooks.SubagentStop[0].hooks[0];
    // Two 5s state writes, two 10s HUD reads, 3s lifecycle recording,
    // 10s usage recording, a 2s version lookup, and 0.5s registration precede local file work.
    expect(hook.timeout).toBeGreaterThan(2 * 5 + 2 * 10 + 3 + 10 + 2 + 0.5);
    expect(hook.timeout).toBeLessThanOrEqual(60);
  });
  it("collects the explicit child transcript without reading the parent source", async () => {
    const child = transcript("child.jsonl", [row()]);
    const parent = transcript("parent.jsonl", [row("parent-response")]);
    await stop({ agent_transcript_path: child, transcript_path: parent });
    expect(mocks.register).toHaveBeenCalledWith(
      "aide",
      cwd,
      "claude-code",
      "session",
      child,
    );
    expect(mocks.write).toHaveBeenCalledWith("aide", cwd, [
      expect.objectContaining({
        session: "session",
        attrs: expect.objectContaining({
          host: "claude-code",
          usage_id: "child-response",
          input_tokens: "9",
          usage_coverage: "partial",
        }),
      }),
    ]);
    expect(mocks.observe).toHaveBeenCalledWith(
      "aide",
      cwd,
      expect.objectContaining({ name: "subagent-stop" }),
    );
  });

  it("preserves canonical response identity on repeated child delivery and parent overlap", async () => {
    const child = transcript("child.jsonl", [row("shared"), row("child-only")]);
    const parent = transcript("parent.jsonl", [row("shared")]);
    await stop({ agent_transcript_path: child });
    await stop({ agent_transcript_path: child });
    expect(mocks.write).toHaveBeenCalledTimes(2);
    expect(mocks.write.mock.calls[0]).toEqual(mocks.write.mock.calls[1]);
    const { collectTranscriptUsage } = await import("../core/model-usage.js");
    const parentEvent = collectTranscriptUsage(parent, "claude-code", "session")
      .events[0];
    expect(mocks.write.mock.calls[0][2][0]).toEqual(parentEvent);
  });

  it.each([undefined, "", "relative.jsonl", 7, null])(
    "does not invent a child path from %j",
    async (path) => {
      const parent = transcript("parent.jsonl", [row()]);
      await stop({ agent_transcript_path: path, transcript_path: parent });
      expect(mocks.write).not.toHaveBeenCalled();
    },
  );

  it.each(["missing", "directory", "mismatched", "malformed"])(
    "handles %s child evidence without blocking",
    async (kind) => {
      let path = join(cwd, "missing.jsonl");
      if (kind === "directory") path = cwd;
      if (kind === "mismatched")
        path = transcript("child.jsonl", [row("r", "other-session")]);
      if (kind === "malformed") {
        path = join(cwd, "child.jsonl");
        writeFileSync(path, "not-json\n");
      }
      await stop({ agent_transcript_path: path });
      expect(mocks.write).not.toHaveBeenCalled();
    },
  );

  it.each([undefined, "", "unknown"])(
    "rejects missing session identity %j",
    async (session) => {
      await stop({
        session_id: session,
        agent_transcript_path: transcript("child.jsonl", [row()]),
      });
      expect(mocks.write).not.toHaveBeenCalled();
    },
  );

  it("does not collect on another host or lifecycle event or without a binary", async () => {
    const path = transcript("child.jsonl", [row()]);
    mocks.host = "codex";
    await stop({ agent_transcript_path: path });
    mocks.host = "claude-code";
    await stop({ hook_event_name: "Stop", agent_transcript_path: path });
    mocks.binary = null;
    await stop({ agent_transcript_path: path });
    expect(mocks.write).not.toHaveBeenCalled();
  });

  it("retries failed writes on later delivery and still completes the hook", async () => {
    const path = transcript("child.jsonl", [row()]);
    mocks.write.mockReturnValueOnce(false);
    await stop({ agent_transcript_path: path });
    await stop({ agent_transcript_path: path });
    expect(mocks.write).toHaveBeenCalledTimes(2);
  });

  it("does not block completion if usage recording throws", async () => {
    mocks.write.mockImplementationOnce(() => {
      throw new Error("write failed");
    });
    await stop({ agent_transcript_path: transcript("child.jsonl", [row()]) });
    expect(mocks.write).toHaveBeenCalledTimes(1);
    expect(mocks.observe).toHaveBeenCalledWith(
      "aide",
      cwd,
      expect.objectContaining({ name: "subagent-stop" }),
    );
  });
});
