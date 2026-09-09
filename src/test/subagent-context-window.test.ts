import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  input: "",
  host: "claude-code" as "claude-code" | "codex",
  states: new Map<string, string>(),
  readFailure: false,
  writeFailure: false,
  writes: vi.fn(),
  exec: vi.fn((_binary: string, _args: string[], _options: unknown) =>
    Buffer.from(""),
  ),
  emit: vi.fn(),
}));
vi.mock("child_process", () => ({ execFileSync: mocks.exec }));
vi.mock("../core/aide-client.js", () => ({
  getState: (_binary: string, _cwd: string, key: string) =>
    mocks.readFailure ? null : (mocks.states.get(key) ?? null),
  setState: (_binary: string, _cwd: string, key: string, value: string) => {
    mocks.writes(key, value);
    if (mocks.writeFailure) return false;
    mocks.states.set(key, value);
    return true;
  },
  runAide: vi.fn((_binary: string, _cwd: string, args: string[]) => {
    if (mocks.readFailure) return null;
    const key = args[2];
    if (args[0] !== "state" || args[1] !== "init" || args[4] !== "--json")
      throw new Error("Unexpected state command");
    if (!mocks.states.has(key)) {
      mocks.writes(key, args[3]);
      if (mocks.writeFailure) return null;
      mocks.states.set(key, args[3]);
    }
    return JSON.stringify({ key, value: mocks.states.get(key) });
  }),
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: async () => mocks.input,
  detectPlatform: () => mocks.host,
  findAideBinary: () => "aide",
  emitHookResult: mocks.emit,
  installHookSafetyNet: vi.fn(),
  setMemoryState: () => true,
  isFalsy: () => true,
}));
vi.mock("../lib/anchor.js", () => ({ setSessionContext: vi.fn() }));
vi.mock("../core/read-tracking.js", () => ({
  recordObserveEvent: vi.fn(),
  emitInjectionEvent: vi.fn(),
  recordFileRead: vi.fn(),
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

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
  mocks.states.clear();
  mocks.readFailure = false;
  mocks.writeFailure = false;
  mocks.host = "claude-code";
});

async function lifecycle(overrides: Record<string, unknown> = {}) {
  mocks.input = JSON.stringify({
    hook_event_name: "SubagentStart",
    cwd: "/project",
    session_id: "session",
    agent_id: "child",
    agent_type: "worker",
    ...overrides,
  });
  mocks.emit.mockClear();
  vi.resetModules();
  if (overrides.hook_event_name === "PostCompact")
    await import("../hooks/post-compact.js");
  else await import("../hooks/subagent-tracker.js");
  await vi.waitFor(() => expect(mocks.emit).toHaveBeenCalled());
}

describe("subagent context initialization", () => {
  it("handles Codex PostCompact for only the reported actor and records its reset", async () => {
    mocks.host = "codex";
    const { updateContextWindow, contextWindow } =
      await import("../core/context-window.js");
    const { recordObserveEvent } = await import("../core/read-tracking.js");
    const parent = { host: "codex", sessionId: "session", actorId: "session" };
    const child = { ...parent, actorId: "child" };
    const parentWindow = updateContextWindow(
      "aide",
      "/project",
      parent,
      "startup",
    );
    const previous = updateContextWindow("aide", "/project", child, "startup");
    updateContextWindow("aide", "/project", child, "compact_pending");
    await lifecycle({ hook_event_name: "PostCompact", trigger: "auto" });
    const completed = contextWindow("aide", "/project", child);
    expect(completed).toMatchObject({ status: "active", continuity: "reset" });
    expect(completed?.id).not.toBe(previous?.id);
    expect(contextWindow("aide", "/project", parent)).toEqual(parentWindow);
    expect(recordObserveEvent).toHaveBeenCalledWith(
      "aide",
      "/project",
      expect.objectContaining({
        name: "post-compact",
        session: "session",
        subtype: "auto",
        attrs: {
          host: "codex",
          actor_id: "child",
          context_status: "active",
          context_epoch: completed?.id,
        },
      }),
    );
    await lifecycle({ hook_event_name: "PostCompact", agent_id: undefined });
    expect(contextWindow("aide", "/project", parent)?.id).not.toBe(
      parentWindow?.id,
    );
    expect(contextWindow("aide", "/project", child)).toEqual(completed);
  });

  it("keeps failed PostCompact persistence unknown in recorded evidence", async () => {
    mocks.writeFailure = true;
    await lifecycle({ hook_event_name: "PostCompact" });
    const { recordObserveEvent } = await import("../core/read-tracking.js");
    expect(recordObserveEvent).toHaveBeenCalledWith(
      "aide",
      "/project",
      expect.objectContaining({
        attrs: {
          host: "claude-code",
          actor_id: "child",
          context_status: "unknown",
        },
      }),
    );
    expect(mocks.states.size).toBe(0);
  });

  it("does not establish PostCompact windows without a known session", async () => {
    await lifecycle({ hook_event_name: "PostCompact", session_id: "unknown" });
    expect(mocks.writes).not.toHaveBeenCalled();
  });
  it.each(["claude-code", "codex"] as const)(
    "initializes %s's explicit child scope for later tool observations",
    async (host) => {
      mocks.host = host;
      const { updateContextWindow, contextWindow } =
        await import("../core/context-window.js");
      const parentIdentity = { host, sessionId: "session", actorId: "session" };
      const childIdentity = { host, sessionId: "session", actorId: "child" };
      const parent = updateContextWindow(
        "aide",
        "/project",
        parentIdentity,
        "startup",
      )!;
      await lifecycle();
      const child = contextWindow("aide", "/project", childIdentity);
      expect(child).toMatchObject({ status: "active", continuity: "new" });
      expect(child?.id).not.toBe(parent.id);
      expect(contextWindow("aide", "/project", parentIdentity)).toEqual(parent);

      const { recordToolEvent } = await import("../core/tool-observe.js");
      recordToolEvent("aide", "/project", {
        host,
        sessionId: "session",
        actorId: "child",
        invocationId: "call",
        toolName: "Bash",
        toolInput: { command: "pwd" },
        toolResponse: "/project",
      });
      const args = mocks.exec.mock.calls.at(-1)?.[1];
      expect(args).toEqual(
        expect.arrayContaining([
          `--attr=host=${host}`,
          "--session=session",
          "--attr=actor_id=child",
          "--attr=context_status=active",
          `--attr=context_epoch=${child!.id}`,
        ]),
      );
    },
  );

  it.each(["compact_pending", "compact"] as const)(
    "preserves an existing %s child window on repeated start delivery",
    async (transition) => {
      await lifecycle();
      const { updateContextWindow, contextWindow } =
        await import("../core/context-window.js");
      const identity = {
        host: mocks.host,
        sessionId: "session",
        actorId: "child",
      };
      const expected = updateContextWindow(
        "aide",
        "/project",
        identity,
        transition,
      );
      mocks.writes.mockClear();
      await lifecycle();
      expect(contextWindow("aide", "/project", identity)).toEqual(expected);
      expect(mocks.writes).not.toHaveBeenCalled();
    },
  );

  it.each([
    { hook_event_name: "SubagentStop" },
    { agent_id: "" },
    { agent_id: undefined },
    { agent_id: "session" },
    { agent_id: "unknown" },
    { agent_id: " " },
    { session_id: "" },
    { session_id: "unknown" },
    { session_id: undefined },
  ])("does not invent a scope for %j", async (overrides) => {
    await lifecycle(overrides);
    expect(mocks.writes).not.toHaveBeenCalled();
    expect(mocks.states.size).toBe(0);
  });

  it("does not initialize after a failed read or overwrite malformed stored state", async () => {
    mocks.readFailure = true;
    await lifecycle();
    expect(mocks.writes).not.toHaveBeenCalled();
    mocks.readFailure = false;
    const { contextScope } = await import("../core/context-window.js");
    const key = `context-window:${contextScope({ host: mocks.host, sessionId: "session", actorId: "child" })}`;
    mocks.states.set(key, "broken state");
    await lifecycle();
    expect(mocks.writes).not.toHaveBeenCalled();
    expect(mocks.states.get(key)).toBe("broken state");
  });

  it("rejects unsupported or mismatched atomic initialization responses", async () => {
    const { runAide } = await import("../core/aide-client.js");
    const { ensureContextWindow, contextScope } =
      await import("../core/context-window.js");
    const identity = {
      host: mocks.host,
      sessionId: "session",
      actorId: "child",
    };
    const key = `context-window:${contextScope(identity)}`;
    const window = {
      version: 1,
      id: "epoch",
      status: "active",
      continuity: "new",
      reason: "startup",
    };
    for (const result of [
      "Unknown command: state init",
      "{}",
      JSON.stringify({ key: "wrong-scope", value: JSON.stringify(window) }),
      JSON.stringify({ key, value: JSON.stringify({ ...window, version: 2 }) }),
      JSON.stringify({
        key,
        value: JSON.stringify({ ...window, status: "unknown" }),
      }),
      JSON.stringify({ key, value: JSON.stringify(window), agent: "parent" }),
    ]) {
      vi.mocked(runAide).mockReturnValueOnce(result);
      expect(ensureContextWindow("aide", "/project", identity)).toBeNull();
    }
    expect(mocks.writes).not.toHaveBeenCalled();
  });

  it.each([
    { continuity: "unverified" },
    { reason: undefined },
    { id: " " },
    { id: "epoch\nother" },
  ])(
    "keeps malformed existing state unknown to later observations: %j",
    async (patch) => {
      const { contextScope } = await import("../core/context-window.js");
      const identity = {
        host: mocks.host,
        sessionId: "session",
        actorId: "child",
      };
      const key = `context-window:${contextScope(identity)}`;
      const malformed = JSON.stringify({
        version: 1,
        id: "epoch",
        status: "active",
        continuity: "new",
        reason: "startup",
        ...patch,
      });
      mocks.states.set(key, malformed);
      await lifecycle();
      expect(mocks.writes).not.toHaveBeenCalled();
      expect(mocks.states.get(key)).toBe(malformed);
      const { recordToolEvent } = await import("../core/tool-observe.js");
      recordToolEvent("aide", "/project", {
        ...identity,
        invocationId: "call",
        toolName: "Bash",
        toolResponse: "result",
      });
      const args = mocks.exec.mock.calls.at(-1)?.[1];
      expect(args).toContain("--attr=context_status=unknown");
      expect(args).not.toEqual(
        expect.arrayContaining([
          expect.stringMatching(/^--attr=context_epoch=/),
        ]),
      );
    },
  );

  it("keeps later observations unknown after initialization fails to persist", async () => {
    mocks.writeFailure = true;
    await lifecycle();
    expect(mocks.writes).toHaveBeenCalledTimes(1);
    expect(mocks.states.size).toBe(0);
    const { recordToolEvent } = await import("../core/tool-observe.js");
    recordToolEvent("aide", "/project", {
      host: mocks.host,
      sessionId: "session",
      actorId: "child",
      invocationId: "call",
      toolName: "Bash",
      toolResponse: "result",
    });
    const args = mocks.exec.mock.calls.at(-1)?.[1];
    expect(args).toContain("--attr=context_status=unknown");
    expect(args).not.toEqual(
      expect.arrayContaining([expect.stringMatching(/^--attr=context_epoch=/)]),
    );
  });
});
