/**
 * Tests for the OpenCode plugin's project-root resolution.
 *
 * Regression guard for the hooks-vs-MCP split-brain: resolveProjectRoot
 * previously preferred ctx.project.worktree unconditionally, which for a
 * submodule points at the superproject side — while `aide mcp` (resolving
 * from its own cwd) anchored the submodule. The canonical resolver seeded
 * with ctx.directory must win whenever it finds a marker.
 *
 * Run with: npx vitest run src/test/opencode-resolve-root.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync, realpathSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import type { PluginInput } from "../opencode/types.js";

let tempHome = "";

vi.mock("os", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("os");
  return {
    ...actual,
    homedir: () => tempHome,
  };
});

function ctxWith(
  directory: string,
  worktree: string,
  projectWorktree: string,
): PluginInput {
  return {
    directory,
    worktree,
    project: { worktree: projectWorktree },
  } as unknown as PluginInput;
}

describe("resolveProjectRoot (OpenCode)", () => {
  let tmp: string;

  beforeEach(() => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), "aide-oc-")));
    tempHome = realpathSync(mkdtempSync(join(tmpdir(), "aide-oc-home-")));
    delete process.env.AIDE_PROJECT_ROOT;
    delete process.env.AIDE_FORCE_INIT;
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
    delete process.env.AIDE_PROJECT_ROOT;
    delete process.env.AIDE_FORCE_INIT;
  });

  it("anchors a submodule at the submodule even when ctx.project.worktree points at the superproject", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const superRepo = join(tmp, "super");
    mkdirSync(join(superRepo, ".git", "modules", "lib"), { recursive: true });
    const submodule = join(superRepo, "vendor", "lib");
    mkdirSync(join(submodule, "src"), { recursive: true });
    writeFileSync(
      join(submodule, ".git"),
      `gitdir: ${join(superRepo, ".git", "modules", "lib")}\n`,
    );

    const result = resolveProjectRoot(
      ctxWith(join(submodule, "src"), submodule, superRepo),
    );
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(submodule);
  });

  it("resolves a plain repo to its root", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const repo = join(tmp, "repo");
    mkdirSync(join(repo, ".git"), { recursive: true });
    const sub = join(repo, "pkg");
    mkdirSync(sub, { recursive: true });

    const result = resolveProjectRoot(ctxWith(sub, repo, repo));
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(repo);
  });

  it("still shares the main repo root from a linked worktree", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const mainRepo = join(tmp, "main");
    mkdirSync(join(mainRepo, ".git", "worktrees", "wt"), { recursive: true });
    const worktree = join(tmp, "wt");
    mkdirSync(join(worktree, "sub"), { recursive: true });
    writeFileSync(
      join(worktree, ".git"),
      `gitdir: ${join(mainRepo, ".git", "worktrees", "wt")}\n`,
    );

    const result = resolveProjectRoot(
      ctxWith(join(worktree, "sub"), worktree, mainRepo),
    );
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(mainRepo);
  });

  it("falls back to ctx.project.worktree when the walk finds no marker", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const bare = join(tmp, "bare");
    mkdirSync(bare, { recursive: true });
    const elsewhere = join(tmp, "elsewhere");
    mkdirSync(join(elsewhere, ".git"), { recursive: true });

    const result = resolveProjectRoot(ctxWith(bare, "/", elsewhere));
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(elsewhere);
  });

  it("reports no project root for a non-git directory", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const bare = join(tmp, "bare");
    mkdirSync(bare, { recursive: true });

    const result = resolveProjectRoot(ctxWith(bare, "/", "/"));
    expect(result.hasProjectRoot).toBe(false);
  });

  it("honors AIDE_FORCE_INIT for a non-git directory", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const bare = join(tmp, "bare");
    mkdirSync(bare, { recursive: true });
    process.env.AIDE_FORCE_INIT = "1";

    const result = resolveProjectRoot(ctxWith(bare, "/", "/"));
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(bare);
  });

  it("honors AIDE_PROJECT_ROOT over everything", async () => {
    const { resolveProjectRoot } = await import("../opencode/resolve-root.js");

    const override = join(tmp, "override");
    mkdirSync(override, { recursive: true });
    const repo = join(tmp, "repo");
    mkdirSync(join(repo, ".git"), { recursive: true });
    process.env.AIDE_PROJECT_ROOT = override;

    const result = resolveProjectRoot(ctxWith(repo, repo, repo));
    expect(result.hasProjectRoot).toBe(true);
    expect(result.root).toBe(override);
  });
});
