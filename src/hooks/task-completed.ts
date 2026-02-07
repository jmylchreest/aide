#!/usr/bin/env node
/**
 * Task Completed Hook (TaskCompleted)
 *
 * Validates SDLC stage completion before allowing tasks to be marked complete.
 * Parses task subject for [story-id][STAGE] pattern and runs stage-specific checks.
 *
 * Stage validations:
 * - DESIGN: Check for design output (decisions, interfaces)
 * - TEST: Check that test files exist
 * - DEV: Check that tests pass
 * - VERIFY: Full suite green, lint clean
 * - DOCS: Check that docs were updated
 *
 * Exit codes:
 * - 0: Allow completion
 * - 2: Block completion (stderr fed back as feedback)
 */

import { execSync } from "child_process";
import { existsSync, readFileSync } from "fs";
import { join } from "path";
import { debug, setDebugCwd } from "../lib/logger.js";
import { readStdin } from "../lib/hook-utils.js";

const SOURCE = "task-completed";

// Safety limit for regex parsing
const MAX_SUBJECT_LENGTH = 1000;

interface HookInput {
  hook_event_name: "TaskCompleted";
  session_id: string;
  cwd: string;
  task_id: string;
  task_subject: string;
  task_description?: string;
  teammate_name?: string;
  team_name?: string;
}

interface StageInfo {
  storyId: string;
  stage: string;
}

/**
 * Parse task subject for SDLC stage pattern
 * Expected: [story-id][STAGE] Description
 */
function parseStageFromSubject(subject: string): StageInfo | null {
  // Safety check for regex
  if (subject.length > MAX_SUBJECT_LENGTH) {
    debug(SOURCE, `Subject too long for parsing (${subject.length} chars)`);
    return null;
  }

  // Match patterns like:
  // [story-auth][DESIGN] Design auth module
  // [Story-1][DEV] Implement feature
  const match = subject.match(/\[([^\]]+)\]\[([A-Z]+)\]/i);
  if (!match) return null;

  return {
    storyId: match[1],
    stage: match[2].toUpperCase(),
  };
}

/**
 * Check if a command succeeds
 */
function commandSucceeds(cmd: string, cwd: string): boolean {
  try {
    execSync(cmd, { cwd, stdio: "pipe", timeout: 60000 });
    return true;
  } catch {
    return false;
  }
}

/**
 * Get command output or null on failure
 */
function getCommandOutput(cmd: string, cwd: string): string | null {
  try {
    return execSync(cmd, { cwd, stdio: "pipe", timeout: 30000 })
      .toString()
      .trim();
  } catch {
    return null;
  }
}

/**
 * Detect project type (typescript, go, python)
 */
function detectProjectType(
  cwd: string,
): "typescript" | "go" | "python" | "unknown" {
  if (existsSync(join(cwd, "package.json"))) return "typescript";
  if (existsSync(join(cwd, "go.mod"))) return "go";
  if (
    existsSync(join(cwd, "pyproject.toml")) ||
    existsSync(join(cwd, "setup.py"))
  )
    return "python";
  return "unknown";
}

/**
 * Validate DESIGN stage completion
 */
function validateDesign(
  cwd: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  // Check if any decisions were recorded for this story
  // This is a soft check - design output is hard to validate programmatically
  debug(SOURCE, `Validating DESIGN for ${storyId}`);

  // For now, just pass - design validation is subjective
  // Could be enhanced to check for design doc files or decisions
  return { ok: true };
}

/**
 * Validate TEST stage completion
 */
function validateTest(
  cwd: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  debug(SOURCE, `Validating TEST for ${storyId}`);

  const projectType = detectProjectType(cwd);

  // Check if test files exist (recently modified)
  const testPatterns: Record<string, string[]> = {
    typescript: [
      "**/*.test.ts",
      "**/*.spec.ts",
      "**/*.test.tsx",
      "**/*.spec.tsx",
    ],
    go: ["**/*_test.go"],
    python: ["**/test_*.py", "**/*_test.py"],
    unknown: [],
  };

  // For now, just pass - test file existence is hard to validate without grep
  // The real validation happens in DEV stage (tests must pass)
  return { ok: true };
}

/**
 * Validate DEV stage completion
 */
function validateDev(
  cwd: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  debug(SOURCE, `Validating DEV for ${storyId}`);

  const projectType = detectProjectType(cwd);

  // Run tests based on project type
  const testCommands: Record<string, string> = {
    typescript: "npm test",
    go: "go test ./...",
    python: "pytest",
    unknown: "",
  };

  const testCmd = testCommands[projectType];
  if (!testCmd) {
    debug(SOURCE, "Unknown project type, skipping test validation");
    return { ok: true };
  }

  // Check if test command exists in package.json scripts
  if (projectType === "typescript") {
    try {
      const pkgPath = join(cwd, "package.json");
      if (!existsSync(pkgPath)) return { ok: true };
      const pkgJson = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (!pkgJson?.scripts?.test) {
        debug(SOURCE, "No test script defined, skipping");
        return { ok: true };
      }
    } catch (err) {
      debug(SOURCE, `Failed to read package.json: ${err}`);
      return { ok: true };
    }
  }

  if (!commandSucceeds(testCmd, cwd)) {
    return {
      ok: false,
      reason: `Tests are failing. Run \`${testCmd}\` and fix failures before completing DEV stage.`,
    };
  }

  return { ok: true };
}

