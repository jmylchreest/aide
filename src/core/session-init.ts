/**
 * Session initialization logic — platform-agnostic.
 *
 * Extracted from src/hooks/session-start.ts.
 * Handles directory creation, config loading, session state,
 * memory fetching, and welcome context building.
 */

import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  readdirSync,
  unlinkSync,
  statSync,
} from "fs";
import { join } from "path";
import { execSync, execFileSync } from "child_process";
import { homedir } from "os";
import type {
  AideConfig,
  SessionState,
  SessionInitResult,
  MemoryInjection,
  StartupNotices,
} from "./types.js";
import { DEFAULT_CONFIG } from "./types.js";

/**
 * Ensure all .aide directories exist
 */
export function ensureDirectories(cwd: string): {
  created: number;
  existed: number;
} {
  const dirs = [
    join(cwd, ".aide"),
    join(cwd, ".aide", "skills"),
    join(cwd, ".aide", "config"),
    join(cwd, ".aide", "state"),
    join(cwd, ".aide", "memory"),
    join(cwd, ".aide", "worktrees"),
    join(cwd, ".aide", "_logs"),
    join(homedir(), ".aide"),
    join(homedir(), ".aide", "skills"),
    join(homedir(), ".aide", "config"),
  ];

  let created = 0;
  let existed = 0;

  for (const dir of dirs) {
    if (!existsSync(dir)) {
      try {
        mkdirSync(dir, { recursive: true });
        created++;
      } catch {
        // May not have permission
      }
    } else {
      existed++;
    }
  }

  // Ensure .gitignore exists in .aide directory.
  // Structure: exclude all local-only runtime data, allow shared/ and config/.
  // shared/ contains git-friendly markdown exports (decisions, memories).
  const gitignorePath = join(cwd, ".aide", ".gitignore");
  const requiredGitignoreContent = `# AIDE local runtime files - do not commit
# These are machine-specific and/or binary (non-mergeable)
_logs/
state/
bin/
worktrees/
memory/
code/

# Legacy top-level database
aide-memory.db

# Shared data IS committed (git-friendly markdown with frontmatter)
# See: aide share export / aide share import
!shared/
`;
  if (!existsSync(gitignorePath)) {
    try {
      writeFileSync(gitignorePath, requiredGitignoreContent);
    } catch {
      // Ignore
    }
  } else {
    // Migrate old gitignore format: ensure shared/ is allowed
    try {
      const existingContent = readFileSync(gitignorePath, "utf-8");
      if (!existingContent.includes("!shared/")) {
        const updatedContent =
          existingContent.trimEnd() +
          `\n
# Shared data IS committed (git-friendly markdown with frontmatter)
!shared/
`;
        writeFileSync(gitignorePath, updatedContent);
      }
    } catch {
      // Ignore
    }
  }

  return { created, existed };
}

/**
 * Get project name from git remote or directory name
 */
export function getProjectName(cwd: string): string {
  try {
    const remoteUrl = execSync("git config --get remote.origin.url", {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 2000,
    })
      .toString()
      .trim();

    const match = remoteUrl.match(/[/:]([^/]+?)(?:\.git)?$/);
    if (match) return match[1];
  } catch {
    // Not a git repo or no remote
  }

  return cwd.split("/").pop() || "unknown";
}

/**
 * Load config from .aide/config/aide.json (if it exists).
 * Returns DEFAULT_CONFIG if no config file exists or it can't be parsed.
 * Does NOT create a default config file — only user-set values are persisted.
 */
export function loadConfig(cwd: string): AideConfig {
  const configPath = join(cwd, ".aide", "config", "aide.json");

  if (existsSync(configPath)) {
    try {
      const content = readFileSync(configPath, "utf-8");
      return { ...DEFAULT_CONFIG, ...JSON.parse(content) };
    } catch {
      return DEFAULT_CONFIG;
    }
  }

  return DEFAULT_CONFIG;
}

/**
 * Initialize session state file
 */
export function initializeSession(
  sessionId: string,
  cwd: string,
): SessionState {
  const state: SessionState = {
    sessionId,
    startedAt: new Date().toISOString(),
    cwd,
    activeMode: null,
    agentCount: 0,
  };

  const statePath = join(cwd, ".aide", "state", "session.json");
  try {
    writeFileSync(statePath, JSON.stringify(state, null, 2));
  } catch {
    // Ignore
  }

  return state;
}

/**
 * Clean up stale state files older than 24 hours
 */
export function cleanupStaleStateFiles(cwd: string): {
  scanned: number;
  deleted: number;
} {
  const stateDir = join(cwd, ".aide", "state");
  if (!existsSync(stateDir)) {
    return { scanned: 0, deleted: 0 };
  }

  const now = Date.now();
  const maxAge = 24 * 60 * 60 * 1000;
  let scanned = 0;
  let deleted = 0;

  try {
    const files = readdirSync(stateDir);
    for (const file of files) {
      if (file.endsWith("-state.json") || file === "session.json") {
        scanned++;
        const filePath = join(stateDir, file);
        const stats = statSync(filePath);
        const age = now - stats.mtimeMs;
        if (age > maxAge) {
          try {
            unlinkSync(filePath);
            deleted++;
          } catch {
            // Ignore
          }
        }
      }
    }
  } catch {
    // Ignore
  }

  return { scanned, deleted };
}

/**
 * Reset HUD state file for clean session start
 */
export function resetHudState(cwd: string): void {
  const hudPath = join(cwd, ".aide", "state", "hud.txt");
  try {
    if (existsSync(hudPath)) {
      writeFileSync(hudPath, "mode:idle");
    }
  } catch {
    // Non-fatal
  }
}

