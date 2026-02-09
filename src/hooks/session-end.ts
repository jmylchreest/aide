#!/usr/bin/env node
/**
 * Session End Hook (SessionEnd)
 *
 * Called when a Claude Code session ends cleanly.
 * Performs final cleanup and records session metrics.
 *
 * SessionEnd data from Claude Code:
 * - session_id, cwd
 * - duration (optional)
 */

import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { cleanupSession as coreCleanupSession } from "../core/cleanup.js";

interface SessionEndInput {
  event: "SessionEnd";
  session_id: string;
  cwd: string;
  duration?: number;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: SessionEndInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "unknown";

    // Cleanup session — delegates to core
    const binary = findAideBinary({
      cwd,
      pluginRoot: process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (binary) {
      coreCleanupSession(binary, cwd, sessionId, data.duration);
    }

    // Always continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, continue anyway
    console.log(JSON.stringify({ continue: true }));
  }
}


process.on("uncaughtException", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});
process.on("unhandledRejection", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});

main();
