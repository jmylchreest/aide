#!/usr/bin/env node
/**
 * Reflect Hook (Stop)
 *
 * Runs the instinct parser catalogue against the session's observe events
 * when AIDE_REFLECT is truthy (1/true/on/yes). Off by default.
 *
 * Fire-and-forget: never blocks Stop, never returns an error to the harness.
 */

import { execFileSync } from "child_process";
import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { debug } from "../lib/logger.js";

const SOURCE = "reflect";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
}

async function main(): Promise<void> {
  try {
    // The CLI itself checks env + .aide/config/aide.json reflect.enabled
    // and no-ops when disabled, so the hook can invoke unconditionally. This
    // is a 1-process spawn at session end — negligible overhead even when
    // disabled.

    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({}));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionID = data.session_id;
    if (!sessionID) {
      console.log(JSON.stringify({}));
      return;
    }

    const binary = findAideBinary({ cwd });
    if (!binary) {
      console.log(JSON.stringify({}));
      return;
    }

    try {
      execFileSync(binary, ["reflect", "run", `--session=${sessionID}`], {
        cwd,
        timeout: 10000,
        stdio: ["pipe", "pipe", "pipe"],
      });
      debug(SOURCE, `reflect run session=${sessionID} ok`);
    } catch (err) {
      debug(SOURCE, `reflect run failed (non-fatal): ${err}`);
    }

    console.log(JSON.stringify({}));
  } catch (err) {
    debug(SOURCE, `error: ${err}`);
    console.log(JSON.stringify({}));
  }
}

void main();
