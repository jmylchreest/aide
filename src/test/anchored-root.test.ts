/**
 * The ambient session context is what lets root resolution deep in the call
 * graph reach the anchor without every signature carrying a session id.
 * These cover the contract that makes that safe: with a session it prefers
 * the anchor, without one it is exactly the walk it replaced.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, rmSync, existsSync } from "fs";
import { join } from "path";
import { tmpdir, homedir } from "os";
import { randomBytes } from "crypto";

import {
  anchoredRoot,
  setSessionContext,
  writeSessionAnchor,
  ANCHOR_SCHEMA_VERSION,
  type AnchorInfo,
} from "../lib/anchor.js";
import { findProjectRoot } from "../lib/project-root.js";

function makeAnchor(root: string): AnchorInfo {
  return {
    schemaVersion: ANCHOR_SCHEMA_VERSION,
    resolverVersion: "test",
    root,
    realRoot: root,
    hasMarker: true,
    source: "test",
    provenance: { marker: ".git" },
    identity: { projectName: "test", source: "test" },
    dbPath: join(root, ".aide", "memory", "memory.db"),
    socketPath: join(root, ".aide", "aide.sock"),
    chain: [{ root, realRoot: root, relation: "self" }],
  };
}

describe("anchoredRoot", () => {
  let dir: string;
  let sessionId: string;

  beforeEach(() => {
    dir = join(tmpdir(), `aide-anchored-${Date.now()}-${randomBytes(4).toString("hex")}`);
    mkdirSync(join(dir, ".git"), { recursive: true });
    sessionId = `test-${randomBytes(6).toString("hex")}`;
  });

  afterEach(() => {
    setSessionContext("");
    rmSync(dir, { recursive: true, force: true });
    for (const base of [process.env.XDG_RUNTIME_DIR, homedir()]) {
      if (!base) continue;
      const p = join(base, base === homedir() ? ".aide" : "aide", "anchors", `${sessionId}.json`);
      if (existsSync(p)) rmSync(p, { force: true });
    }
  });

  it("matches the plain walk when no session is set", () => {
    setSessionContext("");
    expect(anchoredRoot(dir)).toEqual({
      root: findProjectRoot(dir).root,
      hasMarker: findProjectRoot(dir).hasMarker,
    });
  });

  it("prefers the cached anchor over the walk", () => {
    // A root the walk cannot reach from `dir`, so only the anchor can produce it.
    const anchored = join(dir, "nested-root");
    mkdirSync(anchored, { recursive: true });
    writeSessionAnchor(sessionId, dir, makeAnchor(anchored));
    setSessionContext(sessionId);

    expect(anchoredRoot(dir).root).toBe(anchored);
    expect(findProjectRoot(dir).root).not.toBe(anchored);
  });

  it("falls back to the walk when the session has no anchor", () => {
    setSessionContext(`absent-${randomBytes(6).toString("hex")}`);

    expect(anchoredRoot(dir).root).toBe(findProjectRoot(dir).root);
  });
});
