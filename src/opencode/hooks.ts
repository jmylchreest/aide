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

import { execFileSync } from "child_process";
import { join } from "path";
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
import { checkWriteGuard } from "../core/write-guard.js";
import {
  checkComments,
  getCheckableFilePath,
  getContentToCheck,
} from "../core/comment-checker.js";
import { getState, setState } from "../core/aide-client.js";
import { saveStateSnapshot } from "../core/pre-compact-logic.js";
import { cleanupSession } from "../core/cleanup.js";
import {
  buildSessionSummaryFromState,
  getSessionCommits,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import {
  storePartialMemory,
  gatherPartials,
  buildSummaryFromPartials,
  cleanupPartials,
} from "../core/partial-memory.js";
import type { MemoryInjection, SessionState } from "../core/types.js";
import type {
  Hooks,
  OpenCodeClient,
  OpenCodeConfig,
  OpenCodeEvent,
  OpenCodeSession,
  OpenCodePart,
} from "./types.js";
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
  /** Root of the aide plugin package (for finding bundled skills) */
  pluginRoot: string | null;
  sessionState: SessionState | null;
  memories: MemoryInjection | null;
  welcomeContext: string | null;
  /** Track which sessions we've initialized, to avoid double-init */
  initializedSessions: Set<string>;
  /** Track seen message parts for dedup in skill matching */
  processedMessageParts: Set<string>;
  /** Matched skills pending injection via system transform */
  pendingSkillsContext: string | null;
  /** Last user prompt text, used for skill matching in system transform */
  lastUserPrompt: string | null;
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
  pluginRoot?: string,
): Promise<Hooks> {
  const state: AideState = {
    initialized: false,
    binary: null,
    cwd,
    worktree,
    pluginRoot: pluginRoot || null,
    sessionState: null,
    memories: null,
    welcomeContext: null,
    initializedSessions: new Set(),
    processedMessageParts: new Set(),
    pendingSkillsContext: null,
    lastUserPrompt: null,
    sessionInfoMap: new Map(),
    client,
  };

  // Run one-time initialization (directories, binary, config)
  initializeAide(state);

  return {
    event: createEventHandler(state),
    config: createConfigHandler(state),
    "command.execute.before": createCommandHandler(state),
    "tool.execute.before": createToolBeforeHandler(state),
    "tool.execute.after": createToolAfterHandler(state),
    "experimental.session.compacting": createCompactionHandler(state),
    "experimental.chat.system.transform": createSystemTransformHandler(state),
    "permission.ask": createPermissionHandler(state),
    "shell.env": createShellEnvHandler(state),
  };
}

// =============================================================================
// Config handler (register aide commands as OpenCode slash commands)
// =============================================================================

function createConfigHandler(
  state: AideState,
): (input: OpenCodeConfig) => Promise<void> {
  return async (input) => {
    try {
      // Discover all skills and register them as OpenCode commands
      const skills = discoverSkills(state.cwd, state.pluginRoot ?? undefined);

      if (!input.command) {
        input.command = {};
      }

      for (const skill of skills) {
        const commandName = `aide:${skill.name}`;
        // Only register if not already defined (user config takes priority)
        if (!input.command[commandName]) {
          input.command[commandName] = {
            template: `Activate the aide "${skill.name}" skill. {{arguments}}`,
            description: skill.description || `aide ${skill.name} skill`,
          };
        }
      }

      debug(
        SOURCE,
        `Registered ${skills.length} aide commands via config hook`,
      );
    } catch (err) {
      debug(SOURCE, `Config hook failed (non-fatal): ${err}`);
    }
  };
}

// =============================================================================
// Command handler (intercept aide slash command execution)
// =============================================================================