/**
 * Validate VERIFY stage completion
 */
function validateVerify(
  cwd: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  debug(SOURCE, `Validating VERIFY for ${storyId}`);

  const projectType = detectProjectType(cwd);
  const failures: string[] = [];

  // Test validation
  const testCommands: Record<string, string> = {
    typescript: "npm test",
    go: "go test ./...",
    python: "pytest",
    unknown: "",
  };

  const testCmd = testCommands[projectType];
  if (testCmd && !commandSucceeds(testCmd, cwd)) {
    failures.push(`Tests failing: run \`${testCmd}\``);
  }

  // Lint validation
  const lintCommands: Record<string, string> = {
    typescript: "npm run lint",
    go: "go vet ./...",
    python: "ruff check .",
    unknown: "",
  };

  const lintCmd = lintCommands[projectType];
  if (lintCmd) {
    // Check if lint script exists for typescript
    if (projectType === "typescript") {
      try {
        const pkgPath = join(cwd, "package.json");
        if (existsSync(pkgPath)) {
          const pkgJson = JSON.parse(readFileSync(pkgPath, "utf-8"));
          if (pkgJson.scripts?.lint && !commandSucceeds(lintCmd, cwd)) {
            failures.push(`Lint errors: run \`${lintCmd}\``);
          }
        }
      } catch {
        /* ignore */
      }
    } else if (!commandSucceeds(lintCmd, cwd)) {
      failures.push(`Lint errors: run \`${lintCmd}\``);
    }
  }

  // Type check validation (TypeScript only)
  if (projectType === "typescript") {
    if (!commandSucceeds("npx tsc --noEmit", cwd)) {
      failures.push("Type errors: run `npx tsc --noEmit`");
    }
  }

  // Build validation
  const buildCommands: Record<string, string> = {
    typescript: "npm run build",
    go: "go build ./...",
    python: "",
    unknown: "",
  };

  const buildCmd = buildCommands[projectType];
  if (buildCmd) {
    if (projectType === "typescript") {
      try {
        const pkgPath = join(cwd, "package.json");
        if (existsSync(pkgPath)) {
          const pkgJson = JSON.parse(readFileSync(pkgPath, "utf-8"));
          if (pkgJson.scripts?.build && !commandSucceeds(buildCmd, cwd)) {
            failures.push(`Build failing: run \`${buildCmd}\``);
          }
        }
      } catch {
        /* ignore */
      }
    } else if (!commandSucceeds(buildCmd, cwd)) {
      failures.push(`Build failing: run \`${buildCmd}\``);
    }
  }

  if (failures.length > 0) {
    return {
      ok: false,
      reason: `VERIFY stage incomplete:\n${failures.map((f) => `- ${f}`).join("\n")}`,
    };
  }

  return { ok: true };
}

/**
 * Validate DOCS stage completion
 */
function validateDocs(
  cwd: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  debug(SOURCE, `Validating DOCS for ${storyId}`);

  // Documentation validation is subjective
  // Could check for recently modified .md files or doc comments
  // For now, just pass
  return { ok: true };
}

/**
 * Main validation dispatcher
 */
function validateStage(
  cwd: string,
  stage: string,
  storyId: string,
): { ok: boolean; reason?: string } {
  switch (stage) {
    case "DESIGN":
      return validateDesign(cwd, storyId);
    case "TEST":
      return validateTest(cwd, storyId);
    case "DEV":
      return validateDev(cwd, storyId);
    case "VERIFY":
      return validateVerify(cwd, storyId);
    case "DOCS":
      return validateDocs(cwd, storyId);
    case "FIX":
      // FIX stage just needs to pass - it's a remediation stage
      return { ok: true };
    default:
      debug(SOURCE, `Unknown stage: ${stage}, allowing completion`);
      return { ok: true };
  }
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      process.exit(0);
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();

    setDebugCwd(cwd);
    debug(SOURCE, `TaskCompleted: ${data.task_subject}`);

    // Parse stage from task subject
    const stageInfo = parseStageFromSubject(data.task_subject);

    if (!stageInfo) {
      // Not an SDLC task, allow completion
      debug(SOURCE, "Not an SDLC task, allowing completion");
      process.exit(0);
    }

    debug(
      SOURCE,
      `SDLC task: story=${stageInfo.storyId}, stage=${stageInfo.stage}`,
    );

    // Validate the stage
    const result = validateStage(cwd, stageInfo.stage, stageInfo.storyId);

    if (!result.ok) {
      // Block completion - stderr is fed back to the agent
      console.error(result.reason);
      process.exit(2);
    }

    // Allow completion
    debug(SOURCE, `Stage ${stageInfo.stage} validation passed`);
    process.exit(0);
  } catch (err) {
    debug(SOURCE, `Error: ${err}`);
    // On error, allow completion (don't block on hook failures)
    process.exit(0);
  }
}

main();
