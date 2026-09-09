import { beforeEach, describe, expect, it, vi } from "vitest";
import { mocked } from "./helpers/runner-compat.js";

vi.mock("child_process", () => ({ execFileSync: vi.fn() }));
vi.mock("../lib/hook-utils.js", () => ({
  codeWatchEnabled: vi.fn(() => true),
}));
import { execFileSync } from "child_process";
import { codeWatchEnabled } from "../lib/hook-utils.js";
import { checkSearchEnrichment } from "../core/search-enrichment.js";

const mockExec = mocked(execFileSync);
const mockWatch = mocked(codeWatchEnabled);
beforeEach(() => {
  vi.clearAllMocks();
  mockWatch.mockReturnValue(true);
  mockExec.mockImplementation((_binary: unknown, args: unknown) =>
    (args as string[])[1] === "search"
      ? JSON.stringify([
          {
            name: "authenticate",
            kind: "function",
            file: "src/auth.ts",
            start: 12,
          },
        ])
      : "[]",
  );
});

describe("search enrichment retrieval guidance", () => {
  it.each([
    ["Grep", "pattern"],
    ["grep", "query"],
    ["grep", "search"],
  ])(
    "attaches actionable choices to indexed matches for %s/%s",
    (tool, key) => {
      const result = checkSearchEnrichment(
        tool,
        { [key]: "authenticate" },
        "/project",
        "aide",
      );
      expect(result.shouldEnrich).toBe(true);
      expect(result.enrichment).toContain("src/auth.ts:12");
      expect(result.enrichment).toContain("code_search");
      expect(result.enrichment).toContain("code_references");
      expect(result.enrichment).toContain("code_read_symbol");
      expect(result.enrichment).toContain("symbols");
      expect(result.enrichment).toContain("literals");
      expect(result.enrichment).toContain("imports");
      expect(mockExec).toHaveBeenCalledTimes(2);
      expect(mockExec).toHaveBeenNthCalledWith(
        1,
        "aide",
        ["code", "search", "authenticate", "--json", "--limit=5"],
        expect.any(Object),
      );
    },
  );

  it.each([
    "ab",
    "auth.*",
    "import auth",
    "src/auth",
    "auth\tname",
    '"authenticate"',
  ])(
    "does not query the index or emit guidance for non-symbol search %j",
    (pattern) => {
      expect(
        checkSearchEnrichment("Grep", { pattern }, "/project", "aide"),
      ).toEqual({ shouldEnrich: false });
      expect(mockExec).not.toHaveBeenCalled();
    },
  );

  it("does not add a generic tool reminder when the index has no match", () => {
    mockExec.mockReturnValue("[]");
    expect(
      checkSearchEnrichment("Grep", { pattern: "unknown" }, "/project", "aide"),
    ).toEqual({ shouldEnrich: false });
    expect(mockExec).toHaveBeenCalledTimes(1);
  });

  it.each(["failure", "invalid-json", "{}", ""])(
    "does not report a known zero for unavailable reference results: %s",
    (response) => {
      mockExec.mockImplementation((_binary: unknown, args: unknown) => {
        if ((args as string[])[1] === "search") {
          return JSON.stringify([
            { name: "authenticate", file: "src/auth.ts", start: 12 },
          ]);
        }
        if (response === "failure") throw new Error("daemon unavailable");
        return response;
      });
      const result = checkSearchEnrichment(
        "Grep",
        { pattern: "authenticate" },
        "/project",
        "aide",
      );
      expect(result.shouldEnrich).toBe(true);
      expect(result.enrichment).toContain("refs unavailable");
      expect(result.enrichment).not.toMatch(/\b0 (indexed )?refs/);
    },
  );

  it.each([0, 3, 100])(
    "labels bounded reference results by name: %i",
    (count) => {
      mockExec.mockImplementation((_binary: unknown, args: unknown) =>
        (args as string[])[1] === "search"
          ? JSON.stringify([
              { name: "authenticate", file: "src/auth.ts", start: 12 },
            ])
          : JSON.stringify(
              Array.from({ length: count }, () => ({ file: "src/app.ts" })),
            ),
      );
      const result = checkSearchEnrichment(
        "Grep",
        { pattern: "authenticate" },
        "/project",
        "aide",
      );
      expect(result.enrichment).toContain(
        `${count === 100 ? "100+" : count} indexed refs by name`,
      );
    },
  );

  it("keeps unavailable and disabled indexing quiet", () => {
    expect(
      checkSearchEnrichment(
        "Grep",
        { pattern: "authenticate" },
        "/project",
        null,
      ),
    ).toEqual({ shouldEnrich: false });
    mockWatch.mockReturnValue(false);
    expect(
      checkSearchEnrichment(
        "Grep",
        { pattern: "authenticate" },
        "/project",
        "aide",
      ),
    ).toEqual({ shouldEnrich: false });
    expect(mockExec).not.toHaveBeenCalled();
  });
});
