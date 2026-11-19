import { afterEach, beforeEach, expect, it, vi } from "vitest";

const m = vi.hoisted(() => ({
  readStdin: vi.fn(),
  emit: vi.fn(),
  root: vi.fn(),
  globalConfig: vi.fn(),
  directories: vi.fn(),
  mcp: vi.fn(),
  binary: vi.fn(),
  findBinary: vi.fn(),
  context: vi.fn(),
  resolveAnchor: vi.fn(),
  writeAnchor: vi.fn(),
  resetHud: vi.fn(),
  config: vi.fn(),
  cleanup: vi.fn(),
  initialize: vi.fn(),
  sessionInit: vi.fn(),
  welcome: vi.fn(),
  resume: vi.fn(),
  observe: vi.fn(),
  setSession: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: m.readStdin,
  emitHookResult: m.emit,
  installHookSafetyNet: vi.fn(),
  detectPlatform: () => "claude-code",
  isFalsy: (value: string | undefined) => value === "0",
}));
vi.mock("../lib/logger.js", () => ({
  debug: vi.fn(),
  setDebugCwd: vi.fn(),
  Logger: class {
    start() {}
    end() {}
    info() {}
    warn() {}
    error() {}
    debug() {}
    flush() {}
    isEnabled() {
      return false;
    }
    getLogFile() {
      return "unused";
    }
  },
}));
vi.mock("../lib/project-root.js", () => ({ findProjectRoot: m.root }));
vi.mock("../lib/aide-downloader.js", () => ({
  ensureAideBinary: m.binary,
  findAideBinary: m.findBinary,
  getPluginRoot: () => null,
}));
vi.mock("../lib/anchor.js", () => ({
  setSessionContext: m.setSession,
  resolveAnchorViaBinary: m.resolveAnchor,
  writeSessionAnchor: m.writeAnchor,
}));
vi.mock("../core/hook-context.js", () => ({
  updateHookContextWindow: m.context,
}));
vi.mock("../core/mcp-sync.js", () => ({ syncMcpServers: m.mcp }));
vi.mock("../core/session-init.js", () => ({
  ensureDirectories: m.directories,
  loadConfig: m.config,
  loadGlobalConfig: m.globalConfig,
  initializeSession: m.initialize,
  cleanupStaleStateFiles: m.cleanup,
  resetHudState: m.resetHud,
  getProjectName: () => "project",
  runSessionInit: m.sessionInit,
  buildWelcomeContext: m.welcome,
  formatTimeAgo: vi.fn(),
}));
vi.mock("../core/session-resume-logic.js", () => ({
  buildResumeContext: m.resume,
}));
vi.mock("../core/read-tracking.js", () => ({
  recordObserveEventsBatch: m.observe,
  injectionBatchEvent: (input: Record<string, unknown>) => ({
    kind: "injection",
    ...input,
  }),
}));

let phases: string[];
beforeEach(() => {
  vi.resetModules();
  vi.resetAllMocks();
  vi.stubEnv("AIDE_MCP_SYNC", "1");
  vi.spyOn(process.stderr, "write").mockImplementation(() => true);
  phases = [];
  const phase = (name: string, value?: unknown) => () => {
    phases.push(name);
    return value;
  };
  m.readStdin.mockResolvedValue(
    JSON.stringify({
      cwd: "/project/nested",
      session_id: "session",
      source: "resume",
    }),
  );
  m.root.mockReturnValue({
    root: "/project",
    realRoot: "/real/project",
    hasMarker: true,
  });
  m.globalConfig.mockReturnValue({ requireGit: true });
  m.directories.mockImplementation(
    phase("directories", { created: 0, existed: 1 }),
  );
  const scope = { imported: 0, serversWritten: 0, skipped: 0 };
  m.mcp.mockImplementation(phase("mcp", { user: scope, project: scope }));
  m.binary.mockImplementation(async () => {
    phases.push("binary");
    return { binary: "aide", error: null, warning: null, downloaded: false };
  });
  m.findBinary.mockReturnValue("aide");
  m.context.mockImplementation(phase("context-window"));
  m.resolveAnchor.mockImplementation(
    phase("anchor-resolve", {
      root: "/project",
      realRoot: "/real/project",
      chain: [],
    }),
  );
  m.writeAnchor.mockImplementation(phase("anchor-write"));
  m.resetHud.mockImplementation(phase("hud-reset"));
  m.config.mockImplementation(phase("config", {}));
  m.cleanup.mockImplementation(phase("cleanup", { scanned: 0, deleted: 0 }));
  m.initialize.mockImplementation(phase("state", { sessionId: "session" }));
  m.sessionInit.mockImplementation(
    phase("session-init", {
      static: { global: [], project: [], decisions: [] },
      dynamic: { sessions: [] },
      sources: [
        {
          id: "memory",
          kind: "memory",
          name: "Useful",
          content: "remember",
          scope: "project",
          category: "learning",
          tags: ["one"],
          sessionId: "prior",
          score: 0.75,
        },
      ],
    }),
  );
  m.welcome.mockImplementation(phase("welcome", "Welcome"));
  m.resume.mockImplementation(phase("resume", "Checkpoint"));
  m.observe.mockImplementation(phase("observe"));
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
});

