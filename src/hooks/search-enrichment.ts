#!/usr/bin/env node
/**
 * Search Enrichment Hook (PreToolUse)
 *
 * Enriches Grep tool calls with code index context — symbol definitions,
 * file locations, and reference counts. This gives the agent structural
 * awareness without additional tool calls.
 *
 * This is a soft advisory — it never blocks, only injects additive context.
 *
 * Core logic is in src/core/search-enrichment.ts for cross-platform reuse.
 */

import { readStdin } from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { checkSearchEnrichment } from "../core/search-enrichment.js";
import { findAideBinary } from "../core/aide-client.js";

const SOURCE = "search-enrichment";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  agent_name?: string;
  agent_id?: string;
  tool_input?: Record<string, unknown>;
}

interface HookOutput {
  continue: boolean;
  message?: string;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();

    const binary = findAideBinary({
      cwd,
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });

    const result = checkSearchEnrichment(toolName, toolInput, cwd, binary);

    if (result.shouldEnrich && result.enrichment) {
      debug(SOURCE, `Enriching grep with code index context`);
      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.enrichment,
        },
      };
      console.log(JSON.stringify(output));
    } else {
      console.log(JSON.stringify({ continue: true }));
    }
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
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
