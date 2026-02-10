/**
 * Claude Code Usage Module
 *
 * Provides accurate usage statistics by combining:
 * 1. OAuth API (api.anthropic.com/api/oauth/usage) for authoritative utilization percentages
 * 2. Local JSONL session file scanning for raw token counts
 *
 * The OAuth API returns server-computed utilization percentages that match
 * what Claude's web UI shows. No client-side limit guessing needed.
 */

import { readFileSync, readdirSync, statSync } from "fs";
import { join, basename } from "path";
import { homedir } from "os";
import { createReadStream } from "fs";
import { createInterface } from "readline";

// =============================================================================
// Types
// =============================================================================

export interface APILimits {
  fiveHourPercent: number;
  fiveHourResetsAt: string | null;
  fiveHourRemain: string | null;
  weeklyPercent: number;
  weeklyResetsAt: string | null;
  weeklyRemain: string | null;
  weeklySonnetPercent: number | null;
  weeklyOpusPercent: number | null;
  extraUsageEnabled: boolean;
  extraUsageLimit: number;
  extraUsageUsed: number;
  error: string | null;
}

export interface TokenBucket {
  input: number;
  output: number;
  cacheRead: number;
  cacheCreate: number;
  total: number;
  weightedTotal: number;
}

export interface RealtimeUsage {
  window5h: TokenBucket;
  today: TokenBucket;
  messages5h: number;
  messagesToday: number;
}

export interface UsageSummary {
  limits: APILimits | null;
  realtime: RealtimeUsage;
  timestamp: string;
}

interface OAuthCredentials {
  claudeAiOauth?: {
    accessToken: string;
    expiresAt: number;
    refreshToken: string;
  };
}

interface OAuthUsageResponse {
  five_hour?: { utilization: number; resets_at: string };
  seven_day?: { utilization: number; resets_at: string };
  seven_day_sonnet?: { utilization: number; resets_at: string };
  seven_day_opus?: { utilization: number; resets_at: string };
  extra_usage?: {
    is_enabled: boolean;
    monthly_limit: number;
    used_credits: number;
  };
}

// =============================================================================
// Caching
// =============================================================================

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

let apiLimitsCache: CacheEntry<APILimits> | null = null;
const API_CACHE_TTL = 30_000; // 30 seconds

let realtimeCache: CacheEntry<RealtimeUsage> | null = null;
let realtimeCacheTTL = 60_000; // 1 minute default, configurable

/**
 * Set the cache TTL for realtime usage data (JSONL scanning).
 */
export function setRealtimeCacheTTL(ms: number): void {
  realtimeCacheTTL = ms;
}

// =============================================================================
// OAuth API
// =============================================================================

const ANTHROPIC_OAUTH_BETA_VERSION = "oauth-2025-04-20";

/**
 * Read Claude OAuth credentials from ~/.claude/.credentials.json
 */
function readCredentials(): OAuthCredentials | null {
  try {
    const credPath = join(homedir(), ".claude", ".credentials.json");
    const data = readFileSync(credPath, "utf-8");
    return JSON.parse(data);
  } catch {
    return null;
  }
}

/**
 * Format a duration as human-readable remaining time.
 */
function formatRemaining(ms: number): string {
  if (ms <= 0) return "expired";

  const totalMinutes = Math.floor(ms / 60_000);
  const hours = Math.floor(totalMinutes / 60);
  const mins = totalMinutes % 60;

  if (hours > 24) {
    const days = Math.floor(hours / 24);
    const remainHours = hours % 24;
    return `${days}d${remainHours}h`;
  }
  if (hours > 0) {
    return `${hours}h${mins}m`;
  }
  if (mins > 0) {
    return `${mins}m`;
  }
  return "<1m";
}

/**
 * Fetch utilization percentages from Anthropic's OAuth usage API.
 * Returns cached data if within TTL.
 */
