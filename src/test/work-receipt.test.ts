import { beforeEach, describe, expect, it, vi } from "vitest";
import { createHash } from "crypto";
vi.mock("child_process", () => ({ execFileSync: vi.fn(() => "") }));
vi.mock("../core/context-window.js", () => ({ contextWindow: () => null }));
vi.mock("../core/read-tracking.js", () => ({ recordFileRead: vi.fn() }));
import { execFileSync } from "child_process";
import { recordToolEvent } from "../core/tool-observe.js";

const digest = (s: string) => createHash("sha256").update(s).digest("hex");
function response(text = "é", overrides = {}) {
  return {
    content: [{ type: "text", text }],
    _meta: {
      "aide/work": {
        version: 1,
        id: "work-123",
        tool: "code_search",
        text_sha256: digest(text),
        ...overrides,
      },
    },
  };
}
function record(
  toolResponse: unknown,
  toolName = "mcp__aide__code_search",
  host = "codex",
) {
  recordToolEvent("aide", "/project", {
    toolName,
    toolResponse,
    host,
    sessionId: "session",
    actorId: "actor",
    invocationId: "call",
  });
  return vi.mocked(execFileSync).mock.calls.at(-1)?.[1] as string[] | undefined;
}
beforeEach(() => vi.clearAllMocks());
describe("host work receipt evidence", () => {
  it.each([
    ["claude-code", "mcp__plugin_aide_aide__code_search"],
    ["codex", "mcp__aide__code_search"],
    ["opencode", "aide_code_search"],
  ])("preserves exact MCP work identity for %s", (host, tool) => {
    const payload = response();
    expect(record(payload, tool, host)).toEqual(
      expect.arrayContaining([
        "--name=code_search",
        "--attr=observation_stage=host_result",
        "--attr=work_receipt_version=1",
        "--attr=work_id=work-123",
        `--attr=work_text_sha256=${digest("é")}`,
        "--attr=payload_bytes=2",
        `--attr=host=${host}`,
        "--session=session",
        "--attr=actor_id=actor",
        "--attr=invocation_id=call",
      ]),
    );
    expect(payload).toEqual(response());
  });
  it("keeps empty text and failed work attributable without asserting success", () => {
    const args = record({ ...response(""), isError: true });
    expect(args).toContain("--attr=payload_bytes=0");
    expect(args).toContain("--attr=work_id=work-123");
    expect(args).toContain("--error=tool reported failure");
    expect(args?.some((a) => a.startsWith("--attr=work_outcome="))).toBe(false);
  });
  it.each([
    response("é", { version: 2 }),
    response("é", { id: "" }),
    response("é", { id: "x".repeat(129) }),
    response("é", { tool: "memory_list" }),
    response("é", { text_sha256: digest("different") }),
    { ...response(), output: "formatted by host" },
    { content: [{ type: "text", text: "é" }] },
    { ...response(), content: [{ type: "image", data: "opaque" }] },
  ])(
    "retains observed calls without inventing a join for unsupported evidence %#",
    (payload) => {
      const args = record(payload);
      expect(args).toContain("--name=code_search");
      expect(args?.some((a) => a.startsWith("--attr=work_id="))).toBe(false);
    },
  );
  it("does not treat another tool namespace or a native call as aide work", () => {
    expect(record(response(), "mcp__other__code_search")).toBeUndefined();
    const args = record(response("é", { tool: "Bash" }), "Bash");
    expect(args).toContain("--name=Bash");
    expect(args?.some((a) => a.startsWith("--attr=work_id="))).toBe(false);
  });
  it("preserves retrieval evidence alongside the independent work receipt", () => {
    const payload = response("outline", { tool: "code_outline" });
    Object.assign(payload._meta, {
      "aide/retrieval": {
        version: 1,
        id: "retrieval-456",
        tool: "code_outline",
        text_sha256: digest("outline"),
        references: [{ file: "source.ts", sha256: "a".repeat(64), bytes: 900 }],
      },
    });
    const args = record(payload, "mcp__aide__code_outline");
    expect(args).toContain("--attr=work_id=work-123");
    expect(args).toContain("--attr=retrieval_id=retrieval-456");
  });
});
