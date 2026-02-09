#!/usr/bin/env node
/**
 * Agent Cleanup Hook (Stop)
 *
 * Cleans up agent-specific state when an agent stops.
 * This prevents stale state from polluting future agents with the same ID.
 *
 * Runs after persistence hook to clean up when agent is allowed to stop.
 */

import { existsSync, appendFileSync } from "fs";
import { join } from "path";
import { readStdin } from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { cleanupAgent } from "../core/cleanup.js";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  agent_id?: string;
  transcript_path?: string;
  permission_mode?: string;
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const agentId = data.agent_id || data.session_id;

    // Clean up agent-specific state — delegates to core
    if (agentId) {
      const binary = findAideBinary({
        cwd,
        pluginRoot: process.env.CLAUDE_PLUGIN_ROOT,
      });
      if (binary) {
        const cleared = cleanupAgent(binary, cwd, agentId);
        if (cleared) {
          const logDir = join(cwd, ".aide", "_logs");
          if (existsSync(logDir)) {
            const logPath = join(logDir, "agent-cleanup.log");
            const timestamp = new Date().toISOString();
            appendFileSync(
              logPath,
              `${timestamp} Cleaned up state for agent: ${agentId}\n`,
            );
          }
        }
      }
    }

    // Always continue - cleanup is best-effort
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, still allow continuation
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
