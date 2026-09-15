import { beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => new Map<string, string>());
vi.mock("child_process", () => ({ execFileSync: vi.fn(() => "") }));
vi.mock("../core/mcp-sync.js", () => ({ syncMcpServers: vi.fn() }));
vi.mock("../core/session-init.js", async (original) => ({
  ...(await original<object>()),
  ensureDirectories: vi.fn(),
  loadConfig: () => ({}),
  cleanupStaleStateFiles: vi.fn(),
  resetHudState: vi.fn(),
  runSessionInit: () => null,
  initializeSession: () => ({ startedAt: new Date().toISOString() }),
  saveStateSnapshot: vi.fn(),
  getProjectName: () => "context-window-test",
}));
vi.mock("../core/aide-client.js", async (original) => ({
  ...(await original<object>()),
  findAideBinary: () => "aide",
  getState: (_binary: string, cwd: string, key: string) =>
    state.get(`${cwd}:${key}`) ?? null,
  setState: (_binary: string, cwd: string, key: string, value: string) => {
    state.set(`${cwd}:${key}`, value);
    return true;
  },
}));

import { execFileSync } from "child_process";
import { contextWindow } from "../core/context-window.js";
import { createHooks } from "../opencode/hooks.js";
import type { OpenCodeClient } from "../opencode/types.js";

beforeEach(() => {
  state.clear();
  vi.mocked(execFileSync).mockClear();
});

describe("OpenCode child context attribution", () => {
  it("uses the child session for creation, compaction and subsequent tool observations", async () => {
    const cwd = "/tmp/aide-opencode-context-fixture";
    const hooks = await createHooks(cwd, cwd, {} as OpenCodeClient);
    const identity = (sessionId: string) => ({
      host: "opencode",
      sessionId,
      actorId: sessionId,
    });
    for (const id of ["parent", "child"])
      await hooks.event!({
        event: {
          type: "session.created",
          properties: {
            info: { id, ...(id === "child" ? { parentID: "parent" } : {}) },
          },
        },
      });
    const parent = contextWindow("aide", cwd, identity("parent"))!;
    const child = contextWindow("aide", cwd, identity("child"))!;
    expect(parent.status).toBe("active");
    expect(child.status).toBe("active");
    expect(child.id).not.toBe(parent.id);

    await hooks["experimental.session.compacting"]!(
      { sessionID: "child" },
      { context: [] },
    );
    expect(contextWindow("aide", cwd, identity("child"))?.status).toBe(
      "pending",
    );
    expect(contextWindow("aide", cwd, identity("parent"))).toEqual(parent);
    await hooks["tool.execute.after"]!(
      { tool: "aide_code_outline", sessionID: "child", callID: "pending" },
      { output: "outline" },
    );
    const observed = () =>
      vi
        .mocked(execFileSync)
        .mock.calls.map((call) => call[1] as string[])
        .filter((args) => args?.includes("--name=code_outline"));
    expect(observed().at(-1)).toEqual(
      expect.arrayContaining([
        "--session=child",
        "--attr=actor_id=child",
        "--attr=context_status=pending",
        `--attr=context_epoch=${child.id}`,
      ]),
    );

    await hooks.event!({
      event: { type: "session.compacted", properties: { sessionID: "child" } },
    });
    const completed = contextWindow("aide", cwd, identity("child"))!;
    expect(completed.status).toBe("active");
    expect(completed.continuity).toBe("reset");
    expect(completed.id).not.toBe(child.id);
    expect(contextWindow("aide", cwd, identity("parent"))).toEqual(parent);
    await hooks["tool.execute.after"]!(
      { tool: "aide_code_outline", sessionID: "child", callID: "after" },
      { output: "outline" },
    );
    expect(observed().at(-1)).toEqual(
      expect.arrayContaining([
        "--session=child",
        "--attr=actor_id=child",
        "--attr=context_status=active",
        `--attr=context_epoch=${completed.id}`,
      ]),
    );
  });
});
