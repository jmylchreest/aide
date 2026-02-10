/**
 * Tests for write-guard core logic
 *
 * Run with: npx vitest run src/test/write-guard.test.ts
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, writeFileSync, rmSync, existsSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { checkWriteGuard } from "../core/write-guard.js";

describe("checkWriteGuard", () => {
  let testDir: string;

  beforeEach(() => {
    testDir = join(tmpdir(), `aide-write-guard-${Date.now()}`);
    mkdirSync(testDir, { recursive: true });
  });

  afterEach(() => {
    if (existsSync(testDir)) {
      rmSync(testDir, { recursive: true, force: true });
    }
  });

  it("should allow Write for non-existent files", () => {
    const result = checkWriteGuard(
      "Write",
      { filePath: join(testDir, "new-file.ts") },
      testDir,
    );
    expect(result.allowed).toBe(true);
    expect(result.message).toBeUndefined();
  });

  it("should block Write for existing files", () => {
    const existingFile = join(testDir, "existing.ts");
    writeFileSync(existingFile, "const x = 1;");

    const result = checkWriteGuard(
      "Write",
      { filePath: existingFile },
      testDir,
    );
    expect(result.allowed).toBe(false);
    expect(result.message).toContain("already exists");
    expect(result.message).toContain("Edit tool");
  });

  it("should allow non-Write tools", () => {
    const existingFile = join(testDir, "existing.ts");
    writeFileSync(existingFile, "const x = 1;");

    expect(
      checkWriteGuard("Edit", { filePath: existingFile }, testDir).allowed,
    ).toBe(true);
    expect(
      checkWriteGuard("Read", { filePath: existingFile }, testDir).allowed,
    ).toBe(true);
    expect(checkWriteGuard("Bash", { command: "ls" }, testDir).allowed).toBe(
      true,
    );
  });

  it("should allow Write when no file_path provided", () => {
    const result = checkWriteGuard("Write", {}, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to .gitignore (in allow list)", () => {
    const gitignore = join(testDir, ".gitignore");
    writeFileSync(gitignore, "node_modules/");

    const result = checkWriteGuard("Write", { filePath: gitignore }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to tsconfig.json (in allow list)", () => {
    const tsconfig = join(testDir, "tsconfig.json");
    writeFileSync(tsconfig, "{}");

    const result = checkWriteGuard("Write", { filePath: tsconfig }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to package-lock.json (in allow list)", () => {
    const lockfile = join(testDir, "package-lock.json");
    writeFileSync(lockfile, "{}");

    const result = checkWriteGuard("Write", { filePath: lockfile }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to .aide/ directory files", () => {
    const aideDir = join(testDir, ".aide", "state");
    mkdirSync(aideDir, { recursive: true });
    const stateFile = join(aideDir, "session.json");
    writeFileSync(stateFile, "{}");

    const result = checkWriteGuard("Write", { filePath: stateFile }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to .env files (in allow list)", () => {
    const envFile = join(testDir, ".env");
    writeFileSync(envFile, "FOO=bar");

    const result = checkWriteGuard("Write", { filePath: envFile }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should allow Write to rc files (in allow list)", () => {
    const rcFile = join(testDir, ".eslintrc");
    writeFileSync(rcFile, "{}");

    const result = checkWriteGuard("Write", { filePath: rcFile }, testDir);
    expect(result.allowed).toBe(true);
  });

  it("should handle file_path (snake_case) input key", () => {
    const existingFile = join(testDir, "existing.ts");
    writeFileSync(existingFile, "const x = 1;");

    const result = checkWriteGuard(
      "Write",
      { file_path: existingFile },
      testDir,
    );
    expect(result.allowed).toBe(false);
  });

  it("should handle path input key", () => {
    const existingFile = join(testDir, "existing.ts");
    writeFileSync(existingFile, "const x = 1;");

    const result = checkWriteGuard("Write", { path: existingFile }, testDir);
    expect(result.allowed).toBe(false);
  });

  it("should resolve relative paths", () => {
    const existingFile = join(testDir, "relative.ts");
    writeFileSync(existingFile, "const x = 1;");

    const result = checkWriteGuard(
      "Write",
      { filePath: "relative.ts" },
      testDir,
    );
    expect(result.allowed).toBe(false);
  });

  it("should be case-insensitive for tool name", () => {
    const existingFile = join(testDir, "existing.ts");
    writeFileSync(existingFile, "const x = 1;");

    expect(
      checkWriteGuard("write", { filePath: existingFile }, testDir).allowed,
    ).toBe(false);
    expect(
      checkWriteGuard("WRITE", { filePath: existingFile }, testDir).allowed,
    ).toBe(false);
    expect(
      checkWriteGuard("Write", { filePath: existingFile }, testDir).allowed,
    ).toBe(false);
  });
});
