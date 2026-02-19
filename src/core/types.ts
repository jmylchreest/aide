/**
 * Shared types used by both Claude Code hooks and OpenCode plugin.
 *
 * Platform-agnostic interfaces for aide's core functionality.
 */

// =============================================================================
// Configuration
// =============================================================================

export interface AideConfig {
  share?: {
    /** Auto-import shared data from .aide/shared/ on session start (default: false) */
    autoImport?: boolean;
    /** Auto-export on session end (default: false) */
    autoExport?: boolean;
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
  }>;
  project_memories: Array<{
    id: string;
    content: string;
    category: string;
    tags: string[];
  }>;
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

export interface MemoryInjection {
  static: {
    global: string[];
    project: string[];
    decisions: string[];
  };
  dynamic: {
    sessions: string[];
  };
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

export const PERSISTENCE_MODES = ["ralph", "autopilot"] as const;
export type PersistenceMode = (typeof PERSISTENCE_MODES)[number];
export const MAX_PERSISTENCE_ITERATIONS = 20;

// =============================================================================
// Platform Abstraction
// =============================================================================

/**
 * Identifies which host platform aide is running in.
 * Used for platform-specific behavior like binary discovery or context injection.
 */
export type AidePlatform = "claude-code" | "opencode" | "unknown";

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
