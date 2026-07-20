/**
 * Shared HUD formatting and writing logic
 *
 * Used by hud-updater.ts (PostToolUse) and subagent-tracker.ts (SubagentStart/Stop)
 * to maintain a consistent status line display.
 */

import { existsSync, readFileSync, writeFileSync, mkdirSync } from "fs";
import { join } from "path";
import { execFileSync } from "child_process";
import { runAide, findAideBinary } from "./hook-utils.js";
import { findProjectRoot } from "./project-root.js";

// Cache the aide version for the session (won't change)
let aideVersionCache: string | null = null;

/**
 * Get the aide binary version (cached)
 */
export function getAideVersion(cwd: string): string {
  if (aideVersionCache !== null) {
    return aideVersionCache;
  }

  const binary = findAideBinary(cwd);
  if (!binary) {
    aideVersionCache = "?";
    return aideVersionCache;
  }

  try {
    const output = execFileSync(binary, ["version"], {
      stdio: "pipe",
      timeout: 2000,
    })
      .toString()
      .trim();
    // Expected format: "aide version 0.0.5-dev.12+abc1234" or just "0.0.4"
    const match = output.match(
      /(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?(?:\+[a-zA-Z0-9.]+)?)/,
    );
    aideVersionCache = match ? match[1] : "?";
  } catch {
    aideVersionCache = "?";
  }

  return aideVersionCache;
}

export interface AgentState {
  agentId: string;
  mode: string | null;
  startedAt: string | null;
  currentTool: string | null;
  tasksCompleted: number;
  tasksTotal: number;
  status: string | null;
  type: string | null;
  task: string | null;
  skill: string | null;
  session: string | null;
}

export interface HudConfig {
  enabled: boolean;
  elements: string[];
  format: "minimal" | "full" | "icons";
  usageCacheTTL?: number; // Cache TTL in seconds for usage data (default: 30)
}

export interface SessionState {
  activeMode: string | null;
  agentCount: number;
  startedAt: string | null;
  toolCalls: number;
  lastTool: string | null;
}

const DEFAULT_HUD_CONFIG: HudConfig = {
  enabled: true,
  elements: ["mode", "duration", "agents", "usage"],
  format: "minimal",
};

const ICONS = {
  mode: {
    autopilot: "🚀",
    eco: "💚",
    swarm: "🐝",
    plan: "📋",
    working: "⚙️",
    none: "⚪",
  } as Record<string, string>,
  agents: "👥",
  tools: "🔧",
  time: "⏱️",
};

/** One row of `aide state list --json`. */
interface StateEntry {
  key: string;
  value: string;
  agent?: string;
}

/**
 * Fetch all state entries as structured rows via `state list --json`.
 *
 * NOTE: the plain `state list` output is a tabwriter table (AGENT/KEY/VALUE
 * columns) — earlier versions of this file tried to regex `key = value`
 * lines out of it and silently matched nothing. JSON is the stable contract.
 */
function listStateEntries(cwd: string): StateEntry[] {
  const output = runAide(cwd, ["state", "list", "--json"]);
  if (!output) return [];
  try {
    const parsed = JSON.parse(output);
    return Array.isArray(parsed) ? (parsed as StateEntry[]) : [];
  } catch {
    return [];
  }
}

/** Strip the "agent:<id>:" prefix from a scoped state key. */
function scopedKeyField(key: string, agentId: string): string | null {
  const prefix = `agent:${agentId}:`;
  return key.startsWith(prefix) ? key.slice(prefix.length) : null;
}

/**
 * Get all agent states from aide state store
 */
export function getAgentStates(cwd: string): AgentState[] {
  const agents: Map<string, AgentState> = new Map();

  for (const entry of listStateEntries(cwd)) {
    if (!entry.agent) continue;
    const key = scopedKeyField(entry.key, entry.agent);
    if (!key) continue;
    const value = (entry.value ?? "").trim();

    if (!agents.has(entry.agent)) {
      agents.set(entry.agent, {
        agentId: entry.agent,
        mode: null,
        startedAt: null,
        currentTool: null,
        tasksCompleted: 0,
        tasksTotal: 0,
        status: null,
        type: null,
        task: null,
        skill: null,
        session: null,
      });
    }
    const agent = agents.get(entry.agent);
    if (!agent) continue;
    if (key === "mode") agent.mode = value;
    if (key === "startedAt") agent.startedAt = value;
    if (key === "currentTool") agent.currentTool = value;
    if (key === "tasksCompleted") agent.tasksCompleted = parseInt(value, 10) || 0;
    if (key === "tasksTotal") agent.tasksTotal = parseInt(value, 10) || 0;
    if (key === "status") agent.status = value;
    if (key === "type") agent.type = value;
    if (key === "task") agent.task = value;
    if (key === "skill") agent.skill = value;
    if (key === "session") agent.session = value;
  }

  return Array.from(agents.values());
}

/**
 * Load HUD configuration
 */