/**
 * Run `aide session init` — single binary invocation that:
 * 1. Deletes global state keys
 * 2. Cleans up stale agent state
 * 3. Returns memories, decisions, recent sessions
 */
export function runSessionInit(
  binary: string,
  cwd: string,
  projectName: string,
  sessionLimit: number,
  config?: AideConfig,
): MemoryInjection {
  const result: MemoryInjection = {
    static: { global: [], project: [], decisions: [] },
    dynamic: { sessions: [] },
  };

  if (process.env.AIDE_MEMORY_INJECT === "0") {
    return result;
  }

  try {
    const args = [
      "session",
      "init",
      `--project=${projectName}`,
      `--session-limit=${sessionLimit}`,
    ];

    // Add --share-import if configured or env var set
    if (
      config?.share?.autoImport ||
      process.env.AIDE_SHARE_AUTO_IMPORT === "1"
    ) {
      args.push("--share-import");
    }

    const output = execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      timeout: 15000,
    }).trim();

    if (!output) return result;

    const data: SessionInitResult = JSON.parse(output);

    if (process.env.AIDE_MEMORY_INJECT === "0") {
      return result;
    }

    result.static.global = data.global_memories.map((m) => m.content);
    result.static.project = data.project_memories.map((m) => m.content);
    result.static.decisions = data.decisions.map(
      (d) =>
        `**${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ""}`,
    );

    for (const sess of data.recent_sessions) {
      const timeAgo = sess.last_at ? formatTimeAgo(sess.last_at) : "";
      const header = `Session ${sess.session_id}${timeAgo ? ` (${timeAgo})` : ""}`;
      const memories = sess.memories
        .map((m) => `- [${m.category}] ${m.content}`)
        .join("\n");
      result.dynamic.sessions.push(`${header}:\n${memories}`);
    }
  } catch {
    // Best effort
  }

  return result;
}

/**
 * Format a timestamp as relative time
 */
export function formatTimeAgo(isoTimestamp: string): string {
  try {
    const dt = new Date(isoTimestamp);
    const now = new Date();
    const seconds = (now.getTime() - dt.getTime()) / 1000;
    const minutes = seconds / 60;
    const hours = seconds / 3600;
    const days = seconds / 86400;

    if (minutes < 60) return `${Math.floor(minutes)}m ago`;
    if (hours < 24) return `${Math.floor(hours)}h ago`;
    if (days < 7) return `${Math.floor(days)}d ago`;

    const month = dt.toLocaleString("en", { month: "short" });
    return `${dt.getDate()} ${month}`;
  } catch {
    return "";
  }
}

/**
 * Build welcome context with proper memory injection
 */
export function buildWelcomeContext(
  state: SessionState,
  memories: MemoryInjection,
  notices: StartupNotices = {},
): string {
  const lines = ["<aide-context>", ""];

  if (notices.error) {
    lines.push("## Setup Issue");
    lines.push("");
    lines.push(notices.error);
    lines.push("");
  }

  if (notices.warning) {
    lines.push("## Update Available");
    lines.push("");
    lines.push(notices.warning);
    lines.push("");
  }

  if (notices.info && notices.info.length > 0) {
    lines.push("## Startup");
    lines.push("");
    for (const info of notices.info) {
      lines.push(`- ${info}`);
    }
    lines.push("");
  }

  lines.push("## Session");
  lines.push("");
  lines.push(`ID: ${state.sessionId.slice(0, 8)}`);
  lines.push(`Project: ${getProjectName(state.cwd)}`);
  lines.push("");

  lines.push("## Binary Path");
  lines.push("");
  lines.push(
    "The aide CLI is at `.aide/bin/aide`. When running aide commands in Bash:",
  );
  lines.push("- Use full path: `./.aide/bin/aide <command>`");
  lines.push(
    '- Or add to PATH: `export PATH="$PWD/.aide/bin:$PATH" && aide <command>`',
  );
  lines.push("");

  if (memories.static.global.length > 0) {
    lines.push("## Preferences (Global)");
    lines.push("");
    lines.push("User preferences that apply across all projects:");
    lines.push("");
    for (const mem of memories.static.global) {
      lines.push(`- ${mem}`);
    }
    lines.push("");
  }

  if (memories.static.project.length > 0) {
    lines.push("## Project Context");
    lines.push("");
    lines.push("Memories specific to this project:");
    lines.push("");
    for (const mem of memories.static.project) {
      lines.push(`- ${mem}`);
    }
    lines.push("");
  }

  if (memories.static.decisions.length > 0) {
    lines.push("## Project Decisions");
    lines.push("");
    lines.push("Architectural decisions for this project. Follow these:");
    lines.push("");
    for (const decision of memories.static.decisions) {
      lines.push(`- ${decision}`);
    }
    lines.push("");
  }

  if (memories.dynamic.sessions.length > 0) {
    lines.push("## Recent Sessions");
    lines.push("");
    for (const session of memories.dynamic.sessions) {
      const sessionLines = session.split("\n");
      lines.push(`### ${sessionLines[0]}`);
      if (sessionLines.length > 1) {
        lines.push("");
        lines.push(...sessionLines.slice(1));
      }
      lines.push("");
    }
  }

  lines.push("## Available Modes");
  lines.push("");
  lines.push("- **autopilot**: Full autonomous execution");
  lines.push("- **eco**: Token-efficient mode");
  lines.push("- **ralph**: Persistence until verified complete");
  lines.push("- **swarm**: Parallel agents with shared memory");
  lines.push("- **plan**: Planning interview workflow");
  lines.push("");
  lines.push("</aide-context>");

  return lines.join("\n");
}
