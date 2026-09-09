import { afterEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import type { UsageWriteRequest, UsageWriteResult } from "../core/model-usage.js";

const mocks = vi.hoisted(() => ({
  write: vi.fn(({ events }: UsageWriteRequest): UsageWriteResult => ({
    status: "acknowledged",
    recorded: events.length,
  })),
  input: "",
  host: "claude-code" as "claude-code" | "codex",
}));
vi.mock("../core/model-usage.js", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../core/model-usage.js")>();
  return {
    ...actual,
    recordModelUsage: mocks.write,
    createOpenCodeUsageRecorder: () =>
      actual.createOpenCodeUsageRecorder(mocks.write),
  };
});
vi.mock("../core/aide-client.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  findAideBinary: () => "/fake-aide",
}));
vi.mock("../core/session-init.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  runSessionInit: () => null,
}));
vi.mock("../core/mcp-sync.js", () => ({ syncMcpServers: () => {} }));
vi.mock("../core/session-summary-logic.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  buildSessionSummary: () => null,
}));
vi.mock("../core/partial-memory.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  gatherPartials: () => [],
  cleanupPartials: () => 0,
}));
vi.mock("../lib/hook-utils.js", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  readStdin: async () => mocks.input,
  detectPlatform: () => mocks.host,
  findAideBinary: () => "/fake-aide",
  emitHookResult: () => {},
  installHookSafetyNet: () => {},
}));

const dirs: string[] = [];
function directory() {
  const dir = mkdtempSync(join(tmpdir(), "aide-usage-hooks-"));
  dirs.push(dir);
  return dir;
}
afterEach(() => {
  for (const dir of dirs.splice(0))
    rmSync(dir, { recursive: true, force: true });
  mocks.write.mockClear();
});

describe("model usage host entry points", () => {
  it("records OpenCode step parts through the real plugin event handler", async () => {
    const { createHooks } = await import("../opencode/hooks.js");
    const cwd = directory();
    const hooks = await createHooks(cwd, cwd, {
      app: { log: async () => {} },
      session: { create: async () => ({ id: "s" }), prompt: async () => ({}) },
      event: { subscribe: async () => ({ stream: [] as any }) },
    });
    const part = {
      type: "step-finish",
      id: "p",
      sessionID: "s",
      messageID: "m",
      tokens: { input: 12 },
    };
    await hooks.event!({
      event: { type: "message.part.updated", properties: { part } },
    });
    expect(mocks.write).toHaveBeenCalledWith({
      binary: "/fake-aide",
      cwd,
      events: [
        expect.objectContaining({
          session: "s",
          name: "model_usage",
          attrs: expect.objectContaining({
            usage_id: "p",
            uncached_input_tokens: "12",
          }),
        }),
      ],
    });
    await hooks.event!({
      event: {
        type: "message.updated",
        properties: {
          info: {
            id: "m",
            sessionID: "s",
            role: "assistant",
            tokens: { input: 999 },
          },
        },
      },
    });
    expect(mocks.write).toHaveBeenCalledTimes(1);
  });

  it.each(["claude-code", "codex"] as const)(
    "records %s from Stop's explicit transcript",
    async (host) => {
      vi.resetModules();
      const cwd = directory(),
        path = join(cwd, "transcript.jsonl");
      mocks.host = host;
      const row =
        host === "codex"
          ? {
              type: "token_usage_record",
              payload: {
                thread_id: "s",
                response_id: "r",
                usage: { input_tokens: 7 },
              },
            }
          : {
              type: "assistant",
              sessionId: "s",
              message: { id: "r", usage: { input_tokens: 7 } },
            };
      writeFileSync(path, JSON.stringify(row) + "\n");
      mocks.input = JSON.stringify({
        hook_event_name: "Stop",
        session_id: "s",
        cwd,
        transcript_path: path,
      });
      await import("../hooks/session-summary.js");
      await vi.waitFor(() =>
        expect(mocks.write).toHaveBeenCalledWith({
          binary: "/fake-aide",
          cwd,
          events: [
            expect.objectContaining({
              session: "s",
              attrs: expect.objectContaining({ host, usage_id: "r" }),
            }),
          ],
        }),
      );
    },
  );
});
