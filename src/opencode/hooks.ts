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
 *   tool.execute.before        → tool tracking
 *   tool.execute.after         → tool stats update
 *   experimental.session.compacting → state snapshot, context inject
 *   experimental.chat.system.transform → memory injection on session start
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
import { discoverSkills, matchSkills, formatSkillsContext } from "../core/skill-matcher.js";
import { trackToolUse, updateToolStats } from "../core/tool-tracking.js";
import { saveStateSnapshot } from "../core/pre-compact-logic.js";
import { cleanupSession } from "../core/cleanup.js";
import {
  buildSessionSummaryFromState,
  storeSessionSummary,
} from "../core/session-summary-logic.js";
import type { MemoryInjection, SessionState } from "../core/types.js";
import type { Hooks, OpenCodeClient, OpenCodeEvent } from "./types.js";

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
  };
}

// =============================================================================
// Initialization
// =============================================================================

function initializeAide(state: AideState): void {
  try {
    ensureDirectories(state.cwd);
    loadConfig(state.cwd);
    cleanupStaleStateFiles(state.cwd);
    resetHudState(state.cwd);

    state.binary = findAideBinary({ cwd: state.cwd });

    if (state.binary) {
      const projectName = getProjectName(state.cwd);
      state.memories = runSessionInit(state.binary, state.cwd, projectName, 3);
    }

    state.initialized = true;
  } catch {
    // Best-effort initialization
    state.initialized = true;
  }
}

// =============================================================================
// Event handler (session lifecycle + message events)
// =============================================================================

function createEventHandler(state: AideState): (input: { event: OpenCodeEvent }) => Promise<void> {
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

async function handleSessionCreated(state: AideState, event: OpenCodeEvent): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  if (state.initializedSessions.has(sessionId)) return;
  state.initializedSessions.add(sessionId);

  state.sessionState = initializeSession(sessionId, state.cwd);

  if (state.memories) {
    state.welcomeContext = buildWelcomeContext(state.sessionState, state.memories);
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
  } catch {
    // Non-critical
  }
}

async function handleSessionIdle(state: AideState, event: OpenCodeEvent): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  // Capture session summary (best effort, no transcript available)
  if (state.binary) {
    const summary = buildSessionSummaryFromState(state.cwd);
    if (summary) {
      storeSessionSummary(state.binary, state.cwd, sessionId, summary);
    }
  }
}

async function handleSessionDeleted(state: AideState, event: OpenCodeEvent): Promise<void> {
  const sessionId = (event.properties.sessionID as string) || "unknown";

  if (state.binary) {
    cleanupSession(state.binary, state.cwd, sessionId);
  }

  state.initializedSessions.delete(sessionId);
  state.processedMessageParts.clear();
}

async function handleMessagePartUpdated(state: AideState, event: OpenCodeEvent): Promise<void> {
  // Skill injection: only process user messages
  const part = event.properties.part as { type?: string; text?: string; role?: string } | undefined;
  if (!part) return;

  // Only match skills for user text parts
  const role = (event.properties.role as string) || part.role;
  if (role !== "user") return;
  if (part.type !== "text" || !part.text) return;

  // Dedup: don't re-process the same part
  const partId = (event.properties.partID as string) || "";
  if (partId && state.processedMessageParts.has(partId)) return;
  if (partId) state.processedMessageParts.add(partId);

  // Cap dedup set size
  if (state.processedMessageParts.size > 1000) {
    state.processedMessageParts.clear();
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
    } catch {
      // Non-critical
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

    // Inject any pending matched skills
    if (state.pendingSkillsContext) {
      output.system.push(state.pendingSkillsContext);
      state.pendingSkillsContext = null;
    }
  };
}