async function start() {
  await import("../hooks/session-start.js");
  await vi.waitFor(() => expect(m.emit).toHaveBeenCalledTimes(1));
}

it("validates the root before creating state or ensuring the binary", async () => {
  m.root.mockReturnValue({
    root: "/project/nested",
    realRoot: "/project/nested",
    hasMarker: false,
  });
  await start();
  expect(phases).toEqual([]);
  expect(m.writeAnchor).not.toHaveBeenCalled();
  expect(m.emit).toHaveBeenCalledWith();
});

it("preserves startup phase order and batches lifecycle and injection observations", async () => {
  await start();
  expect(phases).toEqual([
    "directories",
    "mcp",
    "binary",
    "context-window",
    "anchor-resolve",
    "anchor-write",
    "hud-reset",
    "config",
    "cleanup",
    "state",
    "session-init",
    "welcome",
    "resume",
    "observe",
  ]);
  expect(m.resolveAnchor).toHaveBeenCalledWith("aide", "/project/nested");
  expect(m.initialize).toHaveBeenCalledWith("session", "/project");
  expect(m.binary).toHaveBeenCalledTimes(1);
  expect(m.observe).toHaveBeenCalledTimes(1);
  expect(m.observe).toHaveBeenCalledWith("aide", "/project", [
    expect.objectContaining({
      source: "resume-checkpoint",
      content: "Checkpoint",
      sessionId: "session",
    }),
    expect.objectContaining({
      kind: "session",
      name: "session-start",
      subtype: "resume",
      session: "session",
    }),
    expect.objectContaining({
      content: "remember",
      attrs: {
        scope: "project",
        category: "learning",
        tags: "one",
        source_session_id: "prior",
        score_at_injection: "0.75",
        source_id: "memory",
      },
    }),
  ]);
  expect(m.emit).toHaveBeenCalledWith(
    expect.objectContaining({
      continue: true,
      hookSpecificOutput: {
        hookEventName: "SessionStart",
        additionalContext: "Welcome\n\nCheckpoint",
      },
    }),
  );
});

it.each(["mcp", "resolveAnchor", "resume", "observe"] as const)(
  "continues when optional %s work fails",
  async (phase) => {
    m[phase].mockImplementation(() => {
      throw new Error("unavailable");
    });
    await start();
    expect(m.emit).toHaveBeenCalledWith(
      expect.objectContaining({
        continue: true,
        hookSpecificOutput: expect.objectContaining({
          additionalContext: expect.stringContaining("Welcome"),
        }),
      }),
    );
  },
);

it("continues without binary-backed phases when the binary is unavailable", async () => {
  m.binary.mockResolvedValue({
    binary: null,
    error: "unavailable",
    warning: null,
    downloaded: false,
  });
  m.findBinary.mockReturnValue(null);
  await start();
  for (const fn of [
    m.context,
    m.resolveAnchor,
    m.writeAnchor,
    m.sessionInit,
    m.resume,
    m.observe,
  ])
    expect(fn).not.toHaveBeenCalled();
  expect(m.welcome).toHaveBeenCalledWith(expect.anything(), expect.anything(), {
    error: "unavailable",
    warning: null,
    info: [],
  });
});

it.each(["", "not json"])(
  "continues without bootstrapping for invalid input %j",
  async (input) => {
    m.readStdin.mockResolvedValue(input);
    await start();
    expect(phases).toEqual([]);
    expect(m.emit).toHaveBeenCalledWith();
  },
);
