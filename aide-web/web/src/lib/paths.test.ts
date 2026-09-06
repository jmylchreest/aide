import { describe, expect, it } from "vitest";
import { relativeToRoot } from "./paths";

describe("project-relative display paths", () => {
  it.each([
    ["/work/aide/src/index.ts", "/work/aide", "src/index.ts"],
    ["/work/aide/src/index.ts", "/work/aide/", "src/index.ts"],
    [
      "/work/aide-other/src/index.ts",
      "/work/aide",
      "/work/aide-other/src/index.ts",
    ],
    ["/work/aide", "/work/aide", "."],
    ["src/index.ts", "/work/aide", "src/index.ts"],
    ["session-start", "/work/aide", "session-start"],
    ["C:\\Work\\Aide\\src\\index.ts", "c:/work/aide", "src/index.ts"],
    ["\\\\host\\share\\aide\\file.go", "//host/share/aide/", "file.go"],
    ["/work/aide/file.go", "/", "work/aide/file.go"],
  ])("formats %s relative to %s", (path, root, expected) => {
    expect(relativeToRoot(path, root)).toBe(expected);
  });
});
