#!/usr/bin/env node
/**
 * HUD Updater Hook (PostToolUse)
 *
 * Updates the terminal status line with current aide state.
 * Shows: mode, model tier, active agents, context usage
 *
 * Output is written to .aide/state/hud.txt for the terminal to display.
 */

import { Logger, debug } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";

const SOURCE = "hud-updater";
import { findAideBinary } from "../core/aide-client.js";
import { updateToolStats } from "../core/tool-tracking.js";
import {
  getAgentStates,
  loadHudConfig,
  getSessionState,
  formatHud,
  writeHudOutput,
} from "../lib/hud.js";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_id?: string;
  tool_result?: {
    success: boolean;
    duration?: number;
  };
  transcript_path?: string;
  permission_mode?: string;
}

async function main(): Promise<void> {
  let log: Logger | null = null;

  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const toolName = data.tool_name || "";
    const agentId = data.agent_id || data.session_id;
    const sessionId = data.session_id;

    // Initialize logger
    log = new Logger("hud-updater", cwd);
    log.start("total");
    log.debug(
      `Processing PostToolUse for tool: ${toolName}, agent: ${agentId}, session: ${sessionId}`,
    );

    // Update session state (per-agent tracking) — delegates to core
    if (toolName) {
      log.start("updateSessionState");
      const binary = findAideBinary({
        cwd,
        pluginRoot:
          process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
      });
      if (binary) {
        updateToolStats(binary, cwd, toolName, agentId);
      }
      log.end("updateSessionState");
    }

    // Load config and get state
    log.start("loadHudConfig");
    const config = loadHudConfig(cwd);
    log.end("loadHudConfig");

    log.start("getSessionState");
    const state = getSessionState(cwd);
    log.end("getSessionState", state);

    log.start("getAgentStates");
    const allAgents = getAgentStates(cwd);
    // Filter to ONLY show agents from the current session
    const agents = sessionId
      ? allAgents.filter((a) => a.session === sessionId)
      : [];
    log.end("getAgentStates", {
      total: allAgents.length,
      filtered: agents.length,
    });

    // Format and write HUD (includes per-agent lines)
    log.start("formatHud");
    const hudOutput = formatHud(config, state, agents, cwd);
    log.end("formatHud");
    log.debug(`HUD output: ${hudOutput}`);

    log.start("writeHudOutput");
    writeHudOutput(cwd, hudOutput);
    log.end("writeHudOutput");

    log.end("total");
    log.flush();

    // Always continue
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    if (log) {
      log.error("HUD update failed", error);
      log.flush();
    }
    console.log(JSON.stringify({ continue: true }));
  }
}

process.on("uncaughtException", (err) => {
  debug(SOURCE, `UNCAUGHT EXCEPTION: ${err}`);
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    console.log('{"continue":true}');
  }
  process.exit(0);
});
process.on("unhandledRejection", (reason) => {
  debug(SOURCE, `UNHANDLED REJECTION: ${reason}`);
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    console.log('{"continue":true}');
  }
  process.exit(0);
});

main();