export async function fetchAPILimits(): Promise<APILimits> {
  const now = Date.now();

  // Return cached data if fresh
  if (apiLimitsCache && now - apiLimitsCache.timestamp < API_CACHE_TTL) {
    return apiLimitsCache.data;
  }

  const creds = readCredentials();
  if (!creds?.claudeAiOauth) {
    return { ...emptyLimits(), error: "credentials not found" };
  }

  const token = creds.claudeAiOauth.accessToken;
  if (!token) {
    return { ...emptyLimits(), error: "no access token" };
  }

  // Check if token is expired
  if (creds.claudeAiOauth.expiresAt > 0 && now > creds.claudeAiOauth.expiresAt) {
    return { ...emptyLimits(), error: "token expired" };
  }

  try {
    const resp = await fetch("https://api.anthropic.com/api/oauth/usage", {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
        "anthropic-beta": ANTHROPIC_OAUTH_BETA_VERSION,
        "Content-Type": "application/json",
      },
      signal: AbortSignal.timeout(5000),
    });

    if (!resp.ok) {
      let errorDetail = `API status ${resp.status}`;
      try {
        const body = await resp.text();
        if (body.length > 0 && body.length < 500) {
          errorDetail += `: ${body}`;
        }
      } catch {
        // ignore body parse errors
      }
      return { ...emptyLimits(), error: errorDetail };
    }

    const data = (await resp.json()) as OAuthUsageResponse;
    const limits = parseOAuthResponse(data);

    // Cache successful result
    apiLimitsCache = { data: limits, timestamp: now };
    return limits;
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    return { ...emptyLimits(), error: `API error: ${msg}` };
  }
}

function parseOAuthResponse(data: OAuthUsageResponse): APILimits {
  const now = Date.now();
  const limits: APILimits = emptyLimits();

  if (data.five_hour) {
    limits.fiveHourPercent = data.five_hour.utilization;
    limits.fiveHourResetsAt = data.five_hour.resets_at;
    try {
      const resetMs = new Date(data.five_hour.resets_at).getTime() - now;
      limits.fiveHourRemain = formatRemaining(resetMs);
    } catch {
      // ignore parse errors
    }
  }

  if (data.seven_day) {
    limits.weeklyPercent = data.seven_day.utilization;
    limits.weeklyResetsAt = data.seven_day.resets_at;
    try {
      const resetMs = new Date(data.seven_day.resets_at).getTime() - now;
      limits.weeklyRemain = formatRemaining(resetMs);
    } catch {
      // ignore parse errors
    }
  }

  if (data.seven_day_sonnet) {
    limits.weeklySonnetPercent = data.seven_day_sonnet.utilization;
  }

  if (data.seven_day_opus) {
    limits.weeklyOpusPercent = data.seven_day_opus.utilization;
  }

  if (data.extra_usage) {
    limits.extraUsageEnabled = data.extra_usage.is_enabled;
    limits.extraUsageLimit = data.extra_usage.monthly_limit;
    limits.extraUsageUsed = data.extra_usage.used_credits;
  }

  return limits;
}

function emptyLimits(): APILimits {
  return {
    fiveHourPercent: 0,
    fiveHourResetsAt: null,
    fiveHourRemain: null,
    weeklyPercent: 0,
    weeklyResetsAt: null,
    weeklyRemain: null,
    weeklySonnetPercent: null,
    weeklyOpusPercent: null,
    extraUsageEnabled: false,
    extraUsageLimit: 0,
    extraUsageUsed: 0,
    error: null,
  };
}

// =============================================================================
// JSONL Token Scanning
// =============================================================================

function emptyBucket(): TokenBucket {
  return {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheCreate: 0,
    total: 0,
    weightedTotal: 0,
  };
}

/**
 * Scan JSONL session files for token usage data.
 * Returns cached data if within TTL.
 */
export async function scanTokenUsage(): Promise<RealtimeUsage> {
  const now = Date.now();

  if (realtimeCache && now - realtimeCache.timestamp < realtimeCacheTTL) {
    return realtimeCache.data;
  }

  const home = homedir();
  const usage: RealtimeUsage = {
    window5h: emptyBucket(),
    today: emptyBucket(),
    messages5h: 0,
    messagesToday: 0,
  };

  const projectsDir = join(home, ".claude", "projects");
  const todayStr = new Date().toISOString().slice(0, 10);
  const fiveHoursAgo = now - 5 * 60 * 60 * 1000;

  let jsonlFiles: string[];
  try {
    jsonlFiles = findJSONLFiles(projectsDir, todayStr);
  } catch {
    realtimeCache = { data: usage, timestamp: now };
    return usage;
  }

  for (const filePath of jsonlFiles) {
    await scanSingleFile(filePath, todayStr, fiveHoursAgo, usage);
  }

  // Calculate totals
  usage.today.total = usage.today.input + usage.today.output;
  usage.today.weightedTotal =
    usage.today.input +
    usage.today.output +
    Math.floor(usage.today.cacheRead * 0.1) +
    Math.floor(usage.today.cacheCreate * 1.25);

  usage.window5h.total = usage.window5h.input + usage.window5h.output;
  usage.window5h.weightedTotal =
    usage.window5h.input +
    usage.window5h.output +
    Math.floor(usage.window5h.cacheRead * 0.1) +
    Math.floor(usage.window5h.cacheCreate * 1.25);

  realtimeCache = { data: usage, timestamp: now };
  return usage;
}

