import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
vi.mock("child_process", () => ({ execFileSync: vi.fn() }));
vi.mock("../core/read-tracking.js", () => ({ recordFileRead: vi.fn() }));
import { execFileSync } from "child_process";
import { recordFileRead } from "../core/read-tracking.js";
import { recordToolEvent } from "../core/tool-observe.js";
import { createHash } from "crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

const roots: string[] = [];
function sourceFixture() {
  const cwd = mkdtempSync(join(tmpdir(), "aide-accounting-"));
  roots.push(cwd);
  mkdirSync(join(cwd, "nested"));
  writeFileSync(join(cwd, "source.ts"), "first\né\nlast\n");
  writeFileSync(join(cwd, "nested", "source.ts"), "different\né\nsource\n");
  return cwd;
}
afterEach(() =>
  roots
    .splice(0)
    .forEach((root) => rmSync(root, { recursive: true, force: true })),
);

function recorded() {
  return vi.mocked(execFileSync).mock.calls.at(-1)?.[1] as string[];
}
beforeEach(() => vi.clearAllMocks());
describe("observed tool accounting", () => {
  it("keeps exact Codex payload bytes while missing shell workdir prevents root attribution", () => {
    const cwd = sourceFixture();
    for (const [cmd, output, method] of [
      ["cat source.ts", "first\né\nlast\n", "shell_cat"],
      ["sed -n '2,2p' source.ts", "é\n", "shell_sed"],
    ]) {
      recordToolEvent("aide", cwd, {
        toolName: "exec_command",
        toolInput: { cmd },
        toolResponse: { output, exit_code: 0 },
        host: "codex",
      });
      expect(recorded()).toEqual(
        expect.arrayContaining([
          `--attr=payload_bytes=${Buffer.byteLength(output)}`,
          `--attr=retrieval_method=${method}`,
          "--attr=retrieval_status=unverified",
          "--attr=retrieval_reason=unknown_shell_workdir",
        ]),
      );
      expect(
        recorded().some((arg) =>
          /^--attr=(retrieval_target|source_references|reference_kind|source_verification|delivered_start_line|delivered_end_line)=/.test(
            arg,
          ),
        ),
      ).toBe(false);
    }
  });
  it("uses explicit Codex workdir and absolute shell paths for source attribution", () => {
    const cwd = sourceFixture();
    for (const toolInput of [
      { cmd: "sed -n '2,2p' source.ts", workdir: "nested" },
      { cmd: "sed -n '2,2p' source.ts", cwd: join(cwd, "nested") },
      { cmd: `sed -n '2,2p' '${join(cwd, "nested", "source.ts")}'` },
    ]) {
      recordToolEvent("aide", cwd, {
        toolName: "exec_command",
        toolInput,
        toolResponse: { output: "é\n", exit_code: 0 },
        host: "codex",
      });
      expect(recorded()).toEqual(
        expect.arrayContaining([
          "--attr=payload_bytes=3",
          "--attr=retrieval_status=range",
          `--attr=retrieval_target=${join("nested", "source.ts")}`,
          `--attr=source_references=${JSON.stringify([{ file: join("nested", "source.ts"), sha256: createHash("sha256").update("different\né\nsource\n").digest("hex"), bytes: Buffer.byteLength("different\né\nsource\n") }])}`,
        ]),
      );
    }
  });
  it.each(["claude-code", "opencode", undefined])(
    "retains implicit project workdir for host %s",
    (host) => {
      recordToolEvent("aide", sourceFixture(), {
        toolName: "bash",
        toolInput: { command: "cat source.ts" },
        toolResponse: "first\né\nlast\n",
        host,
      });
      expect(recorded()).toContain("--attr=retrieval_status=full_file");
      expect(recorded()).toContain("--attr=retrieval_target=source.ts");
    },
  );
  it("keeps native Codex reads verifiable without shell workdir", () => {
    recordToolEvent("aide", sourceFixture(), {
      toolName: "read_file",
      toolInput: { file_path: "source.ts" },
      toolResponse: "first\né\nlast\n",
      host: "codex",
    });
    expect(recorded()).toContain("--attr=retrieval_status=full_file");
    expect(recorded()).toContain("--attr=retrieval_method=native_read");
  });
  it("preserves hook exit codes independently of the returned text", () => {
    recordToolEvent("aide", "/tmp", {
      toolName: "Bash",
      toolInput: { command: "cat source.ts" },
      toolResponse: "partial text",
      exitCode: 2,
    });
    expect(recorded()).toContain("--attr=retrieval_status=failed");
    expect(recorded()).toContain("--attr=payload_bytes=12");
    recordToolEvent("aide", "/tmp", {
      toolName: "Bash",
      toolInput: { command: "rg absent source.ts" },
      toolResponse: "",
      exitCode: 1,
    });
    expect(recorded()).toContain("--attr=retrieval_status=search");
    expect(recorded()).toContain("--attr=payload_bytes=0");
    expect(recorded().some((arg) => arg.startsWith("--error="))).toBe(false);
  });
  it.each(["claude-code", "codex"])(
    "observes aide MCP results under %s identity with exact receipt evidence",
    (host) => {
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
        host,
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
    },
  );
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
