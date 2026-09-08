import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  readStdin: vi.fn(),
  recordToolEvent: vi.fn(),
  emitHookResult: vi.fn(),
  platform: "claude-code",
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: mocks.readStdin,
  emitHookResult: mocks.emitHookResult,
  installHookSafetyNet: vi.fn(),
  findAideBinary: () => "aide",
  detectPlatform: () => mocks.platform,
}));
vi.mock("../core/tool-observe.js", () => ({
  recordToolEvent: mocks.recordToolEvent,
}));
vi.mock("../lib/logger.js", () => ({ debug: vi.fn() }));

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
  mocks.platform = "claude-code";
});
describe("tool observe hook adapter", () => {
  it.each(["claude-code", "codex"])(
    "preserves work metadata and actual identity for %s",
    async (host) => {
      mocks.platform = host;
      const response = {
        content: [{ type: "text", text: "result" }],
        _meta: {
          "aide/work": {
            version: 1,
            id: "receipt",
            tool: "code_search",
            text_sha256: "digest",
          },
        },
      };
      mocks.readStdin.mockResolvedValue(
        JSON.stringify({
          hook_event_name: "PostToolUse",
          cwd: "/project",
          session_id: "s",
          agent_id: "a",
          tool_use_id: "call",
          tool_name: "mcp__aide__code_search",
          tool_response: response,
        }),
      );
      await import("../hooks/tool-observe.js");
      await vi.waitFor(() => expect(mocks.emitHookResult).toHaveBeenCalled());
      expect(mocks.recordToolEvent).toHaveBeenCalledWith(
        "aide",
        "/project",
        expect.objectContaining({
          host,
          sessionId: "s",
          actorId: "a",
          invocationId: "call",
          toolResponse: response,
        }),
      );
    },
  );
  it.each(["PostToolUse", "PostToolUseFailure"])(
    "passes top-level exit code and unchanged payload from %s",
    async (event) => {
      const response = { stdout: "partial", stderr: "error" };
      mocks.readStdin.mockResolvedValue(
        JSON.stringify({
          hook_event_name: event,
          cwd: "/project",
          session_id: "session",
          tool_name: "Bash",
          tool_use_id: "call",
          tool_input: { command: "cat source.ts" },
          tool_response: response,
          exit_code: 2,
        }),
      );
      await import("../hooks/tool-observe.js");
      await vi.waitFor(() => expect(mocks.emitHookResult).toHaveBeenCalled());
      expect(mocks.recordToolEvent).toHaveBeenCalledWith(
        "aide",
        "/project",
        expect.objectContaining({
          exitCode: 2,
          toolResponse: response,
          invocationId: "call",
          sessionId: "session",
          success: event === "PostToolUseFailure" ? false : undefined,
        }),
      );
    },
  );
});
