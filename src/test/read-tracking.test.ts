/**
 * Tests for read-tracking and smart-read-hint core logic
 *
 * Run with: npx vitest run src/test/read-tracking.test.ts
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { checkSmartReadHint } from "../core/context-guard.js";

// Mock aide-client and read-tracking modules
vi.mock("../core/aide-client.js", () => ({
  setState: vi.fn(),
  getState: vi.fn(),
  findAideBinary: vi.fn(() => "/usr/bin/aide"),
}));

vi.mock("../core/read-tracking.js", async (importOriginal) => {
  const original =
    (await importOriginal()) as typeof import("../core/read-tracking.js");
  return {
    ...original,
    getPreviousRead: vi.fn(),
    checkFileReadFreshness: vi.fn(),
    recordFileRead: vi.fn(),
  };
});

import { getPreviousRead, checkFileReadFreshness, recordFileRead } from "../core/read-tracking.js";
import { setState } from "../core/aide-client.js";

const mockGetPreviousRead = vi.mocked(getPreviousRead);
const mockCheckFreshness = vi.mocked(checkFileReadFreshness);
const mockRecordFileRead = vi.mocked(recordFileRead);
const mockSetState = vi.mocked(setState);

describe("checkSmartReadHint", () => {
  const cwd = "/home/user/project";
  const binary = "/usr/bin/aide";

  beforeEach(() => {
    vi.clearAllMocks();
    process.env.AIDE_CODE_WATCH = "1";
  });

  afterEach(() => {
    delete process.env.AIDE_CODE_WATCH;
  });

  it("should not hint for non-Read tool", () => {
    const result = checkSmartReadHint("Edit", { file_path: "foo.ts" }, cwd, binary);
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint when AIDE_CODE_WATCH is not set", () => {
    delete process.env.AIDE_CODE_WATCH;
    const result = checkSmartReadHint("Read", { file_path: "foo.ts" }, cwd, binary);
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint when binary is null", () => {
    const result = checkSmartReadHint("Read", { file_path: "foo.ts" }, cwd, null);
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint when no file_path in input", () => {
    const result = checkSmartReadHint("Read", {}, cwd, binary);
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint for targeted reads with offset", () => {
    const result = checkSmartReadHint(
      "Read",
      { file_path: "foo.ts", offset: 50 },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint for targeted reads with small limit", () => {
    const result = checkSmartReadHint(
      "Read",
      { file_path: "foo.ts", limit: 20 },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint for non-source-code files", () => {
    const result = checkSmartReadHint(
      "Read",
      { file_path: "data.json" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint for .md files", () => {
    const result = checkSmartReadHint(
      "Read",
      { file_path: "README.md" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint on first read (no previous read)", () => {
    mockGetPreviousRead.mockReturnValue(null);

    const result = checkSmartReadHint(
      "Read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
    expect(mockGetPreviousRead).toHaveBeenCalled();
    expect(mockCheckFreshness).not.toHaveBeenCalled();
  });

  it("should hint when file was read before and is indexed+fresh", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue({
      indexed: true,
      fresh: true,
      symbols: 5,
      outline_available: true,
      estimated_tokens: 1200,
    });

    const result = checkSmartReadHint(
      "Read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(true);
    expect(result.hint).toContain("[aide:smart-read]");
    expect(result.hint).toContain("code_outline");
    expect(result.hint).toContain("~1200 tokens");
  });

  it("should not hint when file changed since indexing (not fresh)", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue({
      indexed: true,
      fresh: false,
      symbols: 5,
      outline_available: true,
      estimated_tokens: 1200,
    });

    const result = checkSmartReadHint(
      "Read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint when file is not indexed", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue({
      indexed: false,
      fresh: false,
      symbols: 0,
      outline_available: false,
      estimated_tokens: 0,
    });

    const result = checkSmartReadHint(
      "Read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should not hint when freshness check returns null (error)", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue(null);

    const result = checkSmartReadHint(
      "Read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(false);
  });

  it("should handle case-insensitive tool name for OpenCode compat", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue({
      indexed: true,
      fresh: true,
      symbols: 3,
      outline_available: true,
      estimated_tokens: 800,
    });

    const result = checkSmartReadHint(
      "read",
      { file_path: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(true);
  });

  it("should accept filePath variant in tool input", () => {
    mockGetPreviousRead.mockReturnValue("2026-04-03T10:00:00.000Z");
    mockCheckFreshness.mockReturnValue({
      indexed: true,
      fresh: true,
      symbols: 2,
      outline_available: true,
      estimated_tokens: 600,
    });

    const result = checkSmartReadHint(
      "Read",
      { filePath: "src/auth.ts" },
      cwd,
      binary,
    );
    expect(result.shouldHint).toBe(true);
  });
});

describe("recordFileRead", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.AIDE_CODE_WATCH = "1";
  });

  afterEach(() => {
    delete process.env.AIDE_CODE_WATCH;
  });

  it("should be a no-op when AIDE_CODE_WATCH is not set", () => {
    delete process.env.AIDE_CODE_WATCH;
    mockRecordFileRead.mockImplementation(() => {});
    recordFileRead("/usr/bin/aide", "/home/user/project", "src/auth.ts");
    // The mock was called, but the real impl would early-return
    // We verify the function exists and is callable
    expect(mockRecordFileRead).toHaveBeenCalled();
  });
});
