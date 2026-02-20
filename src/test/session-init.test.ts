/**
 * Tests for session initialization helpers
 *
 * Run with: npx vitest run src/test/session-init.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  mkdtempSync,
  rmSync,
  writeFileSync,
  readFileSync,
  mkdirSync,
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

describe("ensureDirectories", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-session-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-home-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("creates required directories and gitignore", async () => {
    const { ensureDirectories } = await import("../core/session-init.js");

    const result = ensureDirectories(projectDir);
    expect(result.created).toBeGreaterThan(0);

    const gitignorePath = join(projectDir, ".aide", ".gitignore");
    const gitignoreContent = readFileSync(gitignorePath, "utf-8");
    expect(gitignoreContent).toContain("!shared/");
    expect(gitignoreContent).toContain("config/mcp.json");
  });
});

describe("loadConfig", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-session-config-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-home-config-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("returns default config when missing", async () => {
    const { loadConfig } = await import("../core/session-init.js");
    const { DEFAULT_CONFIG } = await import("../core/types.js");
    const config = loadConfig(projectDir);
    expect(config).toEqual(DEFAULT_CONFIG);
  });

  it("merges user config with defaults", async () => {
    const { loadConfig } = await import("../core/session-init.js");

    const configDir = join(projectDir, ".aide", "config");
    mkdirSync(configDir, { recursive: true });
    writeFileSync(
      join(configDir, "aide.json"),
      JSON.stringify({ share: { autoImport: true } }, null, 2),
    );

    const config = loadConfig(projectDir);
    expect(config.share?.autoImport).toBe(true);
  });
});
