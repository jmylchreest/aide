import { describe, expect, it } from "vitest";
import { readFileSync } from "fs";
import * as path from "path";
import { runInNewContext } from "vm";
import { transpileModule } from "typescript";

// Execute the real standalone hook with an isolated filesystem. Its resolver
// walks to the filesystem root, so host ancestor markers must not affect it.
const hook = transpileModule(
  readFileSync(
    path.join(import.meta.dirname, "../hooks/session-end.ts"),
    "utf8",
  ),
  { compilerOptions: { target: 99 } },
).outputText;

describe("session-end fallback project identity", () => {
  it.each([
    "unmarked child",
    "home itself",
    "home git repository",
    "child project",
  ])("%s respects the home marker exclusion", (scenario) => {
    const home = path.resolve("fixture-home");
    const child = path.join(home, "child");
    const markers = new Set([path.join(home, ".aide")]);
    if (scenario === "home git repository")
      markers.add(path.join(home, ".git"));
    if (scenario === "child project") markers.add(path.join(child, ".aide"));
    const writes: string[] = [];
    const output: string[] = [];
    runInNewContext(hook, {
      performance,
      console: { log: (value: string) => output.push(value) },
      process: {
        env: {},
        cwd: () => (scenario === "home itself" ? home : child),
        on: () => {},
        exit: () => {},
      },
      require: (name: string) => {
        if (name === "path") return path;
        if (name === "os") return { homedir: () => home };
        if (name === "which") return { sync: () => null };
        if (name === "child_process") return {};
        if (name === "fs")
          return {
            existsSync: (p: string) => markers.has(p),
            statSync: (p: string) => {
              if (!markers.has(p)) throw new Error("ENOENT");
              return { isFile: () => false, isDirectory: () => true };
            },
            readFileSync: () => "{}",
            mkdirSync: () => {},
            appendFileSync: (p: string) => writes.push(p),
          };
        throw new Error(`Unexpected module ${name}`);
      },
    });
    expect(output.map((line) => JSON.parse(line))).toEqual([
      { continue: true },
    ]);
    const expectedRoot = scenario === "child project" ? child : home;
    expect(writes.length > 0).toBe(scenario !== "unmarked child");
    expect(
      writes.every(
        (p) =>
          p === path.join(expectedRoot, ".aide", "_logs", "session-end.log"),
      ),
    ).toBe(true);
  });
});
