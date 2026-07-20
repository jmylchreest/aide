/**
 * Tests for the TS anchor reader (src/lib/anchor.ts): session-cache
 * round-trip, validation rejections, binary shell-out, TTL sweep, and the
 * getAnchoredRoot fallback ladder.
 *
 * Run with: npx vitest run src/test/anchor.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  mkdtempSync,
  rmSync,
  mkdirSync,
  writeFileSync,
  chmodSync,
  existsSync,
  readFileSync,
  utimesSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

let tempHome = "";

vi.mock("os", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("os");
  return {
    ...actual,
    homedir: () => tempHome,
  };
});

import {
  ANCHOR_SCHEMA_VERSION,
  getAnchor,
  getAnchoredRoot,
  readSessionAnchor,
  resolveAnchorViaBinary,
  sweepAnchorCache,
  writeSessionAnchor,
  type AnchorInfo,
} from "../lib/anchor.js";

function makeAnchor(root: string, overrides: Partial<AnchorInfo> = {}): AnchorInfo {
  return {
    schemaVersion: ANCHOR_SCHEMA_VERSION,
    resolverVersion: "test",
    root,
    realRoot: root,
    hasMarker: true,
    source: "walk",
    provenance: { marker: ".git", gitdirShape: "directory" },
    identity: { projectName: "proj", source: "basename" },
    dbPath: join(root, ".aide", "memory", "memory.db"),
    socketPath: join(root, ".aide", "aide.sock"),
    chain: [{ root, realRoot: root, relation: "self" }],
    ...overrides,
  };
}

describe("anchor reader", () => {
  let tmp: string;
  let projectRoot: string;

  beforeEach(() => {
    tmp = mkdtempSync(join(tmpdir(), "aide-anchor-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-anchor-home-"));
    projectRoot = join(tmp, "proj");
    mkdirSync(join(projectRoot, ".aide"), { recursive: true });
    delete process.env.AIDE_PROJECT_ROOT;
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
    delete process.env.AIDE_PROJECT_ROOT;
  });

  it("round-trips through the session cache and project copy", () => {
    const anchor = makeAnchor(projectRoot);
    writeSessionAnchor("sess-1", projectRoot, anchor);

    const back = readSessionAnchor("sess-1", projectRoot);
    expect(back?.root).toBe(projectRoot);
    expect(back?.chain[0].relation).toBe("self");

    const projectCopy = join(projectRoot, ".aide", "state", "anchor.json");
    expect(existsSync(projectCopy)).toBe(true);
    const entry = JSON.parse(readFileSync(projectCopy, "utf-8"));
    expect(entry.anchor.root).toBe(projectRoot);
    expect(entry.launchCwd).toBe(projectRoot);
  });

  it("never creates .aide for the project copy", () => {
    const bareRoot = join(tmp, "bare");
    mkdirSync(bareRoot, { recursive: true });
    writeSessionAnchor("sess-2", bareRoot, makeAnchor(bareRoot));

    expect(existsSync(join(bareRoot, ".aide"))).toBe(false);
    // Session cache still written.
    expect(readSessionAnchor("sess-2", bareRoot)?.root).toBe(bareRoot);
  });

  it("rejects a cache entry from a different launch cwd", () => {
    writeSessionAnchor("sess-3", projectRoot, makeAnchor(projectRoot));
    expect(readSessionAnchor("sess-3", join(tmp, "elsewhere"))).toBeNull();
  });

  it("rejects a cache entry whose root no longer exists", () => {
    const goneRoot = join(tmp, "gone");
    mkdirSync(goneRoot, { recursive: true });
    writeSessionAnchor("sess-4", goneRoot, makeAnchor(goneRoot));
    rmSync(goneRoot, { recursive: true, force: true });

    expect(readSessionAnchor("sess-4", goneRoot)).toBeNull();
  });

  it("rejects wrong schema versions and malformed session ids", () => {
    writeSessionAnchor(
      "sess-5",
      projectRoot,
      makeAnchor(projectRoot, { schemaVersion: 99 }),
    );
    expect(readSessionAnchor("sess-5", projectRoot)).toBeNull();
    expect(readSessionAnchor("../../etc/passwd", projectRoot)).toBeNull();
  });

  it("resolveAnchorViaBinary parses the binary's JSON and validates it", () => {
    const binary = join(tmp, "aide-stub");
    const anchor = makeAnchor(projectRoot);
    writeFileSync(
      binary,
      `#!/usr/bin/env bun\nconsole.log(JSON.stringify(${JSON.stringify(anchor)}));\n`,
    );
    chmodSync(binary, 0o755);

    expect(resolveAnchorViaBinary(binary, projectRoot)?.root).toBe(projectRoot);

    const badBinary = join(tmp, "aide-bad");
    writeFileSync(badBinary, `#!/usr/bin/env bun\nconsole.log("not json");\n`);
    chmodSync(badBinary, 0o755);
    expect(resolveAnchorViaBinary(badBinary, projectRoot)).toBeNull();
  });

  it("getAnchor prefers the session cache over the binary", () => {
    writeSessionAnchor("sess-6", projectRoot, makeAnchor(projectRoot));
    const explodingBinary = join(tmp, "aide-exploding");
    writeFileSync(explodingBinary, `#!/usr/bin/env bun\nprocess.exit(1);\n`);
    chmodSync(explodingBinary, 0o755);

    const got = getAnchor({
      sessionId: "sess-6",
      cwd: projectRoot,
      binary: explodingBinary,
    });
    expect(got?.root).toBe(projectRoot);
  });

  it("getAnchoredRoot falls back to the TS walk when cache and binary fail", () => {
    mkdirSync(join(projectRoot, ".git"), { recursive: true });
    const sub = join(projectRoot, "pkg");
    mkdirSync(sub, { recursive: true });

    const result = getAnchoredRoot({ cwd: sub, binary: null });
    expect(result.anchor).toBeNull();
    expect(result.root).toBe(projectRoot);
    expect(result.hasMarker).toBe(true);
  });

  it("sweeps session caches past the TTL", () => {
    writeSessionAnchor("sess-old", projectRoot, makeAnchor(projectRoot));
    writeSessionAnchor("sess-new", projectRoot, makeAnchor(projectRoot));

    const oldPath = join(tempHome, ".aide", "anchors", "sess-old.json");
    const past = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000);
    utimesSync(oldPath, past, past);

    sweepAnchorCache();

    expect(existsSync(oldPath)).toBe(false);
    expect(readSessionAnchor("sess-new", projectRoot)?.root).toBe(projectRoot);
  });
});
