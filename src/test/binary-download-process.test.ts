import { afterEach, beforeEach, expect, it } from "vitest";
import { spawn, type ChildProcessWithoutNullStreams } from "child_process";
import { once } from "events";
import { mkdtempSync, readdirSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { pathToFileURL } from "url";
import { setTimeout as delay } from "timers/promises";

let dir: string;
let runner: string;
let children: ChildProcessWithoutNullStreams[];

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "aide-lock-process-"));
  children = [];
  runner = join(dir, "installer.ts");
  const module = pathToFileURL(
    join(import.meta.dirname, "../lib/binary-download-lock.ts"),
  ).href;
  writeFileSync(
    runner,
    `
    import { acquireBinaryDownloadLock } from ${JSON.stringify(module)};
    const release = await acquireBinaryDownloadLock(process.argv[2]);
    console.log("locked");
    for await (const chunk of process.stdin) { /* hold until stdin closes */ }
    release();
  `,
  );
});

afterEach(async () => {
  await Promise.all(
    children.map(async (child) => {
      if (child.exitCode === null && child.signalCode === null) {
        const closed = once(child, "close");
        child.kill("SIGKILL");
        await closed;
      }
    }),
  );
  rmSync(dir, { recursive: true, force: true });
});

function installer() {
  const child = spawn("bun", [runner, dir], { stdio: "pipe" });
  children.push(child);
  let stderr = "";
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });
  const locked = new Promise<void>((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => {
      reject(new Error(`Installer exited ${code}: ${stderr}`));
    });
    child.stdout.once("data", () => {
      resolve();
    });
  });
  return { child, locked };
}

it("allows only one installer process at a time", async () => {
  const first = installer();
  await first.locked;
  const second = installer();
  let acquired = false;
  void second.locked.then(() => {
    acquired = true;
  });
  await delay(150);
  expect(acquired).toBe(false);
  const closed = once(first.child, "close");
  first.child.stdin.end();
  await closed;
  await second.locked;
  expect(readdirSync(join(dir, ".aide-download.lock"))).toHaveLength(1);
  const secondClosed = once(second.child, "close");
  second.child.stdin.end();
  await secondClosed;
  expect(readdirSync(dir)).toEqual(["installer.ts"]);
}, 10000);

it("recovers ownership after the installer is killed", async () => {
  const first = installer();
  await first.locked;
  const closed = once(first.child, "close");
  first.child.kill("SIGKILL");
  await closed;
  const second = installer();
  await second.locked;
  const secondClosed = once(second.child, "close");
  second.child.stdin.end();
  await secondClosed;
  expect(readdirSync(dir)).toEqual(["installer.ts"]);
}, 10000);
