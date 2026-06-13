/**
 * Tests for shared session-text helpers.
 *
 * Run with: npx vitest run src/test/session-text.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  categorizePartials,
  renderBulletSection,
} from "../core/session-text.js";

describe("categorizePartials", () => {
  it("classifies each partial prefix into the right bucket", () => {
    const c = categorizePartials([
      "Created file: a.ts",
      "Edited file: b.ts",
      "Ran command: go build",
      "Completed task: ship it",
      "Something else entirely",
    ]);
    expect(c.files).toEqual(["a.ts", "b.ts"]);
    expect(c.commands).toEqual(["go build"]);
    expect(c.tasks).toEqual(["ship it"]);
    expect(c.other).toEqual(["Something else entirely"]);
  });

  it("dedupes files while preserving first-seen order", () => {
    const c = categorizePartials([
      "Created file: a.ts",
      "Edited file: b.ts",
      "Edited file: a.ts",
    ]);
    expect(c.files).toEqual(["a.ts", "b.ts"]);
  });

  it("returns empty buckets for empty input", () => {
    const c = categorizePartials([]);
    expect(c).toEqual({ files: [], commands: [], tasks: [], other: [] });
  });
});

describe("renderBulletSection", () => {
  it("returns null for an empty list", () => {
    expect(renderBulletSection("Tasks", [])).toBeNull();
  });

  it("renders a heading and bullet lines", () => {
    expect(renderBulletSection("Tasks", ["a", "b"])).toBe("## Tasks\n- a\n- b");
  });

  it("applies the cap when given", () => {
    expect(renderBulletSection("Files", ["a", "b", "c"], 2)).toBe(
      "## Files\n- a\n- b",
    );
  });

  it("renders all items when no cap is given", () => {
    expect(renderBulletSection("X", ["a", "b", "c"])).toBe(
      "## X\n- a\n- b\n- c",
    );
  });
});
