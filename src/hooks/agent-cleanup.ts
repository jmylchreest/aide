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
import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
} from "../lib/hook-utils.js";
import { cleanupAgent } from "../core/cleanup.js";
import { debug } from "../lib/logger.js";
import { findProjectRoot } from "../lib/project-root.js";

const SOURCE = "agent-cleanup";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const agentId = data.agent_id || data.session_id;

    // Clean up agent-specific state — delegates to core
    if (agentId) {
      const binary = findAideBinary(cwd, data.session_id);
      if (binary) {
        const cleared = cleanupAgent(binary, cwd, agentId);
        if (cleared) {
          const { root } = findProjectRoot(cwd);
          const logDir = join(root, ".aide", "_logs");
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
    emitHookResult({ continue: true });
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult({ continue: true });
  }
}

installHookSafetyNet(SOURCE);

main();
