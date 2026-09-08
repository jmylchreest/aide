import { describe, expect, it, vi } from "vitest";
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
import { execFileSync } from "child_process";
import { createHooks } from "../opencode/hooks.js";
import type { OpenCodeClient } from "../opencode/types.js";

describe("OpenCode MCP observations", () => {
  it.each([
    ["cat source.ts", 2, "failed"],
    ["rg absent source.ts", 1, "search"],
    ["rg absent source.ts", 2, "failed"],
    ["rg absent source.ts", "1", "unverified"],
  ])(
    "uses shell metadata exit status for %s / %s",
    async (command, exit, status) => {
      vi.mocked(execFileSync).mockClear();
      const hooks = await createHooks("/tmp", "/tmp", {} as OpenCodeClient);
      const output = {
        output: "",
        metadata: { exit, output: "a separate UI copy" },
      };
      await hooks["tool.execute.after"]!(
        {
          tool: "bash",
          sessionID: "session",
          callID: "call",
          args: { command },
        },
        output,
      );
      const event = vi
        .mocked(execFileSync)
        .mock.calls.map((call) => call[1] as string[])
        .find((args) => args?.includes("--name=Bash"));
      expect(event).toEqual(
        expect.arrayContaining([
          `--attr=retrieval_status=${status}`,
          "--attr=payload_bytes=0",
        ]),
      );
      expect(output.metadata).toEqual({ exit, output: "a separate UI copy" });
    },
  );
  it("observes content-block results without assuming a rendered output property", async () => {
    const hooks = await createHooks("/tmp", "/tmp", {} as OpenCodeClient);
    const output = { content: [{ type: "text", text: "é" }], isError: true };
    await hooks["tool.execute.after"]!(
      {
        tool: "aide_code_outline",
        sessionID: "session",
        callID: "call",
        args: { file: "source.ts" },
      },
      output,
    );
    const calls = vi
      .mocked(execFileSync)
      .mock.calls.map((call) => call[1] as string[]);
    const event = calls.find((args) => args?.includes("--name=code_outline"));
    expect(event).toEqual(
      expect.arrayContaining([
        "--attr=payload_bytes=2",
        "--attr=retrieval_status=failed",
        "--session=session",
        "--attr=invocation_id=call",
      ]),
    );
    expect(output).toEqual({
      content: [{ type: "text", text: "é" }],
      isError: true,
    });
  });
});
