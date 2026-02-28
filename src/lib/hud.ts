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
    ralph: "🔄",
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

/**
 * Get all agent states from aide state store
 */
export function getAgentStates(cwd: string): AgentState[] {
  const output = runAide(cwd, ["state", "list"]);
  if (!output) return [];

  const agents: Map<string, AgentState> = new Map();

  for (const line of output.split("\n")) {
    const agentMatch = line.match(
      /^\[([^\]]+)\]\s+agent:[^:]+:(\w+)\s*=\s*(.+)$/,
    );
    if (agentMatch) {
      const [, agentId, key, value] = agentMatch;
      if (!agents.has(agentId)) {
        agents.set(agentId, {
          agentId,
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
      const agent = agents.get(agentId);
      if (!agent) continue;
      if (key === "mode") agent.mode = value.trim();
      if (key === "startedAt") agent.startedAt = value.trim();
      if (key === "currentTool") agent.currentTool = value.trim();
      if (key === "tasksCompleted")
        agent.tasksCompleted = parseInt(value.trim(), 10) || 0;
      if (key === "tasksTotal")
        agent.tasksTotal = parseInt(value.trim(), 10) || 0;
      if (key === "status") agent.status = value.trim();
      if (key === "type") agent.type = value.trim();
      if (key === "task") agent.task = value.trim();
      if (key === "skill") agent.skill = value.trim();
      if (key === "session") agent.session = value.trim();
    }
  }

  return Array.from(agents.values());
}

/**
 * Load HUD configuration
 */
export function loadHudConfig(cwd: string): HudConfig {
  const configPath = join(cwd, ".aide", "config", "hud.json");

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
 * Get current session state from aide state store
 */
export function getSessionState(cwd: string): SessionState {
  const state: SessionState = {
    activeMode: null,
    agentCount: 0,
    startedAt: null,
    toolCalls: 0,
    lastTool: null,
  };

  const output = runAide(cwd, ["state", "list"]);
  if (!output) return state;

  for (const line of output.split("\n")) {
    const globalMatch = line.match(/^(\w+)\s*=\s*(.+)$/);
    if (globalMatch && !line.startsWith("[")) {
      const [, key, value] = globalMatch;
      if (key === "mode") state.activeMode = value.trim();
      if (key === "agentCount")
        state.agentCount = parseInt(value.trim(), 10) || 0;
      if (key === "startedAt") state.startedAt = value.trim();
      if (key === "toolCalls")
        state.toolCalls = parseInt(value.trim(), 10) || 0;
      if (key === "lastTool") state.lastTool = value.trim();
    }
  }

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
    switch (element) {
      case "mode": {
        const modeName = state.activeMode || "idle";
        const toolSuffix = state.lastTool ? `(${state.lastTool})` : "";
        if (config.format === "icons") {
          const icon =
            ICONS.mode[state.activeMode || "none"] || ICONS.mode.none;
          parts.push(`${icon} ${modeName}${toolSuffix}`);
        } else {
          parts.push(`mode:${modeName}${toolSuffix}`);
        }
        break;
      }

      case "agents": {
        const runningCount = agents.filter(
          (a) => a.status === "running",
        ).length;
        if (runningCount > 0) {
          if (config.format === "icons") {
            parts.push(`${ICONS.agents} ${runningCount}`);
          } else {
            parts.push(`agents:${runningCount}`);
          }
        }
        break;
      }

      case "duration": {
        const duration = formatDuration(state.startedAt);
        if (config.format === "icons") {
          parts.push(`${ICONS.time} ${duration}`);
        } else {
          parts.push(duration);
        }
        break;
      }

      case "tools":
        if (state.toolCalls > 0) {
          if (config.format === "icons") {
            parts.push(`🔧 ${state.toolCalls}`);
          } else {
            parts.push(`tools:${state.toolCalls}`);
          }
        }
        break;

      case "usage": {
        const cacheTTL = config.usageCacheTTL
          ? config.usageCacheTTL * 1000
          : undefined;
        const usage = getUsageSummary(cwd, cacheTTL);
        if (usage) {
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
            parts.push(`📊 ${usageStr}`);
          } else {
            parts.push(usageStr);
          }
        }
        break;
      }
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
  const stateDir = join(cwd, ".aide", "state");
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
  const state = getSessionState(cwd);
  const allAgents = getAgentStates(cwd);

  // Filter to only agents from the current session
  const agents = sessionId
    ? allAgents.filter((a) => a.session === sessionId)
    : [];

  const hudOutput = formatHud(config, state, agents, cwd);
  writeHudOutput(cwd, hudOutput);
}