/**
 * Find JSONL files modified today across all project directories.
 */
function findJSONLFiles(projectsDir: string, todayStr: string): string[] {
  const files: string[] = [];

  function walkDir(dir: string): void {
    try {
      const entries = readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        const fullPath = join(dir, entry.name);
        if (entry.isDirectory()) {
          walkDir(fullPath);
        } else if (entry.name.endsWith(".jsonl")) {
          try {
            const stat = statSync(fullPath);
            if (stat.mtime.toISOString().slice(0, 10) >= todayStr) {
              files.push(fullPath);
            }
          } catch {
            // skip
          }
        }
      }
    } catch {
      // skip inaccessible directories
    }
  }

  walkDir(projectsDir);
  return files;
}

/**
 * Scan a single JSONL file for usage data.
 */
async function scanSingleFile(
  filePath: string,
  todayStr: string,
  fiveHoursAgoMs: number,
  usage: RealtimeUsage,
): Promise<void> {
  return new Promise((resolve) => {
    try {
      const stream = createReadStream(filePath, { encoding: "utf-8" });
      const rl = createInterface({ input: stream, crlfDelay: Infinity });

      rl.on("line", (line) => {
        // Fast pre-filter
        if (!line.includes('"usage"')) return;

        try {
          const msg = JSON.parse(line);
          if (
            !msg.message ||
            msg.message.role !== "assistant" ||
            !msg.message.usage
          )
            return;

          const ts = new Date(msg.timestamp).getTime();
          if (isNaN(ts)) return;

          const msgDate = new Date(msg.timestamp).toISOString().slice(0, 10);
          if (msgDate !== todayStr) return;

          const u = msg.message.usage;
          usage.today.input += u.input_tokens || 0;
          usage.today.output += u.output_tokens || 0;
          usage.today.cacheRead += u.cache_read_input_tokens || 0;
          usage.today.cacheCreate += u.cache_creation_input_tokens || 0;
          usage.messagesToday++;

          if (ts > fiveHoursAgoMs) {
            usage.window5h.input += u.input_tokens || 0;
            usage.window5h.output += u.output_tokens || 0;
            usage.window5h.cacheRead += u.cache_read_input_tokens || 0;
            usage.window5h.cacheCreate += u.cache_creation_input_tokens || 0;
            usage.messages5h++;
          }
        } catch {
          // skip malformed lines
        }
      });

      rl.on("close", resolve);
      rl.on("error", () => resolve());
    } catch {
      resolve();
    }
  });
}

// =============================================================================
// Combined Usage
// =============================================================================

/**
 * Get combined usage data: API limits + local token counts.
 */
export async function getUsage(): Promise<UsageSummary> {
  const [limits, realtime] = await Promise.all([
    fetchAPILimits(),
    scanTokenUsage(),
  ]);

  return {
    limits: limits.error ? null : limits,
    realtime,
    timestamp: new Date().toISOString(),
  };
}

/**
 * Get usage formatted for HUD display.
 * Prioritizes API percentages, falls back to token counts.
 */
export async function getUsageForHud(): Promise<{
  fiveHourPercent: number | null;
  fiveHourRemain: string | null;
  weeklyPercent: number | null;
  weeklyRemain: string | null;
  window5hTokens: number;
  todayTokens: number;
} | null> {
  try {
    const limits = await fetchAPILimits();
    const hasAPI = !limits.error;

    // Only scan JSONL if API is unavailable (for fallback token display)
    let window5hTokens = 0;
    let todayTokens = 0;
    if (!hasAPI) {
      const realtime = await scanTokenUsage();
      window5hTokens = realtime.window5h.weightedTotal;
      todayTokens = realtime.today.weightedTotal;
    }

    return {
      fiveHourPercent: hasAPI ? limits.fiveHourPercent : null,
      fiveHourRemain: hasAPI ? limits.fiveHourRemain : null,
      weeklyPercent: hasAPI ? limits.weeklyPercent : null,
      weeklyRemain: hasAPI ? limits.weeklyRemain : null,
      window5hTokens,
      todayTokens,
    };
  } catch {
    return null;
  }
}
