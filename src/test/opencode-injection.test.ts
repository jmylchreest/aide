import { describe, expect, it, vi } from "vitest";
vi.mock("child_process", () => ({ execFileSync: vi.fn(() => "") }));
vi.mock("../core/mcp-sync.js", () => ({ syncMcpServers: vi.fn() }));
vi.mock("../core/session-init.js", async (original) => ({
  ...(await original<object>()),
  ensureDirectories: vi.fn(),
  loadConfig: () => ({}),
  cleanupStaleStateFiles: vi.fn(),
  resetHudState: vi.fn(),
  runSessionInit: () => ({}),
  initializeSession: () => ({ sessionId: "s", cwd: "/tmp" }),
  buildWelcomeContext: () => "<context>Résumé 🦊</context>",
  getProjectName: () => "test",
}));
vi.mock("../core/aide-client.js", async (original) => ({
  ...(await original<object>()),
  findAideBinary: () => "aide",
  getState: () => null,
  setState: vi.fn(),
}));
vi.mock("../core/skill-matcher.js", () => ({
  discoverSkills: () => [{ name: "test", content: "é🦊" }],
  matchSkills: () => [{ name: "test", content: "é🦊" }],
  formatSkillsContext: () => "<skills>é🦊</skills>",
}));
import { execFileSync } from "child_process";
import { createHooks } from "../opencode/hooks.js";
import type { OpenCodeClient } from "../opencode/types.js";

function recorded() {
  return vi.mocked(execFileSync).mock.calls.flatMap((call) => {
    const args = call[1] as string[];
    if (!args?.includes("--stdin")) return [];
    return String((call[2] as { input: string }).input)
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line));
  });
}

describe("OpenCode prepared context observations", () => {
  it("measures the context appended to compaction without including existing context", async () => {
    const hooks = await createHooks("/tmp", "/tmp", {} as OpenCodeClient);
    await hooks.event!({
      event: { type: "session.created", properties: { info: { id: "s" } } },
    });
    vi.mocked(execFileSync).mockClear();
    const output = { context: ["existing context"] };
    await hooks["experimental.session.compacting"]!({ sessionID: "s" }, output);
    const events = recorded();
    expect(events).toHaveLength(1);
    expect(events[0]).toMatchObject({
      name: "opencode-compaction",
      session: "s",
      attrs: {
        payload_bytes: String(Buffer.byteLength(output.context[1], "utf8")),
        content_boundary: "appended_text",
      },
    });
  });
  it("records only aide-added strings at command and system boundaries, including repeat submissions", async () => {
    const hooks = await createHooks("/tmp", "/tmp", {} as OpenCodeClient);
    vi.mocked(execFileSync).mockClear();
    const parts = { parts: [{ type: "text", text: "existing user text" }] };
    await hooks["command.execute.before"]!(
      { command: "aide:test", sessionID: "s", arguments: "please test" },
      parts,
    );
    const system = { system: ["existing system text"] };
    await hooks["experimental.chat.system.transform"]!(
      { sessionID: "s", model: { providerID: "p", modelID: "m" } },
      system,
    );
    const repeated = { system: [] as string[] };
    await hooks["experimental.chat.system.transform"]!(
      { sessionID: "s", model: { providerID: "p", modelID: "m" } },
      repeated,
    );
    const events = recorded();
    expect(events).toHaveLength(3);
    const texts = [parts.parts[1].text, system.system[1], repeated.system[0]];
    for (let i = 0; i < events.length; i++) {
      expect(events[i]).toMatchObject({
        kind: "injection",
        session: "s",
        attrs: {
          host: "opencode",
          observation_stage: "aide_context",
          content_boundary: "appended_text",
          payload_bytes: String(Buffer.byteLength(texts[i], "utf8")),
        },
      });
      expect(events[i].tokens).toBeUndefined();
    }
  });
  it("does not invent a session or emit a zero measurement when no context was added", async () => {
    const hooks = await createHooks("/tmp", "/tmp", {} as OpenCodeClient);
    vi.mocked(execFileSync).mockClear();
    await hooks["experimental.chat.system.transform"]!(
      { model: { providerID: "p", modelID: "m" } },
      { system: ["unrelated"] },
    );
    expect(recorded()).toEqual([]);
    await hooks["command.execute.before"]!(
      { command: "aide:test", sessionID: "s", arguments: "" },
      { parts: [] },
    );
    vi.mocked(execFileSync).mockClear();
    await hooks["experimental.chat.system.transform"]!(
      { model: { providerID: "p", modelID: "m" } },
      { system: [] },
    );
    expect(recorded()).toHaveLength(1);
    expect(recorded()[0].session).toBeUndefined();
  });
});
