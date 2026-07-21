/**
 * Golden gitdir-shape table (testdata/gitdir-shapes.json) run against the
 * TS resolver mirror — the same table the Go side runs in
 * aide/cmd/aide/gitdir_shapes_test.go, so the two implementations cannot
 * drift on gitdir classification. TS asserts anchoring only (the TS
 * resolver exposes no provenance surface).
 *
 * Run with: npx vitest run src/test/gitdir-shapes.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  mkdtempSync,
  rmSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
  realpathSync,
} from "fs";
import { join, dirname } from "path";
import { tmpdir } from "os";
import { fileURLToPath } from "url";

let tempHome = "";

vi.mock("os", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("os");
  return {
    ...actual,
    homedir: () => tempHome,
  };
});

interface ShapeCase {
  name: string;
  gitdirDirs: string[];
  gitdirTarget: string | null;
  cwdSuffix?: string;
  expectAnchor: "checkout" | "super";
}

const tableFile = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "testdata",
  "gitdir-shapes.json",
);
const cases = (
  JSON.parse(readFileSync(tableFile, "utf-8")) as { cases: ShapeCase[] }
).cases;

describe("gitdir shapes golden table", () => {
  let tmp: string;

  beforeEach(() => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), "aide-shapes-")));
    tempHome = realpathSync(mkdtempSync(join(tmpdir(), "aide-shapes-home-")));
    delete process.env.AIDE_PROJECT_ROOT;
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
    delete process.env.AIDE_PROJECT_ROOT;
  });

  for (const tc of cases) {
    it(tc.name, async () => {
      const { findProjectRoot } = await import("../lib/project-root.js");

      const superRepo = join(tmp, "super");
      for (const d of tc.gitdirDirs) {
        mkdirSync(join(superRepo, ...d.split("/")), { recursive: true });
      }
      const checkout = join(tmp, "checkout");
      const cwd = tc.cwdSuffix
        ? join(checkout, ...tc.cwdSuffix.split("/"))
        : checkout;
      mkdirSync(cwd, { recursive: true });

      const gitPath = join(checkout, ".git");
      if (tc.gitdirTarget === null) {
        mkdirSync(gitPath, { recursive: true });
      } else if (tc.gitdirTarget === "GARBAGE") {
        writeFileSync(gitPath, "not a gitdir pointer\n");
      } else {
        writeFileSync(
          gitPath,
          `gitdir: ${join(superRepo, ...tc.gitdirTarget.split("/"))}\n`,
        );
      }

      const result = findProjectRoot(cwd);
      const wantRoot = tc.expectAnchor === "super" ? superRepo : checkout;
      expect(result.root).toBe(wantRoot);
      expect(result.hasMarker).toBe(true);
    });
  }
});
