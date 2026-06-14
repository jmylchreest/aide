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

import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
} from "../lib/hook-utils.js";
import { debug } from "../lib/logger.js";
import { checkSearchEnrichment } from "../core/search-enrichment.js";
import { findAideBinary } from "../core/aide-client.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

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
      emitHookResult({ continue: true });
      return;
    }

    const data: HookInput = JSON.parse(input);
    const toolName = data.tool_name || "";
    const toolInput = data.tool_input || {};
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || "";

    const binary = findAideBinary({
      cwd,
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });

    const result = checkSearchEnrichment(toolName, toolInput, cwd, binary);

    if (result.shouldEnrich && result.enrichment) {
      debug(SOURCE, `Enriching grep with code index context`);

      if (binary) {
        try {
          // `name: "enrichment"` is load-bearing: the TokensPage by_delivery
          // rollup keys on Event.Name via observeToTokenEvent.
          emitInjectionEvent(binary, cwd, {
            source: SOURCE,
            subtype: "enrichment",
            name: "enrichment",
            content: result.enrichment,
            sessionId,
            attrs: { tool: toolName },
          });
        } catch {
          // Non-fatal
        }
      }

      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: result.enrichment,
        },
      };
      emitHookResult(output);
    } else {
      emitHookResult({ continue: true });
    }
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
    emitHookResult({ continue: true });
  }
}

installHookSafetyNet(SOURCE);

main();
