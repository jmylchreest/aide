/**
 * Tests for install-time binary version reconciliation.
 *
 * install.ts reads the plugin version from its own package.json (not from
 * AIDE_PLUGIN_ROOT) and uses that to drive the Go binary download. These
 * tests pin the pure-logic contracts and the downloader API extensions
 * that make that work. End-to-end behaviour (stale-bin → network → fresh
 * binary) is verified manually because faking a stale bin/aide in unit
 * tests requires a module seam we deliberately did not introduce.
 *
 * Run with: npx vitest run src/test/install.test.ts
 */

import { describe, it, expect, afterEach } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import {
  ensureBinaryMatchesPluginVersion,
  isBinaryCurrent,
  readBinaryVersion,
} from "../cli/install.js";
import * as downloader from "../lib/aide-downloader.js";

describe("isBinaryCurrent", () => {
  it("returns true when release binary equals plugin version", () => {
    expect(isBinaryCurrent("0.1.10", "0.1.10")).toBe(true);
  });

  it("returns true when release binary is newer than plugin version", () => {
    expect(isBinaryCurrent("0.2.0", "0.1.10")).toBe(true);
    expect(isBinaryCurrent("0.1.11", "0.1.10")).toBe(true);
  });

  it("returns false when release binary is older than plugin version", () => {
    expect(isBinaryCurrent("0.0.61", "0.1.10")).toBe(false);
    expect(isBinaryCurrent("0.1.9", "0.1.10")).toBe(false);
  });

  it("compares only the base version for dev builds", () => {
    expect(isBinaryCurrent("0.1.10-dev.5+abc1234", "0.1.10")).toBe(true);
    expect(isBinaryCurrent("0.1.9-dev.20+def5678", "0.1.10")).toBe(false);
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
    // No chmod — execFileSync should fail on most filesystems.
    const v = await readBinaryVersion(fakeBin);
    expect(v).toBeNull();
  });
});

describe("ensureBinaryMatchesPluginVersion (sanity contract)", () => {
  it("returns one of the valid UpgradeOutcome kinds", async () => {
    const outcome = await ensureBinaryMatchesPluginVersion();
    expect(["current", "upgraded", "skipped", "failed"]).toContain(
      outcome.kind,
    );
    if (outcome.kind === "current") {
      expect(typeof outcome.binaryVersion).toBe("string");
      expect(typeof outcome.pluginVersion).toBe("string");
    } else if (outcome.kind === "upgraded") {
      expect(typeof outcome.toVersion).toBe("string");
      expect(typeof outcome.path).toBe("string");
    } else if (outcome.kind === "skipped") {
      expect(typeof outcome.reason).toBe("string");
    } else if (outcome.kind === "failed") {
      expect(typeof outcome.error).toBe("string");
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

  it("getDownloadUrls falls back to env-derived pluginVersion", () => {
    savedRoot = process.env.AIDE_PLUGIN_ROOT;
    process.env.AIDE_PLUGIN_ROOT = join(
      import.meta.dirname,
      "..",
      "..",
      "packages",
      "opencode-plugin",
    );
    const urls = downloader.getDownloadUrls();
    const expectedVersion = downloader.getPluginVersion();
    if (expectedVersion) {
      expect(urls[0]).toContain(`releases/download/v${expectedVersion}/`);
    }
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
  expect(typeof ensureBinaryMatchesPluginVersion).toBe("function");
  expect(typeof isBinaryCurrent).toBe("function");
  expect(typeof readBinaryVersion).toBe("function");
});
