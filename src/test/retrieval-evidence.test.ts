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
