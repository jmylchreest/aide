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
import { basename, join } from "path";
import { execFileSync } from "child_process";
import { homedir } from "os";
import type {
  AideConfig,
  SessionState,
  SessionInitResult,
  MemoryInjection,
  InjectedSource,
  StartupNotices,
} from "./types.js";
import { DEFAULT_CONFIG } from "./types.js";
import { isTruthy, isFalsy } from "../lib/hook-utils.js";
import { findProjectRoot } from "../lib/project-root.js";

/**
 * Ensure all .aide directories exist
 */
export function ensureDirectories(cwd: string): {
  created: number;
  existed: number;
} {
  // Resolve to the canonical project root so we don't plant a stray .aide/
  // in a subdirectory the harness happened to launch from. When no marker
  // is found, fall back to cwd (caller's hasMarker gate elsewhere refuses
  // bootstrap unless AIDE_FORCE_INIT is set).
  const { root } = findProjectRoot(cwd);
  const dirs = [
    join(root, ".aide"),
    join(root, ".aide", "skills"),
    join(root, ".aide", "config"),
    join(root, ".aide", "state"),
    join(root, ".aide", "memory"),
    join(root, ".aide", "worktrees"),
    join(root, ".aide", "_logs"),
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
  const gitignorePath = join(root, ".aide", ".gitignore");
  const requiredGitignoreContent = `# AIDE local runtime files - do not commit
# These are machine-specific and/or binary (non-mergeable)
_logs/
state/
bin/
worktrees/
memory/
code/
grammars/
cache/

# Runtime socket - machine-specific
aide.sock

# MCP sync state - machine-specific
config/mcp.json
config/mcp-sync.journal.json

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
    // Migrate old gitignore format
    try {
      let existingContent = readFileSync(gitignorePath, "utf-8");
      let updated = false;

      // Ensure shared/ is allowed
      if (!existingContent.includes("!shared/")) {
        existingContent =
          existingContent.trimEnd() +
          `\n
# Shared data IS committed (git-friendly markdown with frontmatter)
!shared/
`;
        updated = true;
      }

      // Ensure grammars are ignored (platform-specific binaries)
      if (!existingContent.includes("grammars/")) {
        existingContent =
          existingContent.trimEnd() +
          `\n
# Tree-sitter grammars - platform-specific binaries
grammars/
`;
        updated = true;
      }

      // Ensure the subscription cache (git clones of peer context repos)
      // is ignored — an embedded checkout must never reach the host repo
      if (!existingContent.includes("cache/")) {
        existingContent =
          existingContent.trimEnd() +
          `\n
# Subscription cache - machine-local peer checkouts (aide sync)
cache/
`;
        updated = true;
      }

      // Ensure runtime socket is ignored
      if (!existingContent.includes("aide.sock")) {
        existingContent =
          existingContent.trimEnd() +
          `\n
# Runtime socket - machine-specific
aide.sock
`;
        updated = true;
      }

      // Ensure MCP sync files are ignored
      if (!existingContent.includes("config/mcp.json")) {
        existingContent =
          existingContent.trimEnd() +
          `\n
# MCP sync state - machine-specific
config/mcp.json
config/mcp-sync.journal.json
`;
        updated = true;
      }

      if (updated) {
        writeFileSync(gitignorePath, existingContent);
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
    const remoteUrl = execFileSync(
      "git",
      ["config", "--get", "remote.origin.url"],
      {
        cwd,
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 2000,
      },
    )
      .toString()
      .trim();

    const match = remoteUrl.match(/[/:]([^/]+?)(?:\.git)?$/);
    if (match) return match[1];
  } catch {
    // Not a git repo or no remote
  }

  return basename(cwd) || "unknown";
}

/**
 * Load config from ~/.aide/config/aide.json (global).
 * Used before a project root has been resolved (e.g. by the SessionStart
 * hook deciding whether to honour `requireGit`).
 */
export function loadGlobalConfig(): AideConfig {
  const configPath = join(homedir(), ".aide", "config", "aide.json");
  if (!existsSync(configPath)) return DEFAULT_CONFIG;
  try {
    return { ...DEFAULT_CONFIG, ...JSON.parse(readFileSync(configPath, "utf-8")) };
  } catch {
    return DEFAULT_CONFIG;
  }
}

/**
 * Load config layered as: defaults → global (~/.aide/config/aide.json) →
 * project (`<cwd>/.aide/config/aide.json`). Project values override global.
 * Does NOT create a default config file — only user-set values are persisted.
 */
export function loadConfig(cwd: string): AideConfig {
  const global = loadGlobalConfig();
  const { root } = findProjectRoot(cwd);
  const projectPath = join(root, ".aide", "config", "aide.json");

  if (existsSync(projectPath)) {
    try {
      const project = JSON.parse(readFileSync(projectPath, "utf-8"));
      return { ...DEFAULT_CONFIG, ...global, ...project };
    } catch {
      return global;
    }
  }

  return global;
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

  // Session state is kept in-memory only (returned to the caller).
  // We no longer write .aide/state/session.json — this eliminates a race
  // condition where concurrent sessions would overwrite each other's file.
  // Callers that need startedAt (e.g. getSessionCommits) receive it as a
  // parameter from the in-memory SessionState or use a fallback default.

  return state;
}

/**
 * Clean up stale state files older than 24 hours
 */
export function cleanupStaleStateFiles(cwd: string): {
  scanned: number;
  deleted: number;
} {
  const { root } = findProjectRoot(cwd);
  const stateDir = join(root, ".aide", "state");
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
  const { root } = findProjectRoot(cwd);
  const hudPath = join(root, ".aide", "state", "hud.txt");
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

  if (isFalsy(process.env.AIDE_MEMORY_INJECT)) {
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
      isTruthy(process.env.AIDE_SHARE_AUTO_IMPORT)
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

    // Surface retention pruning — a silent mass deletion (e.g. first sweep
    // after a default-TTL change) must leave a user-visible trace.
    if (data.retention_pruned) {
      const total = Object.values(data.retention_pruned).reduce(
        (a, b) => a + b,
        0,
      );
      if (total > 0) {
        const detail = Object.entries(data.retention_pruned)
          .map(([k, v]) => `${v} ${k}`)
          .join(", ");
        result.retentionNote = `Retention sweep removed ${total} records past their configured TTL (${detail}). Tune via cleanup.*_max_age.`;
      }
    }

    if (isFalsy(process.env.AIDE_MEMORY_INJECT)) {
      return result;
    }

    result.static.global = data.global_memories.map((m) => m.content);
    result.static.project = data.project_memories.map((m) => m.content);
    result.static.projectOverflow = data.project_memory_overflow ?? false;
    result.static.decisions = data.decisions.map(
      (d) =>
        `**${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ""}${decisionOriginSuffix(d)}`,
    );

    for (const sess of data.recent_sessions) {
      const timeAgo = sess.last_at ? formatTimeAgo(sess.last_at) : "";
      const header = `Session ${sess.session_id}${timeAgo ? ` (${timeAgo})` : ""}`;
      const memories = sess.memories
        .map((m) => `- [${m.category}] ${m.content}`)
        .join("\n");
      result.dynamic.sessions.push(`${header}:\n${memories}`);
    }

    result.estate = data.estate;

    // Codebase Map — survey modules analyzer output, already capped by the
    // Go producer. Separately kill-switchable from memory injection.
    if (
      !isFalsy(process.env.AIDE_SURVEY_INJECT) &&
      data.codebase_map &&
      data.codebase_map.length > 0
    ) {
      result.codebaseMap = data.codebase_map;
      result.codebaseMapNote = data.codebase_map_note;
    }

    const sources: InjectedSource[] = [];
    for (const m of data.global_memories) {
      sources.push({
        kind: "memory",
        scope: "global",
        id: m.id,
        name: m.tags?.[0] ?? m.category ?? "memory",
        content: m.content,
        category: m.category,
        tags: m.tags,
        score: m.score,
      });
    }
    for (const m of data.project_memories) {
      sources.push({
        kind: "memory",
        scope: "project",
        id: m.id,
        name: m.tags?.[0] ?? m.category ?? "memory",
        content: m.content,
        category: m.category,
        tags: m.tags,
        score: m.score,
      });
    }
    for (const d of data.decisions) {
      sources.push({
        kind: "decision",
        scope: "project",
        id: d.topic,
        name: d.topic,
        content: `${d.value}${d.rationale ? ` (${d.rationale})` : ""}`,
      });
    }
    for (const sess of data.recent_sessions) {
      for (const m of sess.memories) {
        sources.push({
          kind: "session_memory",
          scope: "session",
          id: `${sess.session_id}:${m.content.slice(0, 32)}`,
          name: m.category || "session",
          content: m.content,
          category: m.category,
          sessionId: sess.session_id,
        });
      }
    }
    for (const mod of result.codebaseMap ?? []) {
      sources.push({
        kind: "module",
        scope: "project",
        id: `module:${mod.name}`,
        name: mod.name,
        content: `${mod.name} — ${mod.size} files, hub: ${mod.hub}`,
      });
    }
    result.sources = sources;
  } catch {
    // Best effort
  }

  return result;
}

/**
 * Provenance suffix for a non-local decision: parent decisions cascade
 * from the anchor chain (override locally), peer decisions come from a
 * read-only subscription layer (promote with `aide decision adopt`).
 */
function decisionOriginSuffix(
  d: SessionInitResult["decisions"][number],
): string {
  if (!d.origin_name) return "";
  if (d.origin_kind === "peer") {
    return ` — from peer **${d.origin_name}** (read-only; adopt with \`aide decision adopt ${d.topic} --from=${d.origin_name}\`)`;
  }
  return ` — inherited from parent **${d.origin_name}** (override with a local \`decision set ${d.topic}\`)`;
}

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
    if (memories.static.projectOverflow) {
      lines.push("");
      lines.push(
        "_More project memories exist. Use `memory_search` to find specific context._",
      );
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

  if (memories.codebaseMap && memories.codebaseMap.length > 0) {
    const note = memories.codebaseMapNote
      ? ` (${memories.codebaseMapNote})`
      : "";
    lines.push(`## Codebase Map${note}`);
    lines.push("");
    lines.push(
      "Structural modules discovered by clustering the import graph — what belongs together, not just where files live. Query details via the survey MCP tools (survey_list kind=module).",
    );
    lines.push("");
    for (const mod of memories.codebaseMap) {
      lines.push(`- **${mod.name}** — ${mod.size} files, hub: ${mod.hub}`);
    }
    lines.push("");
  }

  const estate = memories.estate;
  if (estate && ((estate.parents?.length ?? 0) > 0 || (estate.subprojects?.length ?? 0) > 0)) {
    lines.push("## Estate");
    lines.push("");
    lines.push(
      "Related project scopes. Each has its OWN aide store: memories and state never cross between them, and writes stay local unless routed with --store. Parent DECISIONS cascade into this context (labeled 'inherited from parent') until overridden locally.",
    );
    lines.push("");
    for (const p of estate.parents ?? []) {
      lines.push(
        `- parent: **${p.name ?? p.path}** at ${p.path} (${p.evidence ?? "parent"}${p.has_store ? ", has own store" : ""})`,
      );
    }
    for (const c of estate.subprojects ?? []) {
      lines.push(
        `- child: **${c.name ?? c.path}** at ./${c.path} (${c.evidence ?? "subproject"}${c.has_store ? ", has own store" : ""}) — its files belong to its own project`,
      );
    }
    lines.push("");
  }

  if (memories.retentionNote) {
    lines.push(`> ${memories.retentionNote}`);
    lines.push("");
  }

  lines.push("## Available Modes");
  lines.push("");
  lines.push("- **autopilot**: Full autonomous execution");
  lines.push("- **eco**: Token-efficient mode");
  lines.push("- **swarm**: Parallel agents with shared memory");
  lines.push("- **plan**: Planning interview workflow");
  lines.push("");
  lines.push("</aide-context>");

  return lines.join("\n");
}
