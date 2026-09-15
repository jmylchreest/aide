import { createHash } from "crypto";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
vi.mock("../core/aide-client.js", () => ({
  setState: vi.fn(),
  getState: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", () => ({
  codeWatchEnabled: vi.fn(() => true),
}));
import { codeWatchEnabled } from "../lib/hook-utils.js";
import { setState, getState } from "../core/aide-client.js";
import { contextWindow, updateContextWindow } from "../core/context-window.js";
import { updateHookContextWindow } from "../core/hook-context.js";
import { recordFileRead, getPreviousRead } from "../core/read-tracking.js";
import { checkContextGuard } from "../core/context-guard.js";

const identity = {
  host: "claude-code",
  sessionId: "session",
  actorId: "session",
};
let cwd: string;
const states = new Map<string, string>();
beforeEach(() => {
  vi.mocked(codeWatchEnabled).mockReturnValue(true);
  states.clear();
  cwd = mkdtempSync(join(tmpdir(), "aide-context-window-"));
  vi.mocked(setState).mockImplementation((_binary, project, key, value) => {
    states.set(`${project}:${key}`, value);
    return true;
  });
  vi.mocked(getState).mockImplementation(
    (_binary, project, key) => states.get(`${project}:${key}`) ?? null,
  );
  writeFileSync(join(cwd, "file.ts"), "const value = 1;\n");
});
afterEach(() => rmSync(cwd, { recursive: true, force: true }));

function record(content = "const value = 1;\n") {
  recordFileRead("aide", cwd, "file.ts", { identity, content });
}

