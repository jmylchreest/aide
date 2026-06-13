/**
 * Tests for project root resolution.
 *
 * Run with: npx vitest run src/test/project-root.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  mkdtempSync,
  rmSync,
  mkdirSync,
  writeFileSync,
  realpathSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

let tempHome = "";

vi.mock("os", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("os");
  return {
    ...actual,
    homedir: () => tempHome,
  };
});

describe("findProjectRoot", () => {
  let tmp: string;

  beforeEach(() => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), "aide-pr-")));
    tempHome = realpathSync(mkdtempSync(join(tmpdir(), "aide-home-")));
    delete process.env.AIDE_PROJECT_ROOT;
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
    delete process.env.AIDE_PROJECT_ROOT;
  });

  it("walks up from a subdir to a git repo root", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tmp, ".git"), { recursive: true });
    const sub = join(tmp, "models", "skadis-container");
    mkdirSync(sub, { recursive: true });

    const result = findProjectRoot(sub);

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(tmp);
  });

  it("returns the dir with .aide/ even when no .git/ exists", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tmp, ".aide"), { recursive: true });
    const sub = join(tmp, "a", "b");
    mkdirSync(sub, { recursive: true });

    const result = findProjectRoot(sub);

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(tmp);
  });

  it("resolves a git worktree's .git file back to the main repo root", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");

    // Main repo at <tmp>/main with .git/worktrees/wt/
    const mainRepo = join(tmp, "main");
    mkdirSync(join(mainRepo, ".git", "worktrees", "wt"), { recursive: true });

    // Worktree at <tmp>/wt with .git as a file
    const worktree = join(tmp, "wt");
    mkdirSync(join(worktree, "sub"), { recursive: true });
    writeFileSync(
      join(worktree, ".git"),
      `gitdir: ${join(mainRepo, ".git", "worktrees", "wt")}\n`,
    );

    const result = findProjectRoot(join(worktree, "sub"));

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(mainRepo);
  });

  it("anchors a submodule checkout at the submodule, not the superproject", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");

    // Superproject at <tmp>/super with .git/modules/lib/ and .aide/
    const superRepo = join(tmp, "super");
    mkdirSync(join(superRepo, ".git", "modules", "lib"), { recursive: true });
    mkdirSync(join(superRepo, ".aide"), { recursive: true });

    // Submodule at <tmp>/super/vendor/lib with .git as a file pointing
    // into the superproject's .git/modules/ tree (no .aide/ of its own yet).
    const submodule = join(superRepo, "vendor", "lib");
    mkdirSync(join(submodule, "src"), { recursive: true });
    writeFileSync(
      join(submodule, ".git"),
      `gitdir: ${join(superRepo, ".git", "modules", "lib")}\n`,
    );

    // Starting inside the submodule anchors the submodule.
    const fromSub = findProjectRoot(join(submodule, "src"));
    expect(fromSub.hasMarker).toBe(true);
    expect(fromSub.root).toBe(submodule);

    // Starting in the superproject anchors the superproject.
    const fromSuper = findProjectRoot(superRepo);
    expect(fromSuper.hasMarker).toBe(true);
    expect(fromSuper.root).toBe(superRepo);
  });

  it("resolves a submodule's relative gitdir and nested modules paths", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");

    const superRepo = join(tmp, "super2");
    mkdirSync(join(superRepo, ".git", "modules", "a", "modules", "b"), {
      recursive: true,
    });

    // Nested submodule with a RELATIVE gitdir, as git writes it.
    const inner = join(superRepo, "vendor", "a", "deps", "b");
    mkdirSync(inner, { recursive: true });
    writeFileSync(
      join(inner, ".git"),
      "gitdir: ../../../../.git/modules/a/modules/b\n",
    );

    const result = findProjectRoot(inner);
    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(inner);
  });

  it("skips ~/.aide/ when cwd is not $HOME", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    // ~/.aide/ exists but cwd is an unrelated dir under tempHome
    mkdirSync(join(tempHome, ".aide"), { recursive: true });
    const unrelated = join(tempHome, "projects", "loose");
    mkdirSync(unrelated, { recursive: true });

    const result = findProjectRoot(unrelated);

    expect(result.hasMarker).toBe(false);
    expect(result.root).toBe(unrelated);
  });

  it("treats ~/.aide/ as a marker when cwd is $HOME itself", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tempHome, ".aide"), { recursive: true });

    const result = findProjectRoot(tempHome);

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(tempHome);
  });

  it("returns hasMarker=false with cwd when no markers exist", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    const loose = join(tmp, "loose");
    mkdirSync(loose, { recursive: true });

    const result = findProjectRoot(loose);

    expect(result.hasMarker).toBe(false);
    expect(result.root).toBe(loose);
  });

  it("honours AIDE_PROJECT_ROOT when set to a valid directory", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    const override = join(tmp, "override");
    mkdirSync(override, { recursive: true });
    process.env.AIDE_PROJECT_ROOT = override;

    // cwd is somewhere totally unrelated — the env override wins.
    const result = findProjectRoot(tmpdir());

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(override);
  });

  it("falls back to walk-up when AIDE_PROJECT_ROOT is not a directory", async () => {
    const { findProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tmp, ".git"), { recursive: true });
    const sub = join(tmp, "sub");
    mkdirSync(sub, { recursive: true });
    process.env.AIDE_PROJECT_ROOT = join(tmp, "nonexistent");

    const result = findProjectRoot(sub);

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(tmp);
  });

  it("prefers the parent .git+.aide root over a stray child .aide/", async () => {
    // Regression: an accidental CLI invocation from a subdir of a git repo
    // can plant a sibling .aide/. The walk must NOT latch onto that ghost —
    // the real root has both markers and should win.
    const { findProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tmp, ".git"), { recursive: true });
    mkdirSync(join(tmp, ".aide"), { recursive: true });

    const inner = join(tmp, "module");
    mkdirSync(join(inner, ".aide"), { recursive: true });
    const deep = join(inner, "pkg", "deep");
    mkdirSync(deep, { recursive: true });

    const result = findProjectRoot(deep);

    expect(result.hasMarker).toBe(true);
    expect(result.root).toBe(tmp);
  });
});

describe("walkUpForProjectRoot", () => {
  let tmp: string;

  beforeEach(() => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), "aide-pr-")));
    tempHome = realpathSync(mkdtempSync(join(tmpdir(), "aide-home-")));
    delete process.env.AIDE_PROJECT_ROOT;
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("returns null when nothing is found", async () => {
    const { walkUpForProjectRoot } = await import("../lib/project-root.js");
    const loose = join(tmp, "loose");
    mkdirSync(loose, { recursive: true });

    expect(walkUpForProjectRoot(loose)).toBeNull();
  });

  it("returns the root path when a marker is found", async () => {
    const { walkUpForProjectRoot } = await import("../lib/project-root.js");
    mkdirSync(join(tmp, ".git"), { recursive: true });
    const sub = join(tmp, "sub");
    mkdirSync(sub, { recursive: true });

    expect(walkUpForProjectRoot(sub)).toBe(tmp);
  });
});
