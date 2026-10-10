import { beforeEach, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
const mocks = vi.hoisted(() => ({
  input: "",
  host: "claude-code",
  binary: "aide" as string | null,
  catchUp: vi.fn(() => ({ status: "partial" })),
  emit: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: async () => mocks.input,
  emitHookResult: mocks.emit,
  installHookSafetyNet: vi.fn(),
  findAideBinary: () => mocks.binary,
  detectPlatform: () => mocks.host,
}));
vi.mock("../lib/logger.js", () => ({ debug: vi.fn(), setDebugCwd: vi.fn() }));
vi.mock("../lib/anchor.js", () => ({ setSessionContext: vi.fn() }));
vi.mock("../core/transcript-usage.js", () => ({
  catchUpTranscriptUsage: mocks.catchUp,
}));
beforeEach(() => {
  vi.clearAllMocks();
  mocks.host = "claude-code";
  mocks.binary = "aide";
});
async function hook(fields: Record<string, unknown> = {}) {
  mocks.input = JSON.stringify({
    hook_event_name: "UserPromptSubmit",
    session_id: "session",
    cwd: "/project",
    transcript_path: "/explicit/main.jsonl",
    ...fields,
  });
  vi.resetModules();
  await import("../hooks/usage-catchup.js");
  await vi.waitFor(() =>
    expect(mocks.emit).toHaveBeenCalledWith({ continue: true }),
  );
}
it.each(["SessionStart", "UserPromptSubmit", "SessionEnd"])(
  "uses explicit host paths and bounded work at %s",
  async (event) => {
    await hook({ hook_event_name: event });
    expect(mocks.catchUp).toHaveBeenCalledWith(
      "aide",
      "/project",
      "claude-code",
      "session",
      "/explicit/main.jsonl",
      { budgetMs: event === "SessionEnd" ? 1000 : 2000 },
    );
  },
);
it.each(["Stop", "SubagentStop", "PostToolUse"])(
  "does not pretend %s is a later catch-up boundary",
  async (event) => {
    await hook({ hook_event_name: event });
    expect(mocks.catchUp).not.toHaveBeenCalled();
  },
);
it.each([undefined, "", "unknown"])(
  "does not infer a missing session identity %s",
  async (session_id) => {
    await hook({ session_id });
    expect(mocks.catchUp).not.toHaveBeenCalled();
  },
);
it("does not add transcript polling to OpenCode", async () => {
  mocks.host = "opencode";
  await hook();
  expect(mocks.catchUp).not.toHaveBeenCalled();
});
it("uses Codex host identity", async () => {
  mocks.host = "codex";
  await hook();
  expect(mocks.catchUp).toHaveBeenCalledWith(
    "aide",
    "/project",
    "codex",
    "session",
    "/explicit/main.jsonl",
    { budgetMs: 2000 },
  );
});
it("registers bounded catch-up on available Claude and Codex lifecycle events", async () => {
  const manifest = JSON.parse(
    readFileSync(
      new URL("../../.claude-plugin/plugin.json", import.meta.url),
      "utf8",
    ),
  );
  for (const event of ["SessionStart", "UserPromptSubmit", "SessionEnd"]) {
    const hooks = manifest.hooks[event].flatMap(
      (group: { hooks: { command: string; timeout: number }[] }) => group.hooks,
    );
    expect(
      hooks.find((h: { command: string }) =>
        h.command.includes("usage-catchup.ts"),
      )?.timeout,
    ).toBe(event === "SessionEnd" ? 2 : 3);
  }
  const { generateHooksJson } = await import("../cli/codex-config.js");
  const generated = generateHooksJson("aide-plugin hook");
  for (const event of ["SessionStart", "UserPromptSubmit"])
    expect(
      generated.hooks[event][0].hooks.some(
        (h) =>
          h.command === "aide-plugin hook usage-catchup" && h.timeout === 3,
      ),
    ).toBe(true);
  expect(generated.hooks.SessionEnd).toBeUndefined();
});
