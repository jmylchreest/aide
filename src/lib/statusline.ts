/**
 * Statusline composer — renders the aide status line at READ time, the way
 * the wider statusline ecosystem works: a pure function of
 *
 *   1. the Claude Code stdin payload (model, context %, cost — native
 *      fields; never re-derived from transcripts),
 *   2. aide's session-scoped state (mode, activity, tool counts, subagents),
 *   3. the session anchor (project identity and estate).
 *
 * Elements render only when they carry signal: no "mode:idle" noise, no
 * empty separators. The composer is pure so the exact lines are pinned by
 * golden tests (src/test/statusline.test.ts).
 */

import type { AgentState, SessionState } from "./hud.js";
import { formatDuration } from "./hud.js";

/** Tolerantly parsed Claude Code statusline stdin payload. */
export interface StatuslinePayload {
  sessionId?: string;
  cwd?: string;
  modelName?: string;
  /** 0-100, from whichever context field the running CC version provides. */
  contextPercent?: number | null;
  costUSD?: number | null;
}

/** aide-side data gathered by the entry script (cached, session-scoped). */
export interface StatuslineData {
  version: string;
  /** Project identity from the anchor; shown only inside an estate. */
  projectName?: string | null;
  /** Nearest parent project name when the anchor chain has one. */
  parentName?: string | null;
  state: SessionState;
  /** Session-scoped currentTool ("Bash(go test ./...)") if a tool is live. */
  currentTool?: string | null;
  /** ISO timestamp of the last tool use, for idle-age display. */
  lastToolUse?: string | null;
  /** "<n>/<max>" iterations when a persistence mode is active. */
  modeIterations?: string | null;
  agents: AgentState[];
}

export type StatuslineFormat = "minimal" | "full";

const MAX_ACTIVITY = 32;

/**
 * Parse the raw stdin JSON into the fields we use, defensively: the payload
 * schema has grown over CC versions and every field is optional.
 */
export function parsePayload(raw: unknown): StatuslinePayload {
  if (!raw || typeof raw !== "object") return {};
  const p = raw as Record<string, any>;
  const num = (v: unknown): number | null =>
    typeof v === "number" && isFinite(v) ? v : null;

  // Context usage has appeared under several shapes across CC versions.
  let contextPercent: number | null = null;
  const ctx = p.context ?? p.context_usage ?? p.context_window ?? null;
  if (ctx && typeof ctx === "object") {
    contextPercent =
      num(ctx.used_percent) ?? num(ctx.percentage) ?? num(ctx.percent);
    if (contextPercent === null) {
      const used = num(ctx.used_tokens) ?? num(ctx.tokens_used);
      const max = num(ctx.max_tokens) ?? num(ctx.token_limit);
      if (used !== null && max) contextPercent = (used / max) * 100;
    }
  } else {
    contextPercent = num(ctx);
  }

  return {
    sessionId: typeof p.session_id === "string" ? p.session_id : undefined,
    cwd: typeof p.cwd === "string" ? p.cwd : undefined,
    modelName:
      typeof p.model?.display_name === "string"
        ? p.model.display_name
        : typeof p.model?.id === "string"
          ? p.model.id
          : undefined,
    contextPercent,
    costUSD: num(p.cost?.total_cost_usd),
  };
}

/** "Bash(go test ./pkg/... -count=1)" -> "Bash: go test ./pkg/..." */
function formatActivity(currentTool: string): string {
  const m = currentTool.match(/^([A-Za-z_]+)\((.*)\)$/s);
  let text = m ? `${m[1]}: ${m[2]}` : currentTool;
  text = text.replace(/\s+/g, " ").trim();
  if (text.length > MAX_ACTIVITY) text = text.slice(0, MAX_ACTIVITY - 1) + "…";
  return text;
}

function contextPart(percent: number): string {
  const pct = Math.round(percent);
  if (pct >= 90) return `ctx ${pct}%‼`;
  if (pct >= 70) return `ctx ${pct}%⚠`;
  return `ctx ${pct}%`;
}

function idleAge(data: StatuslineData): string {
  const since = data.lastToolUse ?? data.state.startedAt;
  return since ? formatDuration(since) : "";
}

/**
 * Compose the status line(s). Line 1 is the main statusline; running
 * subagents append one row each (Claude Code renders multi-line output).
 */
export function composeStatusline(
  payload: StatuslinePayload,
  data: StatuslineData,
  format: StatuslineFormat = "full",
  elements?: string[],
): string {
  const parts: string[] = [];
  // Segment opt-out: an elements list in .aide/config/hud.json drops any
  // payload-derived or optional segment. Activity is always rendered — a
  // statusline that can't say what's happening isn't one.
  const on = (name: string): boolean => !elements || elements.includes(name);

  // Estate: only when this project actually sits inside another.
  if (on("estate") && data.parentName && data.projectName) {
    parts.push(`${data.projectName}⊂${data.parentName}`);
  }

  // Mode: only when one is actually engaged.
  if (on("mode") && data.state.activeMode) {
    const iter = data.modeIterations ? ` ${data.modeIterations}` : "";
    parts.push(`${data.state.activeMode}${iter}`);
  }

  if (format !== "minimal" && on("model") && payload.modelName) {
    parts.push(payload.modelName);
  }

  if (
    on("context") &&
    payload.contextPercent !== null &&
    payload.contextPercent !== undefined
  ) {
    parts.push(contextPart(payload.contextPercent));
  }

  // Activity: the live tool, else idle + how long since the last one.
  if (data.currentTool) {
    parts.push(`▸ ${formatActivity(data.currentTool)}`);
  } else {
    const age = idleAge(data);
    parts.push(age ? `idle ${age}` : "idle");
  }

  if (format !== "minimal" && on("tools") && data.state.toolCalls > 0) {
    parts.push(`⚒${data.state.toolCalls}`);
  }

  const running = data.agents.filter((a) => a.status === "running");
  if (on("agents") && running.length > 0) {
    parts.push(`agents:${running.length}`);
  }

  if (
    format !== "minimal" &&
    on("cost") &&
    payload.costUSD !== null &&
    payload.costUSD !== undefined &&
    payload.costUSD >= 0.01
  ) {
    parts.push(`$${payload.costUSD.toFixed(2)}`);
  }

  const tag =
    format === "full" && data.version ? `[aide ${data.version}]` : "[aide]";
  const main = [tag, parts.join(" | ")].filter(Boolean).join(" ");

  const lines = [main];
  for (const agent of running) {
    const shortId =
      agent.agentId.length > 7 ? agent.agentId.slice(0, 7) : agent.agentId;
    const head = agent.type ? `▶[${shortId}] ${agent.type}` : `▶[${shortId}]`;
    const row: string[] = [head];
    row.push(formatDuration(agent.startedAt));
    if (agent.currentTool) row.push(`▸ ${formatActivity(agent.currentTool)}`);
    else if (agent.task) row.push(agent.task.slice(0, 35));
    lines.push(`└─ ${row.join(" | ")}`);
  }
  return lines.join("\n");
}
