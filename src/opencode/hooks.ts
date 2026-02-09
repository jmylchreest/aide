/**
 * OpenCode hook implementations.
 *
 * Maps OpenCode lifecycle events to aide core functions.
 * Each hook is a thin adapter that translates OpenCode's event model
 * to aide's platform-agnostic core logic.
 *
 * Hook mapping:
 *   OpenCode event             → aide behavior
 *   ─────────────────────────  ────────────────────
 *   event(session.created)     → init dirs, binary, inject memories
 *   event(session.idle)        → persistence check, session summary
 *   event(session.deleted)     → final cleanup
 *   event(message.part.updated)→ skill matching (user messages)
 *   tool.execute.before        → tool tracking + tool enforcement
 *   tool.execute.after         → tool stats update
 *   permission.ask             → agent role-based tool access control
 *   shell.env                  → inject AIDE_* env vars into shell commands
 *   experimental.session.compacting → state snapshot, context inject
 *   experimental.chat.system.transform → memory/skill/session context injection
 *
 * Platform limitations (OpenCode does not support these events):
 *   SubagentStart/SubagentStop → not available; OpenCode has no subagent lifecycle events
 *                                (sessions are tracked as pseudo-agents via session.created)
 *   Stop (blocking)            → session.idle re-prompts via session.prompt() for persistence
 */

