import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { dirname, join } from "path";
import { findAideBinary } from "../core/aide-client.js";
import { getBinaryFileName, getBinaryPackageName } from "../lib/binary-resolve.js";

describe("hook binary selection across npm upgrades", () => {
  let directory: string;
  let project: string;
  let previousPath: string | undefined;

  beforeEach(() => {
    directory = realpathSync(mkdtempSync(join(tmpdir(), "aide-hook-binary-")));
    project = join(directory, "project");
    previousPath = process.env.PATH;
  });

  afterEach(() => {
    if (previousPath === undefined) delete process.env.PATH;
    else process.env.PATH = previousPath;
    rmSync(directory, { recursive: true, force: true });
  });

  function file(path: string): string {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, "fixture");
    return path;
  }

  function installed(version: string): { root: string; binary: string } {
    const modules = join(directory, version, "node_modules");
    const root = join(modules, "@jmylchreest", "aide-plugin");
    mkdirSync(root, { recursive: true });
    writeFileSync(join(root, "package.json"), JSON.stringify({ version }));
    const binary = file(join(modules, getBinaryPackageName()!, "bin", getBinaryFileName()));
    return { root, binary };
  }

  it("uses the loaded package's binary even when the project still has an older executable", () => {
    const next = installed("0.2.2");
    file(join(project, ".aide", "bin", getBinaryFileName()));
    expect(findAideBinary({ cwd: project, pluginRoot: next.root })).toBe(next.binary);
  });

  it("keeps different loaded package versions independent, including rollback", () => {
    const old = installed("0.2.1");
    const next = installed("0.2.2");
    expect(findAideBinary({ cwd: project, pluginRoot: next.root })).toBe(next.binary);
    expect(findAideBinary({ cwd: project, pluginRoot: old.root })).toBe(old.binary);
  });

  it("preserves an explicit executable in the plugin directory", () => {
    const plugin = installed("0.2.2");
    const explicit = file(join(plugin.root, "bin", getBinaryFileName()));
    expect(findAideBinary({ cwd: project, pluginRoot: plugin.root })).toBe(explicit);
  });

  it("keeps the project fallback for plugins without an optional binary package", () => {
    const root = join(directory, "claude-plugin");
    mkdirSync(root);
    const fallback = file(join(project, ".aide", "bin", getBinaryFileName()));
    expect(findAideBinary({ cwd: project, pluginRoot: root })).toBe(fallback);
  });
});
