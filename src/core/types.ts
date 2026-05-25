/**
 * Shared types used by both Claude Code hooks and OpenCode plugin.
 *
 * Platform-agnostic interfaces for aide's core functionality.
 */

// =============================================================================
// Configuration
// =============================================================================

export interface AideConfig {
  /**
   * When true (default), AIDE refuses to bootstrap if no `.git/` or `.aide/`
   * marker is found walking up from the launched cwd. This prevents the hook
   * from planting an orphan `.aide/` folder in an arbitrary subdirectory of a
   * git repo when `claude` is launched there. Set to false in
   * `~/.aide/config/aide.json` to allow init in non-git directories.
   * Only the global-config value is consulted (the project layer is moot
   * because, if a project root was found, the gate has already passed).
   */
  requireGit?: boolean;
  share?: {
    /** Auto-import shared data from .aide/shared/ on session start (default: false) */
    autoImport?: boolean;
    /** Auto-export on session end (default: false) */
    autoExport?: boolean;
  };
  findings?: {
    /** Complexity analyser settings */
    complexity?: {
      /** Cyclomatic complexity threshold (default: 10) */
      threshold?: number;
    };
    /** Import coupling analyser settings */
    coupling?: {
      /** Fan-out threshold — max outgoing imports (default: 15) */
      fanOut?: number;
      /** Fan-in threshold — max incoming imports (default: 20) */
      fanIn?: number;
    };
    /** Code clone detection settings */
    clones?: {
      /** Sliding window size in tokens (default: 50) */
      windowSize?: number;
      /** Minimum clone size in lines (default: 6) */
      minLines?: number;
    };
  };
}

export const DEFAULT_CONFIG: AideConfig = {};

// =============================================================================
// Session
// =============================================================================

export interface SessionState {
  sessionId: string;
  startedAt: string;
  cwd: string;
  activeMode: string | null;
  agentCount: number;
}

export interface SessionInitResult {
  state_keys_deleted: number;
  stale_agents_cleaned: number;
  global_memories: Array<{
    id: string;
    content: string;
    category: string;
    tags: string[];
    score?: number;
  }>;
  project_memories: Array<{
    id: string;
    content: string;
    category: string;
    tags: string[];
    score?: number;
  }>;
  project_memory_overflow?: boolean;
  decisions: Array<{ topic: string; value: string; rationale?: string }>;
  recent_sessions: Array<{
    session_id: string;
    last_at: string;
    memories: Array<{ content: string; category: string }>;
  }>;
}

// =============================================================================
// Memory Injection
// =============================================================================

export interface InjectedSource {
  kind: "memory" | "decision" | "session_memory";
  scope: "global" | "project" | "session";
  id: string;
  name: string;
  content: string;
  category?: string;
  tags?: string[];
  sessionId?: string;
  score?: number;
}

export interface MemoryInjection {
  static: {
    global: string[];
    project: string[];
    projectOverflow?: boolean;
    decisions: string[];
  };
  dynamic: {
    sessions: string[];
  };
  sources?: InjectedSource[];
}

// =============================================================================
// Startup Notices
// =============================================================================

export interface StartupNotices {
  error?: string | null;
  warning?: string | null;
  info?: string[];
}

// =============================================================================
// Skills
// =============================================================================

export interface Skill {
  name: string;
  path: string;
  triggers: string[];
  description?: string;
  /** Optional platform restriction. If set, only matched on listed platforms ("opencode", "claude-code", "codex"). */
  platforms?: string[];
  /** Optional binary requirement. If set, skill is only matched when all listed binaries exist on PATH. */
  requires_binary?: string[];
  content: string;
}

export interface SkillMatchResult {
  skill: Skill;
  score: number;
}

// =============================================================================
// Tool Tracking
// =============================================================================

export interface ToolUseInfo {
  toolName: string;
  agentId?: string;
  toolInput?: {
    command?: string;
    description?: string;
    prompt?: string;
    file_path?: string;
    model?: string;
    subagent_type?: string;
  };
}

// =============================================================================
// Persistence
// =============================================================================

export const PERSISTENCE_MODES = ["autopilot"] as const;
export type PersistenceMode = (typeof PERSISTENCE_MODES)[number];
export const MAX_PERSISTENCE_ITERATIONS = 20;

// =============================================================================
// Platform Abstraction
// =============================================================================

/**
 * Identifies which host platform aide is running in.
 * Used for platform-specific behavior like binary discovery or context injection.
 */
export type AidePlatform = "claude-code" | "opencode" | "codex" | "unknown";

/**
 * Options for finding the aide binary.
 * Platforms provide different hints for where to find the binary.
 */
export interface FindBinaryOptions {
  /** Current working directory */
  cwd?: string;
  /** Plugin root directory (AIDE_PLUGIN_ROOT or CLAUDE_PLUGIN_ROOT) */
  pluginRoot?: string;
  /** Additional paths to search before PATH fallback */
  additionalPaths?: string[];
}
