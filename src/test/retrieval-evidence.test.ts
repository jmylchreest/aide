import { afterEach, describe, expect, it } from "vitest";
import { createHash } from "crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import {
  retrievalEvidence,
  aideRetrievalTool,
} from "../core/retrieval-evidence.js";

const roots: string[] = [];
function fixture() {
  const cwd = mkdtempSync(join(tmpdir(), "aide-retrieval-"));
  roots.push(cwd);
  writeFileSync(join(cwd, "source.ts"), "first\né\nlast\n");
  return cwd;
}
afterEach(() =>
  roots
    .splice(0)
    .forEach((root) => rmSync(root, { recursive: true, force: true })),
);
const hash = (text: string) => createHash("sha256").update(text).digest("hex");

describe("retrieval evidence", () => {
  describe("shell execution directory evidence", () => {
    const options = { requireShellWorkdir: true };
    it.each([
      ["cat source.ts", "first\né\nlast\n", "first\né\nlast\n"],
      ["sed -n '2,2p' source.ts", "é\n", "different\né\nsource\n"],
    ])(
      "cannot certify coincidentally matching root bytes for %s",
      (cmd, text, nested) => {
        const cwd = fixture();
        mkdirSync(join(cwd, "nested"));
        writeFileSync(join(cwd, "nested", "source.ts"), nested);
        for (const workdir of [
          undefined,
          null,
          "",
          " ",
          42,
          {},
          "bad\0dir",
          "missing",
          "source.ts",
          "missing/..",
        ]) {
          const evidence = retrievalEvidence(
            cwd,
            "Bash",
            { cmd, workdir },
            text,
            { exit_code: 0 },
            false,
            undefined,
            options,
          );
          expect(evidence).toEqual({
            retrieval_method: cmd.startsWith("cat") ? "shell_cat" : "shell_sed",
            retrieval_status: "unverified",
            retrieval_reason: "unknown_shell_workdir",
          });
        }
      },
    );
    it("resolves valid workdir and cwd evidence to the nested source", () => {
      const cwd = fixture();
      mkdirSync(join(cwd, "nested"));
      const nested = "different\né\nsource\n";
      writeFileSync(join(cwd, "nested", "source.ts"), nested);
      for (const args of [
        { workdir: "nested" },
        { workdir: join(cwd, "nested") },
        { cwd: "nested" },
      ]) {
        const evidence = retrievalEvidence(
          cwd,
          "Bash",
          { cmd: "sed -n '2,2p' source.ts", ...args },
          "é\n",
          { exit_code: 0 },
          false,
          undefined,
          options,
        );
        expect(evidence).toMatchObject({
          retrieval_target: join("nested", "source.ts"),
          retrieval_status: "range",
          delivered_start_line: "2",
          delivered_end_line: "2",
        });
        expect(JSON.parse(evidence.source_references)).toEqual([
          {
            file: join("nested", "source.ts"),
            sha256: hash(nested),
            bytes: Buffer.byteLength(nested),
          },
        ]);
      }
    });
    it("allows absolute shell targets without workdir evidence", () => {
      const cwd = fixture();
      for (const workdir of [undefined, "", 42]) {
        const evidence = retrievalEvidence(
          cwd,
          "Bash",
          { cmd: `cat '${join(cwd, "source.ts")}'`, workdir },
          "first\né\nlast\n",
          undefined,
          false,
          undefined,
          options,
        );
        expect(evidence).toMatchObject({
          retrieval_status: "full_file",
          retrieval_target: "source.ts",
        });
        expect(evidence.source_references).toBeDefined();
        expect(evidence.retrieval_reason).toBeUndefined();
      }
    });
    it.each([
      ["cat source.ts", { exit_code: 2 }, false, "failed", "shell_cat"],
      ["cat source.ts", { exit_code: 0 }, true, "failed", "shell_cat"],
      [
        "cat source.ts",
        { session_id: 12, exit_code: null },
        false,
        "pending",
        "shell_cat",
      ],
      [
        "rg absent source.ts",
        { exit_code: 1 },
        false,
        "search",
        "shell_search",
      ],
      [
        "rg absent source.ts",
        { exit_code: 2 },
        false,
        "failed",
        "shell_search",
      ],
      [
        "rg absent source.ts",
        { exit_code: "0" },
        false,
        "unverified",
        "shell_search",
      ],
    ])(
      "retains shell status without directory evidence: %s %j",
      (cmd, response, failed, status, method) => {
        expect(
          retrievalEvidence(
            "/tmp",
            "Bash",
            { cmd },
            "",
            response,
            failed,
            undefined,
            options,
          ),
        ).toEqual({
          retrieval_method: method,
          retrieval_status: status,
          retrieval_reason: "unknown_shell_workdir",
        });
      },
    );
    it("does not infer an unrequested range even with a known workdir", () => {
      const evidence = retrievalEvidence(
        fixture(),
        "Bash",
        { cmd: "cat source.ts", workdir: "." },
        "é\n",
        undefined,
        false,
        undefined,
        options,
      );
      expect(evidence.retrieval_status).toBe("unverified");
      expect(evidence.source_references).toBeUndefined();
      expect(evidence.delivered_start_line).toBeUndefined();
    });
  });
  it("keeps completed no-match searches as searches, without overriding explicit failures", () => {
    for (const command of [
      "rg -n absent source.ts",
      "grep absent source.ts",
      "rtk proxy rg absent source.ts",
    ]) {
      for (const field of ["exit_code", "exitCode"]) {
        const result = { output: "", [field]: 1 };
        expect(
          retrievalEvidence("/tmp", "Bash", { command }, "", result, false),
        ).toMatchObject({
          retrieval_status: "search",
          retrieval_method: "shell_search",
        });
        expect(
          retrievalEvidence("/tmp", "Bash", { command }, "", result, true)
            .retrieval_status,
        ).toBe("failed");
        expect(
          retrievalEvidence(
            "/tmp",
            "Bash",
            { command },
            "",
            { ...result, interrupted: true },
            false,
          ).retrieval_status,
        ).toBe("failed");
        expect(
          retrievalEvidence(
            "/tmp",
            "Bash",
            { command },
            "",
            { ...result, [field]: 2 },
            false,
          ).retrieval_status,
        ).toBe("failed");
      }
    }
    expect(
      retrievalEvidence(
        "/tmp",
        "Bash",
        { cmd: "cat source.ts" },
        "",
        { exit_code: 1 },
        false,
      ).retrieval_status,
    ).toBe("failed");
    expect(
      retrievalEvidence(
        "/tmp",
        "Bash",
        { cmd: "rg absent source.ts | cat" },
        "",
        { exit_code: 1 },
        false,
      ).retrieval_status,
    ).toBe("unclassified_shell");
  });
  it("does not certify a full read from malformed or conflicting exit codes", () => {
    const cwd = fixture();
    for (const response of [
      { exit_code: "0" },
      { exit_code: null },
      { exit_code: 0.5 },
      { exit_code: 0, exitCode: 2 },
    ]) {
      const result = retrievalEvidence(
        cwd,
        "Bash",
        { cmd: "cat source.ts" },
        "first\né\nlast\n",
        response,
        false,
      );
      expect(result.source_references).toBeUndefined();
      expect(result.retrieval_status).not.toBe("full_file");
    }
  });
  it("keeps an explicit MCP failure target relative to the project for window attribution", () => {
    const cwd = fixture();
    const evidence = retrievalEvidence(
      cwd,
      "code_read_symbol",
      { file: join(cwd, "source.ts") },
      "Missing symbol",
      undefined,
      true,
    );
    expect(evidence.retrieval_target).toBe("source.ts");
    expect(evidence.retrieval_status).toBe("failed");
  });
  it("respects an explicit shell working directory instead of attributing the root file", () => {
    const cwd = fixture();
    mkdirSync(join(cwd, "nested"));
    writeFileSync(join(cwd, "nested", "source.ts"), "nested\n");
    const evidence = retrievalEvidence(
      cwd,
      "Bash",
      { cmd: "cat source.ts", workdir: "nested" },
      "nested\n",
      undefined,
      false,
    );
    expect(evidence.retrieval_status).toBe("full_file");
    expect(JSON.parse(evidence.source_references)[0].file).toBe(
      join("nested", "source.ts"),
    );
  });
  it("recognises aide tool namespaces without claiming unrelated tools", () => {
    for (const name of [
      "mcp__aide__code_outline",
      "mcp__plugin_aide_aide__code_outline",
      "aide_code_outline",
    ])
      expect(aideRetrievalTool(name)).toBe("code_outline");
    for (const name of [
      "mcp__other__code_outline",
      "my_aide_code_outline",
      "code_outline",
    ])
      expect(aideRetrievalTool(name)).toBeUndefined();
  });
  it("binds receipt references to exact returned text, never to current disk", () => {
    const refs = [{ file: "source.ts", sha256: hash("old source"), bytes: 10 }];
    const payload = {
      content: [{ type: "text", text: "outline" }],
      _meta: {
        "aide/retrieval": {
          version: 1,
          id: "receipt",
          tool: "code_outline",
          text_sha256: hash("outline"),
          references: refs,
        },
      },
    };
    const get = (text: string, response: unknown = payload) =>
      retrievalEvidence(
        "/tmp",
        "code_outline",
        { file: "source.ts" },
        text,
        response,
        false,
      );
    expect(get("outline")).toMatchObject({
      retrieval_status: "referenced",
      source_references: JSON.stringify(refs),
      retrieval_id: "receipt",
    });
    expect(get("truncated").source_references).toBeUndefined();
    expect(
      get("outline", { ...payload, _meta: undefined }).retrieval_status,
    ).toBe("unverified");
    expect(
      get("outline", {
        ...payload,
        _meta: {
          "aide/retrieval": {
            ...payload._meta["aide/retrieval"],
            references: [...refs, ...refs],
          },
        },
      }).source_references,
    ).toBeUndefined();
  });
  describe("OpenCode 1.18 native read rendering", () => {
    function response(cwd: string, start = 1, end = 3) {
      const lines = ["first", "é", "last"].slice(start - 1, end);
      const path = join(cwd, "source.ts");
      const truncated = end < 3;
      const footer = truncated
        ? `(Showing lines ${start}-${end} of 3. Use offset=${end + 1} to continue.)`
        : "(End of file - total 3 lines)";
      return {
        output: `<path>${path}</path>\n<type>file</type>\n<content>\n${lines.map((line, index) => `${start + index}: ${line}`).join("\n")}\n\n${footer}\n</content>`,
        metadata: {
          truncated,
          display: {
            type: "file",
            path,
            text: lines.join("\n"),
            lineStart: start,
            lineEnd: end,
            totalLines: 3,
            truncated,
          },
        },
      };
    }
    it.each(["first\né\nlast\n", "first\né\nlast", "first\r\né\r\nlast\r\n"])(
      "verifies rendered full source without claiming byte-identical delivery (%j)",
      (source) => {
        const cwd = fixture();
        writeFileSync(join(cwd, "source.ts"), source);
        const result = response(cwd);
        const evidence = retrievalEvidence(
          cwd,
          "Read",
          { filePath: "source.ts" },
          result.output,
          result,
          false,
        );
        expect(evidence).toMatchObject({
          retrieval_status: "full_file",
          source_verification: "current_rendered_file_match",
        });
        expect(JSON.parse(evidence.source_references)).toEqual([
          {
            file: "source.ts",
            sha256: hash(source),
            bytes: Buffer.byteLength(source),
          },
        ]);
      },
    );
    it.each([
      [2, 2],
      [2, 3],
    ])("verifies only delivered lines %i-%i", (start, end) => {
      const cwd = fixture();
      const result = response(cwd, start, end);
      expect(
        retrievalEvidence(
          cwd,
          "Read",
          { filePath: "source.ts", offset: start, limit: end - start + 1 },
          result.output,
          result,
          false,
        ),
      ).toMatchObject({
        retrieval_status: "range",
        source_verification: "current_rendered_range_match",
        delivered_start_line: String(start),
        delivered_end_line: String(end),
      });
    });
    it("refuses truncated, altered or conflicting rendering and metadata", () => {
      const cwd = fixture();
      const base = response(cwd);
      const bad = [
        { ...base, metadata: undefined },
        { ...base, output: base.output.replace("2: é", "2: changed") },
        { ...base, output: base.output.replace("2: é", "3: é") },
        {
          ...base,
          output: base.output.replace(
            "last",
            "la... (line truncated to 2000 chars)",
          ),
        },
        { ...base, output: base.output + " unexpected tail" },
        { ...base, metadata: { ...base.metadata, truncated: true } },
        ...[
          { text: "first\nchanged\nlast" },
          { path: join(cwd, "other.ts") },
          { lineStart: 2 },
          { lineEnd: 2 },
          { lineEnd: "3" },
          { totalLines: 4 },
          { truncated: true },
          { type: "directory" },
        ].map((display) => ({
          ...base,
          metadata: {
            ...base.metadata,
            display: { ...base.metadata.display, ...display },
          },
        })),
      ];
      for (const result of bad) {
        const evidence = retrievalEvidence(
          cwd,
          "Read",
          { filePath: "source.ts" },
          result.output,
          result,
          false,
        );
        expect(evidence.retrieval_status).toBe("unverified");
        expect(evidence.source_references).toBeUndefined();
      }
      for (const bounds of [
        { offset: 2 },
        { offset: "1" },
        { offset: null },
        { limit: 2 },
        { limit: 0 },
      ]) {
        expect(
          retrievalEvidence(
            cwd,
            "Read",
            { filePath: "source.ts", ...bounds },
            base.output,
            base,
            false,
          ).source_references,
        ).toBeUndefined();
      }
      expect(
        retrievalEvidence(
          cwd,
          "Read",
          { filePath: "source.ts" },
          base.output,
          base,
          true,
        ).retrieval_status,
      ).toBe("failed");
      writeFileSync(join(cwd, "source.ts"), "changed\né\nlast\n");
      expect(
        retrievalEvidence(
          cwd,
          "Read",
          { filePath: "source.ts" },
          base.output,
          base,
          false,
        ).source_references,
      ).toBeUndefined();
    });
  });
  it("records exact full source and verifies a requested slice against actual bytes", () => {
    const cwd = fixture();
    const full = retrievalEvidence(
      cwd,
      "Read",
      { file_path: "source.ts" },
      "first\né\nlast\n",
      undefined,
      false,
    );
    expect(full).toMatchObject({
      retrieval_status: "full_file",
      retrieval_method: "native_read",
    });
    expect(JSON.parse(full.source_references)[0]).toMatchObject({
      file: "source.ts",
      bytes: 14,
    });
    expect(
      retrievalEvidence(
        cwd,
        "Read",
        { file_path: "source.ts", offset: 2, limit: 1 },
        "é\n",
        undefined,
        false,
      ),
    ).toMatchObject({
      retrieval_status: "range",
      delivered_start_line: "2",
      delivered_end_line: "2",
    });
    expect(
      retrievalEvidence(
        cwd,
        "Read",
        { file_path: "source.ts", offset: 2, limit: 100 },
        "é",
        undefined,
        false,
      ).retrieval_status,
    ).toBe("unverified");
    expect(
      retrievalEvidence(
        cwd,
        "Read",
        { file_path: "source.ts" },
        "first\né\nlast\n",
        undefined,
        true,
      ).retrieval_status,
    ).toBe("failed");
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { cmd: "cat source.ts" },
        "first\né\nlast\n",
        { exit_code: 1 },
        false,
      ).retrieval_status,
    ).toBe("failed");
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { cmd: "cat source.ts" },
        "first\né\nlast\n",
        { session_id: 123, exit_code: null },
        false,
      ).retrieval_status,
    ).toBe("pending");
  });
  it("recognises simple shell reads and searches while refusing compound shell guesses", () => {
    const cwd = fixture();
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { cmd: "rtk proxy cat -- 'source.ts'" },
        "first\né\nlast\n",
        undefined,
        false,
      ),
    ).toMatchObject({
      retrieval_status: "full_file",
      retrieval_method: "shell_cat",
    });
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { command: "sed -n '2,2p' source.ts" },
        "é\n",
        undefined,
        false,
      ),
    ).toMatchObject({
      retrieval_status: "range",
      retrieval_method: "shell_sed",
    });
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { command: "rg -n 'first' source.ts" },
        "1:first\n",
        undefined,
        false,
      ),
    ).toMatchObject({
      retrieval_status: "search",
      retrieval_method: "shell_search",
    });
    for (const command of [
      "cat source.ts | head",
      "cd elsewhere && cat source.ts",
      "cat $(echo source.ts)",
      "cat *.ts",
      "cat a.ts b.ts",
      "cat source.ts >copy",
      "cat -n source.ts",
      "cat ../source.ts",
    ])
      expect(
        retrievalEvidence(
          cwd,
          "Bash",
          { command },
          "first\né\nlast\n",
          undefined,
          false,
        ).source_references,
      ).toBeUndefined();
    expect(
      retrievalEvidence(
        cwd,
        "Bash",
        { cmd: "cat source.ts" },
        undefined,
        undefined,
        false,
      ).retrieval_status,
    ).toBe("unverified");
  });
});
