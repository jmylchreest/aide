#!/usr/bin/env bun
/**
 * aide-hud.ts - Claude Code statusline entry for aide.
 *
 * Composes the line at READ time (the ecosystem pattern) instead of
 * cat-ing a hook-written file:
 *
 *   stdin payload  -> model, context %, cost (native fields)
 *   session anchor -> project root, identity, estate (sub-ms file read)
 *   aide state     -> mode, activity, tool counts, subagents
 *                     (session-scoped; `state list --json` behind a 2s
 *                     on-disk cache so the ~300ms render cadence costs
 *                     at most one binary spawn every 2 seconds)
 *
 * Fallback ladder when live composition is impossible: the hook-written
 * .aide/state/hud.txt, then a minimal "[aide] idle".
 */

import {
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { join, dirname, sep, basename } from "path";
import { homedir } from "os";
import {
  composeStatusline,
  parsePayload,
  type StatuslineData,
} from "../src/lib/statusline.js";
import {
  hudRenderCacheFile,
  type AgentState,
  type SessionState,
} from "../src/lib/hud.js";

const SESSION_ID_RE = /^[a-zA-Z0-9_-]{1,128}$/;
const STATE_CACHE_TTL_MS = 2000;

interface AnchorIdentity {
  root: string;
  projectName: string | null;
  parentName: string | null;
}

function readStdinRaw(): unknown {
  try {
    if (process.stdin.isTTY) return {};
    const raw = readFileSync(0, "utf-8");
    return raw.trim() ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

/** Same location contract as lib/anchor.ts anchorCacheDirs. */
function anchorCacheDirs(): string[] {
  const dirs: string[] = [];
  const xdg = process.env.XDG_RUNTIME_DIR;
  if (xdg && existsSync(xdg)) dirs.push(join(xdg, "aide"));
  dirs.push(join(homedir(), ".aide"));
  return dirs;
}

function readAnchor(sessionId: string, cwd?: string): AnchorIdentity | null {
  try {
    if (!SESSION_ID_RE.test(sessionId)) return null;
    const p = anchorCacheDirs()
      .map((d) => join(d, "anchors", `${sessionId}.json`))
      .find((f) => existsSync(f));
    if (!p) return null;
    const entry = JSON.parse(readFileSync(p, "utf-8"));
    const anchor = entry?.anchor;
    const root = anchor?.root;
    if (anchor?.schemaVersion !== 1 || typeof root !== "string" || !root)
      return null;
    if (!existsSync(root)) return null;
    if (cwd && cwd !== root && !cwd.startsWith(root + sep)) return null;

    let parentName: string | null = null;
    const chain = Array.isArray(anchor.chain) ? anchor.chain : [];
    if (chain.length > 1 && typeof chain[1]?.root === "string") {
      parentName = basename(chain[1].root);
    }
    return {
      root,
      projectName:
        typeof anchor.identity?.projectName === "string"
          ? anchor.identity.projectName
          : basename(root),
      parentName,
    };
  } catch {
    return null;
  }
}

function walkForAide(startDir: string): string | null {
  let dir = startDir;
  while (true) {
    if (existsSync(join(dir, ".aide"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

function findBinary(root: string): string | null {
  const local = join(
    root,
    ".aide",
    "bin",
    process.platform === "win32" ? "aide.exe" : "aide",
  );
  if (existsSync(local)) return local;
  return "aide"; // PATH fallback; execFileSync throws if absent
}

interface StateEntry {
  key: string;
  value: string;
  agent?: string;
}

/**
 * `aide state list --json`, behind a per-session on-disk cache: the
 * statusline renders every ~300ms but a state snapshot a second old is
 * indistinguishable in a one-line display.
 */
function readStateCached(root: string, sessionId: string): StateEntry[] {
  const cachePath = hudRenderCacheFile(sessionId);
  if (!cachePath) return [];
  const cacheDir = dirname(cachePath);
  try {
    const cached = JSON.parse(readFileSync(cachePath, "utf-8"));
    if (
      typeof cached?.ts === "number" &&
      Date.now() - cached.ts < STATE_CACHE_TTL_MS &&
      Array.isArray(cached.entries)
    ) {
      return cached.entries as StateEntry[];
    }
  } catch {
    /* miss */
  }

  const binary = findBinary(root);
  let entries: StateEntry[] = [];
  try {
    const out = execFileSync(binary!, ["state", "list", "--json"], {
      cwd: root,
      encoding: "utf-8",
      timeout: 3000,
      stdio: ["ignore", "pipe", "ignore"],
    });
    const parsed = JSON.parse(out);
    if (Array.isArray(parsed)) entries = parsed as StateEntry[];
  } catch {
    return [];
  }

  try {
    mkdirSync(cacheDir, { recursive: true });
    const tmp = `${cachePath}.tmp-${process.pid}`;
    writeFileSync(tmp, JSON.stringify({ ts: Date.now(), entries }));
    renameSync(tmp, cachePath);
  } catch {
    /* cache write is best-effort */
  }
  return entries;
}

function buildData(
  root: string,
  sessionId: string | undefined,
  identity: AnchorIdentity | null,
): StatuslineData | null {
  if (!sessionId) return null;
  const entries = readStateCached(root, sessionId);
  if (entries.length === 0) return null;

  const globals: Record<string, string> = {};
  const scoped: Record<string, string> = {};
  const agents = new Map<string, AgentState>();
  const scopedPrefix = `agent:${sessionId}:`;

  for (const e of entries) {
    const value = (e.value ?? "").trim();
    if (!e.agent) {
      globals[e.key] = value;
      continue;
    }
    if (e.agent === sessionId && e.key.startsWith(scopedPrefix)) {
      scoped[e.key.slice(scopedPrefix.length)] = value;
      continue;
    }
    // Subagent rows: agent:<id>:<field> for non-session agents.
    const prefix = `agent:${e.agent}:`;
    if (!e.key.startsWith(prefix)) continue;
    const field = e.key.slice(prefix.length);
    if (!agents.has(e.agent)) {
      agents.set(e.agent, {
        agentId: e.agent,
        mode: null,
        startedAt: null,
        currentTool: null,
        lastTool: null,
        toolCalls: 0,
        tasksCompleted: 0,
        tasksTotal: 0,
        status: null,
        type: null,
        task: null,
        skill: null,
        session: null,
      });
    }
    const a = agents.get(e.agent)!;
    if (field === "startedAt") a.startedAt = value;
    if (field === "currentTool") a.currentTool = value;
    if (field === "lastTool") a.lastTool = value;
    if (field === "toolCalls") a.toolCalls = parseInt(value, 10) || 0;
    if (field === "status") a.status = value;
    if (field === "type") a.type = value;
    if (field === "task") a.task = value;
    if (field === "session") a.session = value;
  }

  const pick = (k: string): string | null => scoped[k] ?? globals[k] ?? null;
  const activeMode = pick("mode");
  const state: SessionState = {
    activeMode,
    agentCount: 0,
    startedAt: pick("startedAt"),
    toolCalls: parseInt(pick("toolCalls") || "0", 10) || 0,
    lastTool: pick("lastTool"),
  };

  // Only this session's subagents belong on this session's statusline.
  const sessionAgents = Array.from(agents.values()).filter(
    (a) => a.session === sessionId || a.session === null,
  );

  return {
    version: "", // filled by caller
    projectName: identity?.projectName ?? null,
    parentName: identity?.parentName ?? null,
    state,
    currentTool: scoped["currentTool"] ?? null,
    lastToolUse: pick("lastToolUse"),
    modeIterations: activeMode
      ? (pick(`${activeMode}_iterations`) ?? null)
      : null,
    agents: sessionAgents,
  };
}

/**
 * Statusline config, mirroring the Go config's layering for the `hud`
 * section: defaults -> global ~/.aide/config/aide.json -> project
 * aide.json -> legacy .aide/config/hud.json (kept winning for setups
 * that predate the aide.json hud section) -> AIDE_HUD_* env vars.
 * `aide config set hud.format minimal` / `hud.segments dir context ...`
 * is the supported way to write these.
 */
function readHudConfig(root: string): {
  format: "minimal" | "full";
  segments?: string[];
} {
  const out: { format: "minimal" | "full"; segments?: string[] } = {
    format: "full",
  };
  const apply = (h: unknown): void => {
    if (!h || typeof h !== "object") return;
    const c = h as Record<string, unknown>;
    if (c.format === "minimal" || c.format === "full") out.format = c.format;
    if (
      Array.isArray(c.segments) &&
      c.segments.every((e) => typeof e === "string")
    ) {
      out.segments = c.segments as string[];
    }
  };
  const layers: Array<[string, boolean]> = [
    [join(homedir(), ".aide", "config", "aide.json"), true],
    [join(root, ".aide", "config", "aide.json"), true],
    [join(root, ".aide", "config", "hud.json"), false],
  ];
  for (const [p, nested] of layers) {
    try {
      const raw = JSON.parse(readFileSync(p, "utf-8"));
      apply(nested ? raw?.hud : raw);
    } catch {
      /* layer absent */
    }
  }
  const envFormat = process.env.AIDE_HUD_FORMAT;
  if (envFormat === "minimal" || envFormat === "full") out.format = envFormat;
  const envSegments = process.env.AIDE_HUD_SEGMENTS;
  if (envSegments) {
    out.segments = envSegments
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }
  return out;
}

function readVersion(root: string): string {
  // The anchor-era binary stamps its version into hud.txt's tag; cheaper
  // than spawning `aide version` per render: parse it opportunistically.
  try {
    const hud = readFileSync(join(root, ".aide", "state", "hud.txt"), "utf-8");
    const m = hud.match(/\[aide\(([^)]+)\)\]/);
    if (m) return m[1];
  } catch {
    /* fall through */
  }
  return "";
}

// ---- main ----

const raw = readStdinRaw();
const payload = parsePayload(raw);
const startCwd = payload.cwd || process.cwd();

let identity: AnchorIdentity | null = null;
if (payload.sessionId) identity = readAnchor(payload.sessionId, startCwd);

let root = identity?.root ?? null;
if (!root) {
  const override = process.env.AIDE_PROJECT_ROOT;
  if (override && existsSync(override)) root = override;
}
if (!root) root = walkForAide(startCwd) ?? startCwd;

const data = buildData(root, payload.sessionId, identity);
if (data) {
  data.version = readVersion(root);
  data.homeDir = homedir();
  const cfg = readHudConfig(root);
  console.log(composeStatusline(payload, data, cfg.format, cfg.segments));
  process.exit(0);
}

// Fallback: the hook-written file, then minimal.
try {
  const content = readFileSync(
    join(root, ".aide", "state", "hud.txt"),
    "utf-8",
  ).trim();
  if (content) {
    console.log(content.split("\n")[0]);
    process.exit(0);
  }
} catch {
  /* fall through */
}
console.log("[aide] idle");
