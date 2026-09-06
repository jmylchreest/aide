/**
 * writeHudOutput creates .aide/state under the resolved project root, so it
 * carries the same risk skill-injector did: bootstrapping an unmarked
 * directory plants a project root wherever a hook happened to run, and every
 * later walk-up beneath it resolves there.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { existsSync, mkdirSync, rmSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { randomBytes } from "crypto";

import { writeHudOutput } from "../lib/hud.js";

describe("writeHudOutput project-marker gate", () => {
  let dir: string;

  beforeEach(() => {
    dir = join(tmpdir(), `aide-hud-${Date.now()}-${randomBytes(4).toString("hex")}`);
    mkdirSync(dir);
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("writes the HUD under a marked project root", () => {
    mkdirSync(join(dir, ".git"));

    writeHudOutput(dir, "hud line");

    expect(existsSync(join(dir, ".aide", "state", "hud.txt"))).toBe(true);
  });

  it("leaves an unmarked directory alone", () => {
    writeHudOutput(dir, "hud line");

    expect(existsSync(join(dir, ".aide"))).toBe(false);
  });
});