function createCommandHandler(state: AideState): (
  input: { command: string; sessionID: string; arguments: string },
  output: {
    parts: Array<{ type: string; text: string; [key: string]: unknown }>;
  },
) => Promise<void> {
  return async (input, output) => {
    // Only handle aide: prefixed commands
    if (!input.command.startsWith("aide:")) return;

    const skillName = input.command.slice("aide:".length);
    const args = input.arguments || "";

    try {
      const skills = discoverSkills(state.cwd, state.pluginRoot ?? undefined);
      const skill = skills.find((s) => s.name === skillName);

      if (skill) {
        // Format the skill content for injection
        const context = formatSkillsContext([skill]);

        // Store for system transform injection
        state.pendingSkillsContext = context;

        // Also store the arguments as the user prompt for the transform
        if (args) {
          state.lastUserPrompt = args;
        }

        debug(SOURCE, `Command handler activated skill: ${skillName}`);

        await state.client.app.log({
          body: {
            service: "aide",
            level: "info",
            message: `Activated skill: ${skillName}${args ? ` with args: ${args.slice(0, 50)}` : ""}`,
          },
        });
      } else {
        debug(SOURCE, `Command handler: unknown skill "${skillName}"`);
      }
    } catch (err) {
      debug(SOURCE, `Command handler failed (non-fatal): ${err}`);
    }
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

/**
 * Extract session ID from an event. OpenCode uses different shapes:
 * - session.created/deleted/updated: { properties: { info: { id: string } } }
 * - session.idle/compacted:          { properties: { sessionID: string } }
 * - message.part.updated:            { properties: { part: { sessionID: string } } }
 */
function extractSessionId(event: OpenCodeEvent): string {
  // session.created / session.deleted / session.updated — nested under info
  const info = event.properties.info as OpenCodeSession | undefined;
  if (info?.id) return info.id;

  // session.idle, session.compacted, tool hooks — direct sessionID
  const sessionID = event.properties.sessionID as string | undefined;
  if (sessionID) return sessionID;

  // message.part.updated — sessionID on the part object
  const part = event.properties.part as OpenCodePart | undefined;
  if (part && "sessionID" in part && part.sessionID) return part.sessionID;

  return "unknown";
}

async function handleSessionCreated(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  const sessionId = extractSessionId(event);

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
  const sessionId = extractSessionId(event);

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
  // Uses partials if available for a richer summary.
  if (state.binary) {
    const partials = gatherPartials(state.binary, state.cwd, sessionId);
    let summary: string | null = null;

    if (partials.length > 0) {
      const commits = getSessionCommits(state.cwd);
      summary = buildSummaryFromPartials(partials, commits, []);
      debug(SOURCE, `Built summary from ${partials.length} partials`);
    }

    // Fall back to state-only summary if no partials
    if (!summary) {
      summary = buildSessionSummaryFromState(state.cwd);
    }

    if (summary) {
      storeSessionSummary(state.binary, state.cwd, sessionId, summary);
      // Clean up partials now that the final summary is stored
      const cleaned = cleanupPartials(state.binary, state.cwd, sessionId);
      if (cleaned > 0) {
        debug(SOURCE, `Cleaned up ${cleaned} partials after final summary`);
      }
    } else if (partials.length > 0) {
      // Clean up partials even if no summary was generated
      cleanupPartials(state.binary, state.cwd, sessionId);
    }
  }
}

async function handleSessionDeleted(
  state: AideState,
  event: OpenCodeEvent,
): Promise<void> {
  const sessionId = extractSessionId(event);

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
  // Skill injection: only process user text parts
  const part = event.properties.part as OpenCodePart | undefined;
  if (!part) return;

  // Only match skills for text parts (user messages)
  if (part.type !== "text") return;
  const textContent = (part as { text?: string }).text;
  if (!textContent) return;

  // Dedup: don't re-process the same part
  const partId = part.id || "";
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

  const prompt = textContent;

  // Store latest user prompt so system transform can match skills even if
  // this event fires after the transform (defensive against ordering).
  state.lastUserPrompt = prompt;

  const skills = discoverSkills(state.cwd, state.pluginRoot ?? undefined);
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
    // Write guard: block Write tool on existing files
    try {
      const guardResult = checkWriteGuard(
        input.tool,
        _output.args || {},
        state.cwd,
      );
      if (!guardResult.allowed) {
        debug(SOURCE, `Write guard blocked: ${guardResult.message}`);
        throw new Error(guardResult.message);
      }
    } catch (err) {
      // Re-throw write guard errors (they're intentional blocks)
      if (
        err instanceof Error &&
        err.message?.includes("already exists. Use the Edit tool")
      ) {
        throw err;
      }
      debug(SOURCE, `Write guard check failed (non-fatal): ${err}`);
    }

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

    // Write a partial memory for significant tool uses
    try {
      const toolArgs = (_output.metadata?.args || {}) as Record<
        string,
        unknown
      >;
      storePartialMemory(state.binary, state.cwd, {
        toolName: input.tool,
        sessionId: input.sessionID,
        filePath: toolArgs.file_path as string | undefined,
        command: toolArgs.command as string | undefined,
        description: toolArgs.description as string | undefined,
      });
    } catch (err) {
      debug(SOURCE, `Partial memory write failed (non-fatal): ${err}`);
    }

    // Comment checker: detect excessive comments in Write/Edit output
    try {
      const toolArgs = (_output.metadata?.args || {}) as Record<
        string,
        unknown
      >;
      const filePath = getCheckableFilePath(input.tool, toolArgs);
      if (filePath) {
        const contentResult = getContentToCheck(input.tool, toolArgs);
        if (contentResult) {
          const [content, isNewContent] = contentResult;
          const result = checkComments(filePath, content, isNewContent);
          if (result.hasExcessiveComments) {
            debug(
              SOURCE,
              `Comment checker: ${result.suspiciousCount} suspicious comments in ${filePath}`,
            );
            _output.output += "\n\n" + result.warning;
          }
        }
      }
    } catch (err) {
      debug(SOURCE, `Comment checker failed (non-fatal): ${err}`);
    }
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
      // Uses partials (if available) for a richer summary, falling back to git-only.
      try {
        const partials = gatherPartials(
          state.binary,
          state.cwd,
          input.sessionID,
        );
        let summary: string | null = null;

        if (partials.length > 0) {
          const commits = getSessionCommits(state.cwd);
          summary = buildSummaryFromPartials(partials, commits, []);
          debug(
            SOURCE,
            `Built pre-compact summary from ${partials.length} partials`,
          );
        }

        // Fall back to state-only summary if no partials
        if (!summary) {
          summary = buildSessionSummaryFromState(state.cwd);
        }

        if (summary) {
          // Tag as partial so the session-end summary supersedes it
          const dbPath = join(state.cwd, ".aide", "memory", "store.db");
          const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
          const tags = `partial,session-summary,session:${input.sessionID.slice(0, 8)}`;
          execFileSync(
            state.binary,
            ["memory", "add", "--category=session", `--tags=${tags}`, summary],
            { env, stdio: "pipe", timeout: 5000 },
          );
          debug(
            SOURCE,
            `Saved pre-compaction partial session summary for ${input.sessionID.slice(0, 8)}`,
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

    // Inject matched skills. If message.part.updated already matched skills,
    // use the pre-computed context. Otherwise, match inline from the last user
    // prompt as a fallback (guards against event ordering issues).
    if (state.pendingSkillsContext) {
      output.system.push(state.pendingSkillsContext);
      state.pendingSkillsContext = null;
    } else if (state.lastUserPrompt) {
      try {
        const skills = discoverSkills(state.cwd, state.pluginRoot ?? undefined);
        const matched = matchSkills(state.lastUserPrompt, skills, 3);
        if (matched.length > 0) {
          output.system.push(formatSkillsContext(matched));
          debug(
            SOURCE,
            `System transform fallback matched ${matched.length} skills`,
          );
        }
      } catch (err) {
        debug(SOURCE, `Fallback skill matching failed (non-critical): ${err}`);
      }
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
