import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
vi.mock("../core/aide-client.js", () => ({
  setState: vi.fn(),
  getState: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", () => ({ codeWatchEnabled: () => true }));
import { setState, getState } from "../core/aide-client.js";
import { contextWindow, updateContextWindow } from "../core/context-window.js";
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