export function loadHudConfig(cwd: string): HudConfig {
  const { root } = findProjectRoot(cwd);
  const configPath = join(root, ".aide", "config", "hud.json");

  if (existsSync(configPath)) {
    try {
      const content = readFileSync(configPath, "utf-8");
      return { ...DEFAULT_HUD_CONFIG, ...JSON.parse(content) };
    } catch {
      return DEFAULT_HUD_CONFIG;
    }
  }

  return DEFAULT_HUD_CONFIG;
}

/**
 * Get current session state from aide state store.
 *
 * Session-descriptive keys are written session-scoped
 * (agent:<sessionId>:<key>) so concurrent sessions don't clobber each other;
 * bare global spellings remain as a fallback for sessionless writers.
 * Scoped entries win over globals when a sessionId is provided.
 */
export function getSessionState(cwd: string, sessionId?: string): SessionState {
  const state: SessionState = {
    activeMode: null,
    agentCount: 0,
    startedAt: null,
    toolCalls: 0,
    lastTool: null,
  };

  const globals: Record<string, string> = {};
  const scoped: Record<string, string> = {};

  for (const entry of listStateEntries(cwd)) {
    if (!entry.agent) {
      globals[entry.key] = (entry.value ?? "").trim();
      continue;
    }
    if (sessionId && entry.agent === sessionId) {
      const key = scopedKeyField(entry.key, entry.agent);
      if (key) scoped[key] = (entry.value ?? "").trim();
    }
  }

  const pick = (key: string): string | null =>
    scoped[key] ?? globals[key] ?? null;

  state.activeMode = pick("mode");
  state.agentCount = parseInt(pick("agentCount") || "0", 10) || 0;
  state.startedAt = pick("startedAt");
  state.toolCalls = parseInt(pick("toolCalls") || "0", 10) || 0;
  state.lastTool = pick("lastTool");

  return state;
}

/**
 * Format duration as human-readable string
 */
