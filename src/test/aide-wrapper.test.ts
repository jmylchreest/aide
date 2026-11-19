import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { spawnSync } from "child_process";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join } from "path";

let dir: string;
let root: string;
let stale: string;

// These executable fixtures use /bin/sh. The installer and process-lock tests
// exercise the platform-independent implementation on Windows too.
describe.skipIf(process.platform === "win32")("MCP wrapper startup", () => {
  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "aide-wrapper-"));
    root = join(dir, "new plugin");
    stale = join(dir, "old plugin");
    for (const [path, version] of [
      [root, "0.2.1"],
      [stale, "0.1.18"],
    ]) {
      mkdirSync(join(path, "bin"), { recursive: true });
      writeFileSync(join(path, "package.json"), JSON.stringify({ version }));
    }
    const repo = join(import.meta.dirname, "../..");
    copyFileSync(
      join(repo, "bin/aide-wrapper.ts"),
      join(root, "bin/aide-wrapper.ts"),
    );
    symlinkSync(join(repo, "node_modules"), join(root, "node_modules"));
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  function binary(path: string, version: string) {
    writeFileSync(
      path,
      `#!/bin/sh\nif [ "$1" = version ]; then\n  echo 'aide version ${version}'\nelse\n  printf '%s' "$AIDE_PLUGIN_ROOT"\nfi\n`,
      { mode: 0o755 },
    );
  }

  function run() {
    return spawnSync("bun", [join(root, "bin/aide-wrapper.ts"), "mcp"], {
      cwd: dir,
      env: {
        ...process.env,
        AIDE_PLUGIN_ROOT: stale,
        CLAUDE_PLUGIN_ROOT: stale,
      },
      encoding: "utf8",
      timeout: 10000,
    });
  }

  it("starts the current package's executable despite inherited stale roots", () => {
    binary(join(root, "bin/aide"), "0.2.1");
    binary(join(stale, "bin/aide"), "0.1.18");
    const result = run();
    expect(result.status, result.stderr).toBe(0);
    expect(result.stdout).toBe(root);
  });

  it("passes its version to the downloader, retains the old file, and keeps stdout clean", () => {
    binary(join(root, "bin/aide"), "0.1.18");
    const downloader = join(root, "src/lib");
    mkdirSync(downloader, { recursive: true });
    writeFileSync(
      join(downloader, "aide-downloader.ts"),
      `
      import { readFileSync, writeFileSync, renameSync } from "fs";
      import { join } from "path";
      const args = process.argv.slice(2);
      if (args[args.indexOf("--version") + 1] !== "0.2.1") throw new Error("wrong target");
      const dest = join(args[args.indexOf("--dest") + 1], "aide");
      const old = readFileSync(dest, "utf8");
      if (!old.includes("0.1.18")) throw new Error("old binary removed before download");
      writeFileSync(dest + ".new", old.replace("0.1.18", "0.2.1"), { mode: 0o755 });
      renameSync(dest + ".new", dest);
      console.log("installer progress");
    `,
    );
    const result = run();
    expect(result.status, result.stderr).toBe(0);
    expect(result.stdout).toBe(root);
    expect(result.stderr).toContain("installer progress");
  });
});
