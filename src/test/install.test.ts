/**
 * Tests for install-time binary status reporting.
 *
 * install.ts is a config writer. Binary provisioning is owned by npm
 * (optionalDependencies) and the MCP wrapper. install's only binary
 * responsibility is reporting where the binary lives so users can spot
 * a missing optionalDependency install.
 *
 * Run with: npx vitest run src/test/install.test.ts
 */

import { describe, it, expect, afterEach } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import {
  checkBinaryStatus,
  isBinaryCurrent,
  readBinaryVersion,
} from "../cli/install.js";
import * as downloader from "../lib/aide-downloader.js";

describe("isBinaryCurrent", () => {
  it("returns true when release binary equals plugin version", () => {
    expect(isBinaryCurrent("0.1.12", "0.1.12")).toBe(true);
  });

  it("returns true when release binary is newer than plugin version", () => {
    expect(isBinaryCurrent("0.2.0", "0.1.12")).toBe(true);
    expect(isBinaryCurrent("0.1.13", "0.1.12")).toBe(true);
  });

  it("returns false when release binary is older than plugin version", () => {
    expect(isBinaryCurrent("0.0.61", "0.1.12")).toBe(false);
    expect(isBinaryCurrent("0.1.9", "0.1.12")).toBe(false);
  });

  it("compares only the base version for dev builds", () => {
    expect(isBinaryCurrent("0.1.12-dev.5+abc1234", "0.1.12")).toBe(true);
    expect(isBinaryCurrent("0.1.9-dev.20+def5678", "0.1.12")).toBe(false);
  });
});

describe("readBinaryVersion", () => {
  let tempDir: string;

  afterEach(() => {
    if (tempDir) rmSync(tempDir, { recursive: true, force: true });
  });

  it("returns null for a nonexistent path", async () => {
    tempDir = mkdtempSync(join(tmpdir(), "aide-readbin-"));
    const v = await readBinaryVersion(join(tempDir, "does-not-exist"));
    expect(v).toBeNull();
  });

  it("returns null when execFileSync fails (e.g. non-executable file)", async () => {
    tempDir = mkdtempSync(join(tmpdir(), "aide-readbin-"));
    const fakeBin = join(tempDir, "aide");
    writeFileSync(fakeBin, "not executable");
    const v = await readBinaryVersion(fakeBin);
    expect(v).toBeNull();
  });
});

describe("checkBinaryStatus (sanity contract)", () => {
  it("returns one of the valid BinaryStatus kinds", async () => {
    const status = await checkBinaryStatus();
    expect(["bundled", "legacy", "missing"]).toContain(status.kind);
    if (status.kind === "bundled") {
      expect(typeof status.version).toBe("string");
      expect(status.version).toMatch(/^\d+\.\d+\.\d+/);
      expect(typeof status.path).toBe("string");
    } else if (status.kind === "legacy") {
      expect(typeof status.version).toBe("string");
      expect(status.version).toMatch(/^\d+\.\d+\.\d+/);
      expect(typeof status.path).toBe("string");
    } else if (status.kind === "missing") {
      expect(typeof status.reason).toBe("string");
      expect(status.reason.length).toBeGreaterThan(0);
    }
  });
});

describe("downloader API: explicit pluginVersion / pluginRoot options", () => {
  let savedRoot: string | undefined;
  let savedClaudeRoot: string | undefined;

  afterEach(() => {
    if (savedRoot === undefined) delete process.env.AIDE_PLUGIN_ROOT;
    else process.env.AIDE_PLUGIN_ROOT = savedRoot;
    if (savedClaudeRoot === undefined)
      delete process.env.CLAUDE_PLUGIN_ROOT;
    else process.env.CLAUDE_PLUGIN_ROOT = savedClaudeRoot;
  });

  it("getPluginVersion reads from explicit pluginRoot, ignoring env", () => {
    savedRoot = process.env.AIDE_PLUGIN_ROOT;
    process.env.AIDE_PLUGIN_ROOT = "/nonexistent/path/that/has/no/package.json";

    const repoRoot = join(import.meta.dirname, "..", "..");
    const v = downloader.getPluginVersion({ pluginRoot: repoRoot });
    expect(v).toBeTruthy();
    expect(v).not.toBe("0.0.0");
    expect(v).toMatch(/^\d+\.\d+\.\d+/);
  });

  it("getPluginVersion falls back to env when no explicit root given", () => {
    savedRoot = process.env.AIDE_PLUGIN_ROOT;
    process.env.AIDE_PLUGIN_ROOT = join(
      import.meta.dirname,
      "..",
      "..",
      "packages",
      "opencode-plugin",
    );
    const v = downloader.getPluginVersion();
    expect(v).toBeTruthy();
    expect(v).toMatch(/^\d+\.\d+\.\d+/);
  });

  it("getDownloadUrls uses explicit pluginVersion over env lookup", () => {
    const urls = downloader.getDownloadUrls({ pluginVersion: "9.9.9-fake" });
    expect(urls[0]).toContain("releases/download/v9.9.9-fake/");
    expect(urls[1]).toContain("releases/latest/download/");
  });

  it("getDownloadUrls returns at least one URL even when version unresolvable", () => {
    savedRoot = process.env.AIDE_PLUGIN_ROOT;
    process.env.AIDE_PLUGIN_ROOT = "/nope";
    savedClaudeRoot = process.env.CLAUDE_PLUGIN_ROOT;
    process.env.CLAUDE_PLUGIN_ROOT = "/nope";

    const urls = downloader.getDownloadUrls();
    expect(urls.length).toBeGreaterThan(0);
    expect(urls[0]).toContain("github.com/jmylchreest/aide/releases");
  });
});

it("module compile sanity", () => {
  expect(typeof checkBinaryStatus).toBe("function");
  expect(typeof isBinaryCurrent).toBe("function");
  expect(typeof readBinaryVersion).toBe("function");
});