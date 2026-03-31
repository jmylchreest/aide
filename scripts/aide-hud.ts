#!/usr/bin/env bun
/**
 * aide-hud.ts - Status line display for aide plugin
 *
 * Reads HUD state from .aide/state/hud.txt and outputs formatted status.
 * Falls back to minimal display if state doesn't exist.
 *
 * Cross-platform TypeScript replacement for aide-hud.sh.
 */

import { existsSync, readFileSync } from "fs";
import { join, dirname } from "path";

function findProjectRoot(): string {
  let dir = process.cwd();
  while (true) {
    if (existsSync(join(dir, ".aide"))) {
      return dir;
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return process.cwd();
}

const projectRoot = findProjectRoot();
const hudFile = join(projectRoot, ".aide", "state", "hud.txt");

if (existsSync(hudFile)) {
  try {
    const content = readFileSync(hudFile, "utf-8").trim();
    if (content) {
      console.log(content);
      process.exit(0);
    }
  } catch {
    // fall through to default
  }
}

// No state at all - show minimal status
console.log("[aide] idle");
