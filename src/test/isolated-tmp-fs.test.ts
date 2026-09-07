import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { isolatedTmpFs } from "./helpers/isolated-tmp-fs.js";

describe("temporary fixture filesystem isolation", () => {
  let outer: string;
  let tempRoot: string;

  beforeEach(() => {
    outer = mkdtempSync(join(tmpdir(), "aide-fs-isolation-"));
    tempRoot = join(outer, "tmp");
    mkdirSync(tempRoot);
  });

  afterEach(() => {
    rmSync(outer, { recursive: true, force: true });
  });

  it("hides ancestor markers while keeping fixture markers visible", () => {
    const fixture = join(tempRoot, "project");
    for (const marker of [".aide", ".git", ".hg", ".svn", ".bzr", ".fossil"]) {
      for (const dir of [outer, tempRoot, fixture]) {
        mkdirSync(join(dir, marker), { recursive: true });
      }
    }

    const fs = isolatedTmpFs(tempRoot);
    for (const marker of [".aide", ".git", ".hg", ".svn", ".bzr", ".fossil"]) {
      expect(fs.existsSync(join(outer, marker))).toBe(false);
      expect(fs.existsSync(join(tempRoot, marker))).toBe(false);
      expect(fs.existsSync(join(fixture, marker))).toBe(true);
    }
  });

  it("preserves ordinary ancestor files and real fixture writes", () => {
    const existing = join(outer, "config.json");
    writeFileSync(existing, "{}");
    const fs = isolatedTmpFs(tempRoot);
    expect(fs.existsSync(existing)).toBe(true);

    const created = join(tempRoot, "written.txt");
    fs.writeFileSync(created, "fixture");
    expect(fs.existsSync(created)).toBe(true);
    expect(fs.readFileSync(created, "utf8")).toBe("fixture");
    expect(fs.existsSync(join(tempRoot, "missing"))).toBe(false);
  });
});