describe("context windows and verified read coverage", () => {
  it.each(["claude-code", "codex"])(
    "scopes %s child compaction and completion to the observed actor",
    (host) => {
      const parent = { host, sessionId: "session", actorId: "session" };
      const child = { ...parent, actorId: "child" };
      const parentWindow = updateContextWindow("aide", cwd, parent, "startup")!;
      const childWindow = updateContextWindow("aide", cwd, child, "startup")!;
      const content = "const value = 1;\n";
      for (const scope of [parent, child])
        recordFileRead("aide", cwd, "file.ts", { identity: scope, content });

      updateHookContextWindow(
        "aide",
        cwd,
        host,
        { session_id: "session", agent_id: "child" },
        "compact_pending",
      );
      expect(contextWindow("aide", cwd, child)?.status).toBe("pending");
      expect(getPreviousRead("aide", cwd, "file.ts", child)).toBeNull();
      expect(contextWindow("aide", cwd, parent)).toEqual(parentWindow);
      expect(getPreviousRead("aide", cwd, "file.ts", parent)).not.toBeNull();

      const completed = updateHookContextWindow(
        "aide",
        cwd,
        host,
        { session_id: "session", agent_id: "child" },
        "compact",
      )!;
      expect(completed.id).not.toBe(childWindow.id);
      expect(completed.continuity).toBe("reset");
      expect(getPreviousRead("aide", cwd, "file.ts", child)).toBeNull();
      expect(contextWindow("aide", cwd, parent)).toEqual(parentWindow);

      updateHookContextWindow(
        "aide",
        cwd,
        host,
        { session_id: "session" },
        "compact_pending",
      );
      expect(contextWindow("aide", cwd, parent)?.status).toBe("pending");
      expect(contextWindow("aide", cwd, child)).toEqual(completed);
    },
  );
  it("keeps large-file navigation advice quiet when code watching is disabled", () => {
    writeFileSync(join(cwd, "large.ts"), "const value = 1;\n".repeat(2000));
    vi.mocked(codeWatchEnabled).mockReturnValue(false);
    expect(
      checkContextGuard("Read", { file_path: "large.ts" }, cwd, "session"),
    ).toEqual({ shouldAdvise: false });
  });
  it.each([
    ["Read", "file_path"],
    ["read", "filePath"],
  ])(
    "offers file structure and batched source retrieval for %s",
    (tool, key) => {
      writeFileSync(join(cwd, "large.ts"), "const value = 1;\n".repeat(2000));
      const result = checkContextGuard(
        tool,
        { [key]: "large.ts" },
        cwd,
        "session",
      );
      expect(result.shouldAdvise).toBe(true);
      expect(result.advisory).toContain("code_symbols");
      expect(result.advisory).toContain("code_outline");
      expect(result.advisory).toContain("code_read_symbol");
      expect(result.advisory).toContain("symbols");
      expect(result.advisory).toContain("most of its contents");
    },
  );
  it("keeps direct reads of small source files quiet", () => {
    expect(
      checkContextGuard("Read", { file_path: "file.ts" }, cwd, "session"),
    ).toEqual({ shouldAdvise: false });
  });
  it.each([1, 99, 100, 200, 1000])(
    "does not advise an explicitly bounded read of %i lines",
    (limit) => {
      writeFileSync(join(cwd, "large.ts"), "const value = 1;\n".repeat(2000));
      expect(
        checkContextGuard(
          "Read",
          { file_path: "large.ts", offset: 1, limit },
          cwd,
          "session",
        ).shouldAdvise,
      ).toBe(false);
    },
  );
  it.each([
    { limit: 0 },
    { limit: -1 },
    { limit: "20" },
    { limit: 1.5 },
    { limit: NaN },
    { limit: Infinity },
    { offset: "50" },
    { offset: 2.5 },
  ])("does not interpret a malformed range as a targeted read: %j", (range) => {
    writeFileSync(join(cwd, "large.ts"), "const value = 1;\n".repeat(2000));
    expect(
      checkContextGuard(
        "Read",
        { file_path: "large.ts", ...range },
        cwd,
        "session",
      ).shouldAdvise,
    ).toBe(true);
  });
  it("does not treat an outline request as a successful delivered outline", () => {
    writeFileSync(join(cwd, "large.ts"), "const value = 1;\n".repeat(500));
    const session = cwd.split(/[\\/]/).pop()!;
    checkContextGuard(
      "mcp__aide__code_outline",
      { file: "large.ts" },
      cwd,
      session,
    );
    expect(
      checkContextGuard("Read", { file_path: "large.ts" }, cwd, session)
        .shouldAdvise,
    ).toBe(true);
  });
  it("requires a known window and complete identity, with no legacy fallback", () => {
    record();
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
    expect(
      updateContextWindow(
        "aide",
        cwd,
        { ...identity, sessionId: "" },
        "startup",
      ),
    ).toBeNull();
    updateContextWindow("aide", cwd, identity, "startup");
    record();
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).not.toBeNull();
    expect(getPreviousRead("aide", cwd, "file.ts")).toBeNull();
  });
  it("isolates projects, sessions, hosts and actors", () => {
    updateContextWindow("aide", cwd, identity, "startup");
    record();
    for (const other of [
      { ...identity, actorId: "child" },
      { ...identity, sessionId: "other" },
      { ...identity, host: "opencode" },
    ]) {
      updateContextWindow("aide", cwd, other, "startup");
      expect(getPreviousRead("aide", cwd, "file.ts", other)).toBeNull();
    }
    expect(contextWindow("aide", join(cwd, "other"), identity)).toBeNull();
  });
  it("retains verified rendered coverage only for matching source and context", () => {
    const scope = {
      host: "opencode",
      sessionId: "rendered",
      actorId: "rendered",
    };
    const digest = createHash("sha256")
      .update("const value = 1;\n")
      .digest("hex");
    updateContextWindow("aide", cwd, scope, "startup");
    recordFileRead("aide", cwd, "file.ts", {
      identity: scope,
      content: "formatted source",
      verifiedRenderedHash: digest,
    });
    expect(getPreviousRead("aide", cwd, "file.ts", scope)).not.toBeNull();
    updateContextWindow("aide", cwd, scope, "compact");
    expect(getPreviousRead("aide", cwd, "file.ts", scope)).toBeNull();
    writeFileSync(join(cwd, "file.ts"), "const value = 2;\n");
    recordFileRead("aide", cwd, "file.ts", {
      identity: scope,
      content: "formatted source",
      verifiedRenderedHash: digest,
    });
    expect(getPreviousRead("aide", cwd, "file.ts", scope)).toBeNull();
  });
  it("compares delivered content, refusing slices and changed source", () => {
    updateContextWindow("aide", cwd, identity, "startup");
    record("const value");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
    record();
    writeFileSync(join(cwd, "file.ts"), "const value = 2;\n");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
  });
  it("preserves coverage on cache expiry and known failed compaction, suspends pending, resets on completion", () => {
    const first = updateContextWindow("aide", cwd, identity, "startup")!;
    record();
    expect(
      updateContextWindow("aide", cwd, identity, "cache_expired")?.id,
    ).toBe(first.id);
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).not.toBeNull();
    updateContextWindow("aide", cwd, identity, "compact_pending");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
    updateContextWindow("aide", cwd, identity, "compact_failed");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).not.toBeNull();
    const next = updateContextWindow("aide", cwd, identity, "compact")!;
    expect(next.id).not.toBe(first.id);
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
  });
  it("does not reuse coverage after clear or an unverified resume", () => {
    updateContextWindow("aide", cwd, identity, "startup");
    record();
    updateContextWindow("aide", cwd, identity, "clear");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
    record();
    expect(
      updateContextWindow("aide", cwd, identity, "resume")?.continuity,
    ).toBe("unknown");
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).toBeNull();
    record();
    expect(getPreviousRead("aide", cwd, "file.ts", identity)).not.toBeNull();
  });
});
