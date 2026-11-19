import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { execFileSync } from "child_process";
import { downloadAideBinary } from "../lib/aide-downloader.js";
import { acquireBinaryDownloadLock } from "../lib/binary-download-lock.js";

vi.mock("child_process", async (original) => ({
  ...(await original<typeof import("child_process")>()),
  execFileSync: vi.fn(),
}));

let dir: string;
const binaryName = process.platform === "win32" ? "aide.exe" : "aide";
const options = { pluginVersion: "0.2.1", quiet: true };

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "aide-download-test-"));
  // Fake only executable invocation. Real files, locks, streams and atomic
  // replacement exercise preservation of the installed executable.
  vi.mocked(execFileSync).mockImplementation((file) => {
    const contents = readFileSync(String(file), "utf8");
    if (!contents.startsWith("aide version "))
      throw new Error("invalid binary");
    return Buffer.from(contents);
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  rmSync(dir, { recursive: true, force: true });
});

function installed(version = "0.1.18") {
  const contents = `aide version ${version}`;
  writeFileSync(join(dir, binaryName), contents);
  return contents;
}

describe("binary installation", () => {
  it("preserves the working binary when the network fails", async () => {
    const old = installed();
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    expect((await downloadAideBinary(dir, options)).success).toBe(false);
    expect(readFileSync(join(dir, binaryName), "utf8")).toBe(old);
    expect(readdirSync(dir)).toEqual([binaryName]);
  });

  it("rejects an old latest fallback during the release publication window", async () => {
    const old = installed();
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(new Response(null, { status: 404 }))
      .mockResolvedValueOnce(new Response("aide version 0.1.18"));
    vi.stubGlobal("fetch", fetch);
    const result = await downloadAideBinary(dir, options);
    expect(result.message).toContain("older than required v0.2.1");
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(readFileSync(join(dir, binaryName), "utf8")).toBe(old);
    expect(readdirSync(dir)).toEqual([binaryName]);
  });

  it("rejects a corrupt download and removes staging files", async () => {
    const old = installed();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("not a binary")),
    );
    expect((await downloadAideBinary(dir, options)).success).toBe(false);
    expect(readFileSync(join(dir, binaryName), "utf8")).toBe(old);
    expect(readdirSync(dir)).toEqual([binaryName]);
  });

  it("serializes simultaneous hook and MCP downloads and rechecks the version", async () => {
    const old = installed();
    let finish!: (response: Response) => void;
    let started!: () => void;
    const fetching = new Promise<void>((resolve) => {
      started = resolve;
    });
    const fetch = vi.fn(() => {
      started();
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    });
    vi.stubGlobal("fetch", fetch);
    const first = downloadAideBinary(dir, options);
    await fetching;
    const second = downloadAideBinary(dir, options);
    expect(readFileSync(join(dir, binaryName), "utf8")).toBe(old);
    finish(new Response("aide version 0.2.1"));
    const results = await Promise.all([first, second]);
    expect(results.every((result) => result.success)).toBe(true);
    expect(results.map((result) => result.cached)).toEqual([false, true]);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(readFileSync(join(dir, binaryName), "utf8")).toBe(
      "aide version 0.2.1",
    );
    expect(readdirSync(dir)).toEqual([binaryName]);
  });

  it.each(["0.3.0", "0.2.2+abc123"])(
    "does not downgrade newer executable %s when an older session starts",
    async (version) => {
      installed(version);
      const fetch = vi.fn();
      vi.stubGlobal("fetch", fetch);
      expect((await downloadAideBinary(dir, options)).cached).toBe(true);
      expect(fetch).not.toHaveBeenCalled();
    },
  );
});

describe("download ownership", () => {
  it("an obsolete cleanup cannot remove a successor before its PID write", async () => {
    const release = await acquireBinaryDownloadLock(dir);
    release();
    const lock = join(dir, ".aide-download.lock");
    mkdirSync(lock);
    release();
    expect(readdirSync(lock)).toEqual([]);
  });

  it("does not remove a legacy lock before its owner writes a PID", async () => {
    const lock = join(dir, ".aide-download.lock");
    mkdirSync(lock);
    await expect(acquireBinaryDownloadLock(dir, 0)).rejects.toThrow(
      "Timed out",
    );
    expect(readdirSync(lock)).toEqual([]);
  });

  it("never steals a live lock after timing out", async () => {
    const release = await acquireBinaryDownloadLock(dir);
    const lock = join(dir, ".aide-download.lock");
    const marker = readdirSync(lock);
    await expect(acquireBinaryDownloadLock(dir, 0)).rejects.toThrow(
      "Timed out",
    );
    expect(readdirSync(lock)).toEqual(marker);
    release();
    expect(readdirSync(dir)).toEqual([]);
  });

  it("reclaims a dead legacy owner without removing a successor's lock", async () => {
    const lock = join(dir, ".aide-download.lock");
    mkdirSync(lock);
    writeFileSync(join(lock, "pid"), "123456789");
    vi.spyOn(process, "kill").mockImplementation(() => {
      throw Object.assign(new Error("no process"), { code: "ESRCH" });
    });
    const release = await acquireBinaryDownloadLock(dir);
    release();
    const releaseNext = await acquireBinaryDownloadLock(dir);
    release(); // an obsolete cleanup must not delete the new owner
    expect(readdirSync(lock)).toHaveLength(1);
    releaseNext();
    expect(readdirSync(dir)).toEqual([]);
  });

  it("treats an inaccessible owner as alive", async () => {
    const lock = join(dir, ".aide-download.lock");
    mkdirSync(lock);
    writeFileSync(join(lock, "pid"), "123456789");
    vi.spyOn(process, "kill").mockImplementation(() => {
      throw Object.assign(new Error("not permitted"), { code: "EPERM" });
    });
    await expect(acquireBinaryDownloadLock(dir, 0)).rejects.toThrow(
      "Timed out",
    );
    expect(readFileSync(join(lock, "pid"), "utf8")).toBe("123456789");
  });
});