import { findAideBinary } from "../core/aide-client.js";
import {
  ensureDirectories,
  getProjectName,
  loadConfig,
  initializeSession,
  cleanupStaleStateFiles,
  resetHudState,
  runSessionInit,
  buildWelcomeContext,
} from "../core/session-init.js";
import { syncMcpServers } from "../core/mcp-sync.js";
import {
  discoverSkills,
  matchSkills,
  formatSkillsContext,
} from "../core/skill-matcher.js";
import { trackToolUse, updateToolStats } from "../core/tool-tracking.js";
import { evaluateToolUse, isToolDenied } from "../core/tool-enforcement.js";
import { checkPersistence, getActiveMode } from "../core/persistence-logic.js";
import { getState, setState } from "../core/aide-client.js";
import { saveStateSnapshot } from "../core/pre-compact-logic.js";
import { cleanupSession } from "../core/cleanup.js";
import {
  buildSessionSummaryFromState,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import type { MemoryInjection, SessionState } from "../core/types.js";
import type { Hooks, OpenCodeClient, OpenCodeEvent } from "./types.js";
import { debug } from "../lib/logger.js";

const SOURCE = "opencode-hooks";

/** Per-session metadata for agent-like tracking */
interface SessionInfo {
  sessionId: string;
  createdAt: string;
  /** Agent role/name if assigned (e.g., via swarm orchestration) */
  agentName?: string;
  /** Worktree path if this session is assigned to one */
  worktreePath?: string;
  /** Worktree branch if applicable */
  worktreeBranch?: string;
}

interface AideState {
  initialized: boolean;
  binary: string | null;
  cwd: string;
  worktree: string;
  sessionState: SessionState | null;
  memories: MemoryInjection | null;
  welcomeContext: string | null;
  /** Track which sessions we've initialized, to avoid double-init */
  initializedSessions: Set<string>;
  /** Track seen message parts for dedup in skill matching */
  processedMessageParts: Set<string>;
  /** Matched skills pending injection via system transform */
  pendingSkillsContext: string | null;
  /** Per-session metadata for agent-like tracking */
  sessionInfoMap: Map<string, SessionInfo>;
  client: OpenCodeClient;
}

/**
 * Create all OpenCode hooks wired to aide core logic.
 */
export async function createHooks(
  cwd: string,
  worktree: string,
  client: OpenCodeClient,
): Promise<Hooks> {
  const state: AideState = {
    initialized: false,
    binary: null,
    cwd,
    worktree,
    sessionState: null,
    memories: null,
    welcomeContext: null,
    initializedSessions: new Set(),
    processedMessageParts: new Set(),
    pendingSkillsContext: null,
    sessionInfoMap: new Map(),
    client,
  };

  // Run one-time initialization (directories, binary, config)
  initializeAide(state);

  return {
    event: createEventHandler(state),
    "tool.execute.before": createToolBeforeHandler(state),
    "tool.execute.after": createToolAfterHandler(state),
    "experimental.session.compacting": createCompactionHandler(state),
    "experimental.chat.system.transform": createSystemTransformHandler(state),
    "permission.ask": createPermissionHandler(state),
    "shell.env": createShellEnvHandler(state),
  };
}

// =============================================================================
// Initialization
// =============================================================================

function initializeAide(state: AideState): void {
  try {
    ensureDirectories(state.cwd);

    // Sync MCP server configs across assistants (FS only, fast)
    try {
      syncMcpServers("opencode", state.cwd);
    } catch (err) {
      debug(SOURCE, `MCP sync failed (non-fatal): ${err}`);
    }

    loadConfig(state.cwd);
    cleanupStaleStateFiles(state.cwd);
    resetHudState(state.cwd);

    state.binary = findAideBinary({ cwd: state.cwd });

    if (state.binary) {
      const projectName = getProjectName(state.cwd);
      state.memories = runSessionInit(state.binary, state.cwd, projectName, 3);
    }

    state.initialized = true;
  } catch (err) {
    debug(SOURCE, `Initialization failed (best-effort): ${err}`);
    // Mark initialized to prevent retry loops, but note that state may be partial.
    // Downstream hooks check state.binary/state.memories individually.
    state.initialized = true;
  }
}

// =============================================================================
// Event handler (session lifecycle + message events)
// =============================================================================

function createEventHandler(
  state: AideState,
): (input: { event: OpenCodeEvent }) => Promise<void> {
  return async ({ event }) => {
    switch (event.type) {
      case "session.created":
        await handleSessionCreated(state, event);
        break;
      case "session.idle":
        await handleSessionIdle(state, event);
        break;
      case "session.deleted":
        await handleSessionDeleted(state, event);
        break;
      case "message.part.updated":
        await handleMessagePartUpdated(state, event);
        break;
      default:
        // Ignore other events
        break;
    }
  };
}

async function handleSessionCreated(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  if (state.initializedSessions.has(sessionId)) return;
  state.initializedSessions.add(sessionId);

  state.sessionState = initializeSession(sessionId, state.cwd);

  // Track this session for per-session context injection
  state.sessionInfoMap.set(sessionId, {
    sessionId,
    createdAt: new Date().toISOString(),
  });

  // Register session as an "agent" in aide state for visibility
  if (state.binary) {
    try {
      setState(state.binary, state.cwd, "status", "running", sessionId);
      setState(state.binary, state.cwd, "type", "opencode-session", sessionId);
      setState(
        state.binary,
        state.cwd,
        "startedAt",
        new Date().toISOString(),
        sessionId,
      );
    } catch (err) {
      debug(SOURCE, `Failed to register session state (non-fatal): ${err}`);
    }
  }

  if (state.memories) {
    state.welcomeContext = buildWelcomeContext(
      state.sessionState,
      state.memories,
    );
  }

  // Log startup info
  try {
    await state.client.app.log({
      body: {
        service: "aide",
        level: "info",
        message: `aide initialized for session ${sessionId.slice(0, 8)}`,
      },
    });
  } catch (err) {
    debug(SOURCE, `Failed to log session creation (non-critical): ${err}`);
  }
}

async function handleSessionIdle(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  // Check persistence: if ralph/autopilot mode is active, re-prompt the session
  if (state.binary) {
    try {
      const persistResult = checkPersistence(state.binary, state.cwd);
      if (persistResult) {
        const activeMode = getActiveMode(state.binary, state.cwd);
        debug(
          SOURCE,
          `Persistence active on idle (${activeMode}), re-prompting session ${sessionId.slice(0, 8)}`,
        );

        // Re-prompt the session to continue working
        try {
          await state.client.session.prompt({
            path: { id: sessionId },
            body: {
              parts: [
                {
                  type: "text",
                  text: persistResult.reason,
                },
              ],
            },
          });
          debug(
            SOURCE,
            `Re-prompted session ${sessionId.slice(0, 8)} for persistence`,
          );
        } catch (err) {
          debug(SOURCE, `Failed to re-prompt session for persistence: ${err}`);
          // Fall back to logging
          try {
            await state.client.app.log({
              body: {
                service: "aide",
                level: "warn",
                message: `[aide:${activeMode || "persistence"}] Session went idle but persistence mode is active. Work may be incomplete.`,
              },
            });
          } catch (logErr) {
            debug(SOURCE, `Failed to log persistence warning: ${logErr}`);
          }
        }
      }
    } catch (err) {
      debug(SOURCE, `Persistence check failed (non-fatal): ${err}`);
    }
  }

  // Capture session summary (best effort, no transcript available)
  if (state.binary) {
    const summary = buildSessionSummaryFromState(state.cwd);
    if (summary) {
      storeSessionSummary(state.binary, state.cwd, sessionId, summary);
    }
  }
}

async function handleSessionDeleted(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  if (state.binary) {
    cleanupSession(state.binary, state.cwd, sessionId);
  }

  state.initializedSessions.delete(sessionId);
  state.sessionInfoMap.delete(sessionId);
  state.processedMessageParts.clear();
}

async function handleMessagePartUpdated(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  // Skill injection: only process user messages
  const part = event.properties.part as
    | { type?: string; text?: string; role?: string }
    | undefined;
  if (!part) return;

  // Only match skills for user text parts
  const role = (event.properties.role as string) || part.role;
  if (role !== "user") return;
  if (part.type !== "text" || !part.text) return;

  // Dedup: don't re-process the same part
  const partId = (event.properties.partID as string) || "";
  if (partId && state.processedMessageParts.has(partId)) return;
  if (partId) state.processedMessageParts.add(partId);

  // Cap dedup set size — evict oldest half to avoid cliff-edge re-processing
  if (state.processedMessageParts.size > 1000) {
    const entries = Array.from(state.processedMessageParts);
    const keepFrom = Math.floor(entries.length / 2);
    state.processedMessageParts.clear();
    for (let i = keepFrom; i < entries.length; i++) {
      state.processedMessageParts.add(entries[i]);
    }
    debug(
      SOURCE,
      `Evicted ${keepFrom} entries from processedMessageParts dedup set`,
    );
  }

  const prompt = part.text;
  const skills = discoverSkills(state.cwd);
  const matched = matchSkills(prompt, skills, 3);

  if (matched.length > 0) {
    try {
      const context = formatSkillsContext(matched);
      await state.client.app.log({
        body: {
          service: "aide",
          level: "info",
          message: `Matched ${matched.length} skills: ${matched.map((s) => s.name).join(", ")}`,
        },
      });
      // Note: OpenCode doesn't have a direct skill injection hook like Claude Code's
      // UserPromptSubmit. Skills are logged here; injection happens via system transform.
      // Store matched skills for injection in system transform
      state.pendingSkillsContext = context;
    } catch (err) {
      debug(
        SOURCE,
        `Failed to log/inject matched skills (non-critical): ${err}`,
      );
    }
  }
}

// =============================================================================
// Tool hooks
// =============================================================================

function createToolBeforeHandler(
  state: AideState,
): (
  input: { tool: string; sessionID: string; callID: string },
  output: { args: Record<string, unknown> },
) => Promise<void> {
  return async (input, _output) => {
    // Tool enforcement: check agent restrictions
    // OpenCode doesn't have named agent types like Claude Code, but
    // we still evaluate in case agent_name is passed via tool args or state
    try {
      const agentName = _output.args?.agent_name as string | undefined;
      if (agentName) {
        const toolMode = state.binary
          ? getState(state.binary, state.cwd, "mode")
          : null;
        const enforcement = evaluateToolUse(input.tool, agentName, toolMode);
        if (!enforcement.allowed) {
          debug(
            SOURCE,
            `Tool ${input.tool} denied for agent ${agentName}: ${enforcement.denyMessage}`,
          );
          // OpenCode doesn't support blocking tool execution from hooks yet,
          // but we log the violation for visibility
        }
      }
    } catch (err) {
      debug(SOURCE, `Tool enforcement check failed (non-fatal): ${err}`);
    }

    // Track tool use
    if (!state.binary) return;

    trackToolUse(state.binary, state.cwd, {
      toolName: input.tool,
      agentId: input.sessionID,
      toolInput: {
        command: _output.args?.command as string | undefined,
        file_path: _output.args?.file_path as string | undefined,
        description: _output.args?.description as string | undefined,
      },
    });
  };
}

function createToolAfterHandler(
  state: AideState,
): (
  input: { tool: string; sessionID: string; callID: string },
  output: { title: string; output: string; metadata: Record<string, unknown> },
) => Promise<void> {
  return async (input, _output) => {
    if (!state.binary) return;

    updateToolStats(state.binary, state.cwd, input.tool, input.sessionID);
  };
}

// =============================================================================
// Compaction hook
// =============================================================================

function createCompactionHandler(
  state: AideState,
): (
  input: { sessionID: string },
  output: { context: string[]; prompt?: string },
) => Promise<void> {
  return async (input, output) => {
    // Save state snapshot before compaction
    if (state.binary) {
      saveStateSnapshot(state.binary, state.cwd, input.sessionID);

      // Persist a session summary as a memory before context is compacted.
      // This ensures the work-so-far is recoverable after compaction.
      try {
        const summary = buildSessionSummaryFromState(state.cwd);
        if (summary) {
          storeSessionSummary(
            state.binary,
            state.cwd,
            input.sessionID,
            summary,
          );
          debug(
            SOURCE,
            `Saved pre-compaction session summary for ${input.sessionID.slice(0, 8)}`,
          );
        }
      } catch (err) {
        debug(
          SOURCE,
          `Failed to save pre-compaction summary (non-fatal): ${err}`,
        );
      }
    }

    // Refresh welcomeContext so the just-saved session summary is included.
    // Without this, post-compaction context would use the stale welcome built
    // at session start — missing the current session's work-so-far summary.
    if (state.binary && state.sessionState) {
      try {
        const projectName = getProjectName(state.cwd);
        const freshMemories = runSessionInit(
          state.binary,
          state.cwd,
          projectName,
          3,
        );
        state.memories = freshMemories;
        state.welcomeContext = buildWelcomeContext(
          state.sessionState,
          freshMemories,
        );
        debug(
          SOURCE,
          "Refreshed welcomeContext after pre-compaction summary save",
        );
      } catch (err) {
        debug(
          SOURCE,
          `Failed to refresh welcomeContext (non-fatal, using stale): ${err}`,
        );
      }
    }

    // Inject preserved context into compaction
    if (state.welcomeContext) {
      output.context.push(state.welcomeContext);
    }
  };
}

// =============================================================================
// System prompt transform (memory + skill injection)
// =============================================================================

function createSystemTransformHandler(
  state: AideState,
): (
  input: { sessionID?: string; model: { providerID: string; modelID: string } },
  output: { system: string[] },
) => Promise<void> {
  return async (_input, output) => {
    // Inject welcome context (memories, decisions, etc.) into system prompt
    if (state.welcomeContext) {
      output.system.push(state.welcomeContext);
    }

    // Inject per-session context only when swarm mode is active and session
    // has been assigned a worktree/role (e.g., by swarm orchestration).
    // Without this guard, every normal session would incorrectly be told
    // it's in a worktree.
    // Use raw mode (not getActiveMode which filters to persistence modes only)
    const rawMode = state.binary
      ? getState(state.binary, state.cwd, "mode")
      : null;
    if (rawMode === "swarm") {
      const sessionId = _input.sessionID;
      if (sessionId) {
        const sessionInfo = state.sessionInfoMap.get(sessionId);
        if (
          sessionInfo &&
          (sessionInfo.worktreePath || sessionInfo.agentName)
        ) {
          const lines: string[] = ["<aide-session-context>"];
          lines.push("");
          lines.push("## Swarm Session Context");
          lines.push("");
          lines.push(`- **Session ID**: ${sessionId.slice(0, 8)}`);
          if (sessionInfo.agentName) {
            lines.push(`- **Role**: ${sessionInfo.agentName}`);
          }
          if (sessionInfo.worktreePath) {
            lines.push(`- **Worktree**: ${sessionInfo.worktreePath}`);
            lines.push(
              `- **Branch**: ${sessionInfo.worktreeBranch || "unknown"}`,
            );
            lines.push("");
            lines.push(
              `**IMPORTANT**: All file operations should be in: ${sessionInfo.worktreePath}`,
            );
          }
          lines.push("");
          lines.push("</aide-session-context>");
          output.system.push(lines.join("\n"));
        }
      }
    }

    // Inject any pending matched skills
    if (state.pendingSkillsContext) {
      output.system.push(state.pendingSkillsContext);
      state.pendingSkillsContext = null;
    }

    // Inject messaging protocol for multi-instance coordination
    output.system.push(`<aide-messaging>

## Agent Messaging

Use aide MCP tools to coordinate with other agents or sessions:

**Send:** \`message_send\` — from (your ID), to (recipient or omit for broadcast), content, type
**Check:** \`message_list\` — agent_id (your ID)
**Ack:** \`message_ack\` — message_id, agent_id

**Message types:** status, request, response, blocker, completion, handoff

**Protocol:**
- Send \`status\` at each stage transition
- Send \`blocker\` when stuck
- Send \`completion\` when done
- Check messages periodically for requests from other agents

</aide-messaging>`);
  };
}

// =============================================================================
// Permission handler (tool access control)
// =============================================================================

function createPermissionHandler(
  state: AideState,
): (
  input: { tool: string; permission: string; patterns: string[] },
  output: { status: "ask" | "deny" | "allow" },
) => Promise<void> {
  return async (input, output) => {
    try {
      const activeMode = state.binary
        ? getState(state.binary, state.cwd, "mode")
        : null;

      // Enforce agent role restrictions in swarm mode where sessions
      // are assigned specific roles (architect, reviewer, etc.)
      if (activeMode === "swarm") {
        for (const [_sessionId, info] of state.sessionInfoMap) {
          if (info.agentName && isToolDenied(info.agentName, input.tool)) {
            debug(
              SOURCE,
              `permission.ask: denying ${input.tool} for agent role ${info.agentName}`,
            );
            output.status = "deny";
            return;
          }
        }
      }

      // For write tools in eco mode, add advisory logging
      if (
        activeMode === "eco" &&
        ["Edit", "Write", "Bash"].includes(input.tool)
      ) {
        debug(
          SOURCE,
          `permission.ask: eco mode active, allowing ${input.tool} (advisory only)`,
        );
      }
    } catch (err) {
      debug(SOURCE, `permission.ask handler failed (non-fatal): ${err}`);
    }
    // Default: don't change output.status (let OpenCode handle normally)
  };
}

// =============================================================================
// Shell environment injection
// =============================================================================

function createShellEnvHandler(
  state: AideState,
): (
  input: { cwd: string },
  output: { env: Record<string, string> },
) => Promise<void> {
  return async (_input, output) => {
    try {
      // Inject aide-specific environment variables into shell commands
      // This allows scripts and tools to detect the aide environment

      // Session ID (most recent)
      if (state.sessionState?.sessionId) {
        output.env.AIDE_SESSION_ID = state.sessionState.sessionId;
      }

      // Active mode
      const activeMode = state.binary
        ? getState(state.binary, state.cwd, "mode")
        : null;
      if (activeMode) {
        output.env.AIDE_MODE = activeMode;
      }

      // Per-session worktree and agent info (only in swarm mode)
      if (activeMode === "swarm") {
        for (const [_sessionId, info] of state.sessionInfoMap) {
          if (info.worktreePath) {
            output.env.AIDE_WORKTREE = info.worktreePath;
            if (info.worktreeBranch) {
              output.env.AIDE_WORKTREE_BRANCH = info.worktreeBranch;
            }
            break; // Use the first session with a worktree
          }
          if (info.agentName) {
            output.env.AIDE_AGENT_NAME = info.agentName;
          }
        }
      }

      // Always set platform identifier
      output.env.AIDE_PLATFORM = "opencode";
    } catch (err) {
      debug(SOURCE, `shell.env handler failed (non-fatal): ${err}`);
    }
  };
}
