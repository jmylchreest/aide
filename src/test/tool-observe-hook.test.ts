import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  readStdin: vi.fn(),
  recordToolEvent: vi.fn(),
  emitHookResult: vi.fn(),
}));
vi.mock("../lib/hook-utils.js", () => ({
  readStdin: mocks.readStdin,
  emitHookResult: mocks.emitHookResult,
  installHookSafetyNet: vi.fn(),
  findAideBinary: () => "aide",
  detectPlatform: () => "claude-code",
}));
vi.mock("../core/tool-observe.js", () => ({
  recordToolEvent: mocks.recordToolEvent,
}));
vi.mock("../lib/logger.js", () => ({ debug: vi.fn() }));

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
});
describe("tool observe hook adapter", () => {
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
