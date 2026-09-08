import { beforeEach, describe, expect, it, vi } from "vitest";
vi.mock("child_process", () => ({ execFileSync: vi.fn() }));
vi.mock("../core/read-tracking.js", () => ({ recordFileRead: vi.fn() }));
import { execFileSync } from "child_process";
import { recordFileRead } from "../core/read-tracking.js";
import { recordToolEvent } from "../core/tool-observe.js";
import { createHash } from "crypto";

function recorded() {
  return vi.mocked(execFileSync).mock.calls.at(-1)?.[1] as string[];
}
beforeEach(() => vi.clearAllMocks());
describe("observed tool accounting", () => {
  it("observes aide MCP results under host identity with exact receipt evidence", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "mcp__aide__code_outline",
      toolInput: { file: "source.ts" },
      toolResponse: {
        content: [{ type: "text", text: "é" }],
        _meta: {
          "aide/retrieval": {
            version: 1,
            id: "receipt",
            tool: "code_outline",
            text_sha256: createHash("sha256").update("é").digest("hex"),
            references: [
              { file: "source.ts", sha256: "a".repeat(64), bytes: 900 },
            ],
          },
        },
      },
      host: "claude-code",
      sessionId: "s",
      invocationId: "call",
    });
    expect(recorded()).toEqual(
      expect.arrayContaining([
        "--name=code_outline",
        "--attr=observation_stage=host_result",
        "--attr=payload_bytes=2",
        "--attr=invocation_id=call",
        "--session=s",
        "--attr=retrieval_id=receipt",
        "--attr=retrieval_status=referenced",
      ]),
    );
    expect(recorded().some((arg) => arg.startsWith("--saved="))).toBe(false);
  });
  it("normalizes aliases and measures UTF-8 stdout and stderr together", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "exec_command",
      toolInput: { cmd: "test" },
      toolResponse: { stdout: "é", stderr: "oops" },
      sessionId: "s",
      host: "codex",
      invocationId: "call1",
    });
    expect(recorded()).toEqual(
      expect.arrayContaining([
        "--name=Bash",
        "--attr=raw_tool=exec_command",
        "--attr=payload_bytes=6",
        "--attr=command=test",
        "--attr=invocation_id=call1",
        "--attr=host=codex",
      ]),
    );
  });
  it("uses the returned slice, including nested text, rather than file size", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "read",
      toolInput: { filePath: "large.ts", offset: 100 },
      toolResponse: {
        content: [
          { type: "text", text: "abc" },
          { type: "text", text: "def" },
        ],
      },
    });
    expect(recorded()).toEqual(
      expect.arrayContaining([
        "--name=Read",
        "--file=large.ts",
        "--attr=payload_bytes=6",
      ]),
    );
  });
  it("keeps missing payload unknown and does not mark a failed read as consumed", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "Read",
      toolInput: { file_path: "missing", offset: 9999 },
      success: false,
    });
    expect(
      recorded().some(
        (a) =>
          a.startsWith("--tokens=") || a.startsWith("--attr=payload_bytes="),
      ),
    ).toBe(false);
    expect(recordFileRead).not.toHaveBeenCalled();
  });
  it("distinguishes known empty output from missing and separates generated arguments", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "write",
      toolInput: { content: "é" },
      toolResponse: "",
    });
    expect(recorded()).toEqual(
      expect.arrayContaining([
        "--attr=payload_bytes=0",
        "--attr=argument_bytes=2",
      ]),
    );
  });
  it("does not double-count alternate output wrappers or serialize opaque objects", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "Bash",
      toolResponse: { output: "abc", stdout: "abc" },
    });
    expect(recorded()).toContain("--attr=payload_bytes=3");
    recordToolEvent("aide", "/tmp", {
      toolName: "Read",
      toolResponse: { image: "opaque" },
    });
    expect(recorded().some((a) => a.startsWith("--attr=payload_bytes="))).toBe(
      false,
    );
  });
  it("records a known empty MCP content list but leaves opaque-only content unknown", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "aide_code_outline",
      toolResponse: { content: [] },
    });
    expect(recorded()).toContain("--attr=payload_bytes=0");
    recordToolEvent("aide", "/tmp", {
      toolName: "aide_code_outline",
      toolResponse: { content: [{ type: "image", data: "opaque" }] },
    });
    expect(
      recorded().some((arg) => arg.startsWith("--attr=payload_bytes=")),
    ).toBe(false);
  });
});