export function formatDuration(startedAt: string | null): string {
  if (!startedAt) return "0m";

  const start = new Date(startedAt).getTime();
  const now = Date.now();
  const diffMs = now - start;

  if (diffMs < 0) return "0m";

  const seconds = Math.floor(diffMs / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h${minutes % 60}m`;
  } else if (minutes > 0) {
    return `${minutes}m`;
  } else {
    return `${seconds}s`;
  }
}

/**
 * Get usage for HUD display (cached internally by usage module).
 * Uses OAuth API for accurate percentages, falls back to token counts.
 */
let hudUsageCache: {
  data: {
    fiveHourPercent: number | null;
    fiveHourRemain: string | null;
    weeklyPercent: number | null;
    weeklyRemain: string | null;
    window5hTokens: number;
    todayTokens: number;
  } | null;
  timestamp: number;
} | null = null;
const DEFAULT_USAGE_CACHE_TTL = 30_000; // 30 seconds

export function getUsageSummary(
  _cwd: string,
  cacheTTL?: number,
): {
  fiveHourPercent: number | null;
  fiveHourRemain: string | null;
  weeklyPercent: number | null;
  weeklyRemain: string | null;
  window5hTokens: number;
  todayTokens: number;
} | null {
  const now = Date.now();
  const ttl = cacheTTL ?? DEFAULT_USAGE_CACHE_TTL;

  // Return cached data synchronously if fresh
  if (hudUsageCache && now - hudUsageCache.timestamp < ttl) {
    return hudUsageCache.data;
  }

  // Trigger async refresh in background, return stale data or null
  refreshUsageCache();

  return hudUsageCache?.data ?? null;
}

/**
 * Refresh usage cache asynchronously.
 * Uses promise-based dedup to prevent concurrent refreshes.
 * Dynamic import avoids circular dependencies and keeps the
 * synchronous HUD path fast.
 */
let refreshPromise: Promise<void> | null = null;

async function refreshUsageCache(): Promise<void> {
  if (refreshPromise) return;

  refreshPromise = (async () => {
    try {
      const { getUsageForHud } = await import("./usage.js");
      const result = await getUsageForHud();
      hudUsageCache = { data: result, timestamp: Date.now() };
    } catch {
      // Keep stale cache on error
    }
  })();

  try {
    await refreshPromise;
  } finally {
    refreshPromise = null;
  }
}

type ElementFormatter = (
  state: SessionState,
  agents: AgentState[],
  config: HudConfig,
  cwd: string,
) => string | null;

const elementFormatters: Record<string, ElementFormatter> = {
  mode: (state, _agents, config) => {
    const modeName = state.activeMode || "idle";
    const toolSuffix = state.lastTool ? `(${state.lastTool})` : "";
    if (config.format === "icons") {
      const icon = ICONS.mode[state.activeMode || "none"] || ICONS.mode.none;
      return `${icon} ${modeName}${toolSuffix}`;
    }
    return `mode:${modeName}${toolSuffix}`;
  },

  agents: (_state, agents, config) => {
    const runningCount = agents.filter((a) => a.status === "running").length;
    if (runningCount <= 0) return null;
    if (config.format === "icons") {
      return `${ICONS.agents} ${runningCount}`;
    }
    return `agents:${runningCount}`;
  },

  duration: (state, _agents, config) => {
    const duration = formatDuration(state.startedAt);
    if (config.format === "icons") {
      return `${ICONS.time} ${duration}`;
    }
    return duration;
  },

  tools: (state, _agents, config) => {
    if (state.toolCalls <= 0) return null;
    if (config.format === "icons") {
      return `🔧 ${state.toolCalls}`;
    }
    return `tools:${state.toolCalls}`;
  },

  usage: (_state, _agents, config, cwd) => {
    const cacheTTL = config.usageCacheTTL
      ? config.usageCacheTTL * 1000
      : undefined;
    const usage = getUsageSummary(cwd, cacheTTL);
    if (!usage) return null;

    const fmt = (n: number) =>
      n >= 1000000
        ? `${(n / 1000000).toFixed(1)}M`
        : n >= 1000
          ? `${(n / 1000).toFixed(0)}K`
          : `${n}`;

    let usageStr: string;
    if (usage.fiveHourPercent !== null) {
      // API-sourced percentages (accurate)
      const remainStr =
        usage.fiveHourRemain && usage.fiveHourRemain !== "expired"
          ? ` ~${usage.fiveHourRemain}`
          : "";
      const weekStr =
        usage.weeklyPercent !== null
          ? ` wk:${Math.round(usage.weeklyPercent)}%`
          : "";
      usageStr = `5h:${Math.round(usage.fiveHourPercent)}%${remainStr}${weekStr}`;
    } else {
      // Fallback to weighted token counts
      usageStr = `5h:${fmt(usage.window5hTokens)}`;
    }

    if (config.format === "icons") {
      return `📊 ${usageStr}`;
    }
    return usageStr;
  },
};

/**
 * Format HUD output based on config
 */
export function formatHud(
  config: HudConfig,
  state: SessionState,
  agents: AgentState[] = [],
  cwd: string = ".",
): string {
  if (!config.enabled) return "";

  const parts: string[] = [];

  for (const element of config.elements) {
    const formatter = elementFormatters[element];
    if (formatter) {
      const part = formatter(state, agents, config, cwd);
      if (part) parts.push(part);
    }
  }

  // Get aide version for display
  const aideVersion = getAideVersion(cwd);
  const versionTag = `[aide(${aideVersion})]`;

  let mainLine: string;
  if (config.format === "minimal") {
    mainLine = `${versionTag} ${parts.join(" | ")}`;
  } else if (config.format === "icons") {
    mainLine = `${versionTag} ${parts.join("  ")}`;
  } else {
    mainLine = `${versionTag} ${parts.join(" | ")}`;
  }

  // Add per-agent lines for running agents
  const lines: string[] = [mainLine];
  const runningAgents = agents.filter((a) => a.status === "running");

  for (const agent of runningAgents) {
    const agentDuration = formatDuration(agent.startedAt);
    const shortId =
      agent.agentId.length > 7 ? agent.agentId.slice(0, 7) : agent.agentId;

    const agentParts: string[] = [`▶[${shortId}]`];

    if (agent.type) {
      agentParts.push(agent.type);
    }

    agentParts.push(agentDuration);

    if (agent.currentTool) {
      const toolDesc =
        agent.currentTool.length > 25
          ? agent.currentTool.slice(0, 22) + "..."
          : agent.currentTool;
      agentParts.push(`→ ${toolDesc}`);
    } else if (agent.task) {
      const taskDesc =
        agent.task.length > 35 ? agent.task.slice(0, 32) + "..." : agent.task;
      agentParts.push(taskDesc);
    }

    lines.push(`└─ ${agentParts.join(" | ")}`);
  }

  return lines.join("\n");
}

/**
 * Write HUD output to state file
 */
export function writeHudOutput(cwd: string, output: string): void {
  const { root } = findProjectRoot(cwd);
  const stateDir = join(root, ".aide", "state");
  if (!existsSync(stateDir)) {
    try {
      mkdirSync(stateDir, { recursive: true });
    } catch {
      return;
    }
  }

  const hudPath = join(stateDir, "hud.txt");
  try {
    writeFileSync(hudPath, output);
  } catch {
    // Ignore write errors
  }
}

/**
 * Refresh the HUD display - reads state, formats, and writes
 * Can be called from any hook that needs to trigger a HUD update.
 */
export function refreshHud(cwd: string, sessionId?: string): void {
  const config = loadHudConfig(cwd);
  const state = getSessionState(cwd, sessionId);
  const allAgents = getAgentStates(cwd);

  // Filter to only agents from the current session
  const agents = sessionId
    ? allAgents.filter((a) => a.session === sessionId)
    : [];

  const hudOutput = formatHud(config, state, agents, cwd);
  writeHudOutput(cwd, hudOutput);
}
