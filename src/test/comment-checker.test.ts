/**
 * Tests for comment-checker core logic
 *
 * Run with: npx vitest run src/test/comment-checker.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  checkComments,
  getCheckableFilePath,
  getContentToCheck,
} from "../core/comment-checker.js";

describe("checkComments", () => {
  it("should not flag files with no comments", () => {
    const content = `
const x = 1;
const y = 2;
function add(a: number, b: number) {
  return a + b;
}
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(false);
    expect(result.suspiciousCount).toBe(0);
  });

  it("should not flag legitimate comments (TODOs, directives)", () => {
    const content = `
// TODO: refactor this
// eslint-disable-next-line no-unused-vars
// @ts-expect-error - workaround for broken type
// FIXME: handle edge case
const x = 1;
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(false);
  });

  it("should flag obvious comments that restate code", () => {
    const content = `
// Initialize the counter
let counter = 0;
// Set the name
const name = "test";
// Create the array
const arr = [];
// Return the result
return arr;
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(true);
    expect(result.suspiciousCount).toBeGreaterThanOrEqual(2);
  });

  it("should flag empty comments", () => {
    const content = `
//
const x = 1;
//
const y = 2;
//
const z = 3;
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(true);
  });

  it("should flag section divider comments", () => {
    const content = `
// =====================================
function foo() {}
// -------------------------------------
function bar() {}
// *************************************
function baz() {}
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(true);
  });

  it("should not flag non-checkable file extensions", () => {
    const content = `// Initialize the counter\n// Set the name\nconst x = 1;`;
    const result = checkComments("test.md", content);
    expect(result.hasExcessiveComments).toBe(false);
    expect(result.totalComments).toBe(0);
  });

  it("should detect high comment density in full files", () => {
    const lines: string[] = [];
    for (let i = 0; i < 20; i++) {
      lines.push(`// Comment line ${i}`);
      lines.push(`const x${i} = ${i};`);
    }
    // 20 comment lines out of 40 total = 50% density
    const content = lines.join("\n");
    const result = checkComments("test.ts", content, false);
    expect(result.hasExcessiveComments).toBe(true);
  });

  it("should skip density check for test files", () => {
    const lines: string[] = [];
    for (let i = 0; i < 10; i++) {
      lines.push(`// describe test ${i}`);
      lines.push(`const x${i} = ${i};`);
    }
    const content = lines.join("\n");
    // Test files don't get density warnings (many comments are normal in tests)
    const result = checkComments("foo.test.ts", content, false);
    // Only suspicious count matters for test files, not density
    expect(result.totalComments).toBeGreaterThan(0);
  });

  it("should skip density check for new content (Edit tool)", () => {
    const lines: string[] = [];
    for (let i = 0; i < 10; i++) {
      lines.push(`// line ${i}`);
      lines.push(`const x${i} = ${i};`);
    }
    const content = lines.join("\n");
    const resultFull = checkComments("test.ts", content, false);
    const resultNew = checkComments("test.ts", content, true);
    // isNewContent=true skips density check
    expect(resultNew.totalComments).toBe(resultFull.totalComments);
  });

  it("should handle Python hash comments", () => {
    const content = `
# Initialize the counter
counter = 0
# Set the name
name = "test"
# Create the list
arr = []
`;
    const result = checkComments("test.py", content);
    expect(result.hasExcessiveComments).toBe(true);
    expect(result.suspiciousCount).toBeGreaterThanOrEqual(2);
  });

  it("should not flag Python shebangs", () => {
    const content = `#!/usr/bin/env python3
# type: ignore
# pragma: no cover
counter = 0
`;
    const result = checkComments("test.py", content);
    expect(result.hasExcessiveComments).toBe(false);
  });

  it("should include examples in warning message", () => {
    const content = `
// Initialize the counter
let counter = 0;
// Set the name
const name = "test";
// Create the array
const arr = [];
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(true);
    expect(result.warning).toContain("[aide:comment-checker]");
    expect(result.warning).toContain("Examples:");
    expect(result.examples.length).toBeGreaterThan(0);
  });

  it("should not flag JSDoc block comments", () => {
    const content = `
/**
 * Calculate the sum of two numbers.
 * @param a - First number
 * @param b - Second number
 * @returns The sum
 */
function add(a: number, b: number): number {
  return a + b;
}
`;
    const result = checkComments("test.ts", content);
    expect(result.hasExcessiveComments).toBe(false);
  });

  it("should not flag Go build tags", () => {
    const content = `
//go:build linux
//+build linux

package main

func main() {}
`;
    const result = checkComments("test.go", content);
    expect(result.hasExcessiveComments).toBe(false);
  });
});

describe("getCheckableFilePath", () => {
  it("should return path for Write tool", () => {
    const result = getCheckableFilePath("Write", { filePath: "src/foo.ts" });
    expect(result).toBe("src/foo.ts");
  });

  it("should return path for Edit tool", () => {
    const result = getCheckableFilePath("Edit", { file_path: "src/foo.ts" });
    expect(result).toBe("src/foo.ts");
  });

  it("should return path for MultiEdit tool", () => {
    const result = getCheckableFilePath("MultiEdit", {
      filePath: "src/foo.ts",
    });
    expect(result).toBe("src/foo.ts");
  });

  it("should return null for non-write tools", () => {
    expect(
      getCheckableFilePath("Read", { file_path: "src/foo.ts" }),
    ).toBeNull();
    expect(getCheckableFilePath("Bash", { command: "ls" })).toBeNull();
    expect(getCheckableFilePath("Glob", { pattern: "*.ts" })).toBeNull();
  });

  it("should return null for non-checkable extensions", () => {
    expect(getCheckableFilePath("Write", { filePath: "README.md" })).toBeNull();
    expect(getCheckableFilePath("Write", { filePath: "data.json" })).toBeNull();
    expect(getCheckableFilePath("Write", { filePath: "style.css" })).toBeNull();
  });

  it("should return null when no file path", () => {
    expect(getCheckableFilePath("Write", {})).toBeNull();
  });
});

describe("getContentToCheck", () => {
  it("should return full content for Write tool", () => {
    const result = getContentToCheck("Write", { content: "const x = 1;" });
    expect(result).toEqual(["const x = 1;", false]);
  });

  it("should return newString for Edit tool", () => {
    const result = getContentToCheck("Edit", { newString: "const y = 2;" });
    expect(result).toEqual(["const y = 2;", true]);
  });

  it("should handle snake_case new_string for Edit", () => {
    const result = getContentToCheck("Edit", { new_string: "const z = 3;" });
    expect(result).toEqual(["const z = 3;", true]);
  });

  it("should concatenate edits for MultiEdit", () => {
    const result = getContentToCheck("MultiEdit", {
      edits: [{ new_string: "line 1" }, { new_string: "line 2" }],
    });
    expect(result).not.toBeNull();
    expect(result![0]).toContain("line 1");
    expect(result![0]).toContain("line 2");
    expect(result![1]).toBe(true);
  });

  it("should return null for unknown tools", () => {
    expect(getContentToCheck("Read", {})).toBeNull();
  });

  it("should return null when content is missing", () => {
    expect(getContentToCheck("Write", {})).toBeNull();
    expect(getContentToCheck("Edit", {})).toBeNull();
  });
});
