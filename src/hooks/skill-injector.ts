#!/usr/bin/env node
/**
 * Skill Injector Hook (UserPromptSubmit)
 *
 * Dynamically discovers and injects relevant skills based on prompt triggers.
 * Searches both built-in skills and project-local .aide/skills/
 *
 * Features:
 * - Recursive skill discovery
 * - YAML frontmatter parsing for triggers
 * - Caching with file watcher invalidation
 * - Auto-creates .aide directories if needed
 *
 * Debug logging: Set AIDE_DEBUG=1 to enable tracing
 * Logs written to: .aide/_logs/startup.log
 */

import { existsSync, mkdirSync } from "fs";
import { join } from "path";
import { Logger, debug, setDebugCwd } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";
import {
  discoverSkills as coreDiscoverSkills,
  matchSkills as coreMatchSkills,
  formatSkillsContext as coreFormatSkillsContext,
} from "../core/skill-matcher.js";
import type { Skill } from "../core/types.js";

const SOURCE = "skill-injector";

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  prompt?: string;
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  continue: boolean;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

// Skill types, discovery, matching, and formatting are now in core
// Import aliases used below: coreDiscoverSkills, coreMatchSkills, coreFormatSkillsContext

// Module-level logger (initialized in main)
let log: Logger | null = null;

/**
 * Ensure .aide directories exist (minimal version for skill-injector)
 */
function ensureDirectories(cwd: string): void {
  const dirs = [
    join(cwd, ".aide"),
    join(cwd, ".aide", "skills"),
    join(cwd, ".aide", "config"),
    join(cwd, ".aide", "state"),
    join(cwd, ".aide", "memory"),
  ];

  for (const dir of dirs) {
    if (!existsSync(dir)) {
      try {
        mkdirSync(dir, { recursive: true });
      } catch (err) {
        debugLog(`Failed to create directory ${dir}: ${err}`);
      }
    }
  }
}

/**
 * Discover all skills — delegates to core
 */
function discoverSkills(cwd: string): Skill[] {
  log?.start("discoverSkills");
  const pluginRoot =
    process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT;
  const skills = coreDiscoverSkills(cwd, pluginRoot || undefined);
  log?.end("discoverSkills", { totalSkills: skills.length });
  return skills;
}

/**
 * Match skills to prompt — delegates to core
 */
function matchSkills(prompt: string, skills: Skill[], maxResults = 3): Skill[] {
  log?.start("matchSkills");
  const matched = coreMatchSkills(prompt, skills, maxResults);
  log?.end("matchSkills", { checked: skills.length, matched: matched.length });
  return matched;
}

/**
 * Format skills for injection — delegates to core
 */
function formatSkillsContext(skills: Skill[]): string {
  return coreFormatSkillsContext(skills);
}

// Debug helper - writes to debug.log (not stderr)
function debugLog(msg: string): void {
  debug(SOURCE, msg);
}

// Ensure we always output valid JSON, even on catastrophic errors
function outputContinue(): void {
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    // Last resort - raw JSON string
    console.log('{"continue":true}');
  }
}

// Global error handlers to prevent hook crashes without JSON output
process.on("uncaughtException", (err) => {
  debugLog(`UNCAUGHT EXCEPTION: ${err}`);
  outputContinue();
  process.exit(0);
});

process.on("unhandledRejection", (reason) => {
  debugLog(`UNHANDLED REJECTION: ${reason}`);
  outputContinue();
  process.exit(0);
});

async function main(): Promise<void> {
  const hookStart = Date.now();
  debugLog(`Hook started at ${new Date().toISOString()}`);

  try {
    debugLog("Reading stdin...");
    const input = await readStdin();
    debugLog(`Stdin read complete (${Date.now() - hookStart}ms)`);

    if (!input.trim()) {
      debugLog("Empty input, exiting");
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const prompt = data.prompt || "";
    const cwd = data.cwd || process.cwd();

    // Switch debug logging to project-local logs
    setDebugCwd(cwd);

    debugLog(`Parsed input: cwd=${cwd}, prompt=${prompt.length} chars`);

    // Initialize logger
    log = new Logger("skill-injector", cwd);
    log.start("total");
    log.debug(`Prompt length: ${prompt.length} chars`);
    debugLog(`Logger initialized, enabled=${log.isEnabled()}`);

    // Ensure .aide directories exist
    debugLog("ensureDirectories starting...");
    log.start("ensureDirectories");
    ensureDirectories(cwd);
    log.end("ensureDirectories");
    debugLog(`ensureDirectories complete (${Date.now() - hookStart}ms)`);

    if (!prompt) {
      debugLog("No prompt provided, exiting");
      log.info("No prompt provided");
      log.end("total");
      log.flush();
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    // Discover and match skills
    debugLog("discoverSkills starting...");
    const skills = discoverSkills(cwd);
    debugLog(
      `discoverSkills complete: ${skills.length} skills (${Date.now() - hookStart}ms)`,
    );

    debugLog("matchSkills starting...");
    const matched = matchSkills(prompt, skills);
    debugLog(
      `matchSkills complete: ${matched.length} matches (${Date.now() - hookStart}ms)`,
    );

    log.end("total");

    if (matched.length > 0) {
      log.info(
        `Injecting ${matched.length} skills: ${matched.map((s) => s.name).join(", ")}`,
      );
      debugLog(`Flushing logs...`);
      log.flush();
      debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);

      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          hookEventName: "UserPromptSubmit",
          additionalContext: formatSkillsContext(matched),
        },
      };
      console.log(JSON.stringify(output));
    } else {
      log.info("No matching skills");
      debugLog(`Flushing logs...`);
      log.flush();
      debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);
      console.log(JSON.stringify({ continue: true }));
    }
  } catch (error) {
    debugLog(`ERROR: ${error}`);
    // Log error if logger is available
    if (log) {
      log.error("Skill injection failed", error);
      log.flush();
    }
    // On error, allow continuation
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
