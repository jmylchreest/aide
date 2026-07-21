/**
 * Anchor reader — the TS side of `aide anchor`.
 *
 * The Go binary is the single resolution authority (resolveAnchor in
 * cmd_anchor.go). Session start shells out once and persists the result;
 * consumers migrate onto getAnchor/getAnchoredRoot incrementally (hooks
 * still walking via findProjectRoot are the remaining migration surface):
 *
 *   ~/.aide/anchors/<session_id>.json   session-keyed cache — O(1) lookup
 *                                       from a hook payload's session_id,
 *                                       no walk, no chicken-and-egg
 *   <root>/.aide/state/anchor.json      project-local copy (state/ is
 *                                       gitignored) for inspection and for
 *                                       consumers without a session id.
 *                                       Last-writer-wins: all worktrees of
 *                                       one repo share the main root, so
 *                                       concurrent sessions overwrite each
 *                                       other here — the session-keyed
 *                                       cache is the authoritative lookup
 *
 * Readers validate before trusting: schema version, launchCwd match, and
 * that the recorded root still exists. On any mismatch they fall back to
 * shelling out to `aide anchor --json`, and ultimately to the TS walk
 * (findProjectRoot) when no binary is available. The anchor records
 * identity and topology only — never liveness.
 */

import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { homedir } from "os";
import { dirname, join } from "path";
import { findProjectRoot } from "./project-root.js";

export interface AnchorScope {
  root: string;
  realRoot: string;
  relation: "self" | "parent";
  evidence?: string;
}

export interface AnchorInfo {
  schemaVersion: number;
  resolverVersion: string;
  root: string;
  realRoot: string;
  hasMarker: boolean;
  source: string;
  provenance: { marker?: string; gitdirShape?: string };
  identity: { projectName: string; source: string };
  dbPath: string;
  socketPath: string;
  chain: AnchorScope[];
}

/** Session cache entry: the anchor plus the launch context it was resolved for. */
interface SessionAnchorEntry {
  launchCwd: string;
  savedAt: string;
  anchor: AnchorInfo;
}

export const ANCHOR_SCHEMA_VERSION = 1;

/** Session-keyed anchor caches older than this are swept. */
const ANCHOR_CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

const SESSION_ID_RE = /^[a-zA-Z0-9_-]{1,128}$/;

function anchorCacheDir(): string {
  return join(homedir(), ".aide", "anchors");
}

function sessionAnchorPath(sessionId: string): string | null {
  if (!SESSION_ID_RE.test(sessionId)) return null;
  return join(anchorCacheDir(), `${sessionId}.json`);
}

function isValidAnchor(a: unknown): a is AnchorInfo {
  if (!a || typeof a !== "object") return false;
  const info = a as AnchorInfo;
  return (
    info.schemaVersion === ANCHOR_SCHEMA_VERSION &&
    typeof info.root === "string" &&
    info.root.length > 0 &&
    typeof info.dbPath === "string" &&
    typeof info.socketPath === "string" &&
    !!info.identity &&
    typeof info.identity.projectName === "string" &&
    !!info.provenance &&
    typeof info.provenance === "object" &&
    Array.isArray(info.chain) &&
    info.chain.length > 0 &&
    info.chain[0].relation === "self" &&
    info.chain.every(
      (s) => typeof s.root === "string" && typeof s.relation === "string",
    )
  );
}

/** Atomic-enough write: temp file + rename in the same directory. */
function atomicWrite(path: string, content: string): void {
  mkdirSync(dirname(path), { recursive: true });
  const tmp = `${path}.tmp-${process.pid}`;
  writeFileSync(tmp, content);
  renameSync(tmp, path);
}

/**
 * Resolve the anchor by shelling out to the Go binary. Returns null when
 * the binary is missing or emits something unusable — callers fall back
 * to the TS walk.
 */
export function resolveAnchorViaBinary(
  binary: string,
  cwd: string,
): AnchorInfo | null {
  try {
    // --cwd is passed explicitly rather than relying on the child's
    // inherited working directory: it survives a deleted/renamed cwd and
    // is immune to getcwd/$PWD spelling differences. stderr is dropped —
    // pre-anchor binaries print "unknown command" there, and hook stderr
    // renders as an error in Claude Code.
    const out = execFileSync(binary, ["anchor", "--json", `--cwd=${cwd}`], {
      encoding: "utf-8",
      timeout: 5000,
      stdio: ["ignore", "pipe", "ignore"],
    });
    const parsed = JSON.parse(out) as unknown;
    return isValidAnchor(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

/**
 * Persist the anchor for a session: the session-keyed cache (primary hook
 * lookup path) and the project-local copy. Best-effort — persistence
 * failing must never break session start. Sweeps expired session caches
 * while it's here.
 */
export function writeSessionAnchor(
  sessionId: string,
  launchCwd: string,
  anchor: AnchorInfo,
): void {
  const entry: SessionAnchorEntry = {
    launchCwd,
    savedAt: new Date().toISOString(),
    anchor,
  };
  const json = JSON.stringify(entry, null, 2);

  const sessionPath = sessionAnchorPath(sessionId);
  if (sessionPath) {
    try {
      atomicWrite(sessionPath, json);
    } catch {
      /* best effort */
    }
  }

  // Project-local copy only when the project actually has a real .aide/
  // (never created here, and never written through a symlinked .aide or
  // state dir — a hostile repo could point one at a user-writable path).
  try {
    const aideDir = join(anchor.root, ".aide");
    const stateDir = join(aideDir, "state");
    if (
      lstatSync(aideDir).isDirectory() &&
      (!existsSync(stateDir) || lstatSync(stateDir).isDirectory())
    ) {
      atomicWrite(join(stateDir, "anchor.json"), json);
    }
  } catch {
    /* best effort */
  }

  sweepAnchorCache();
}

/**
 * Remove session-keyed caches past their TTL, plus orphaned atomic-write
 * temp files (crash between write and rename) older than an hour.
 * Best-effort.
 */
export function sweepAnchorCache(): void {
  try {
    const dir = anchorCacheDir();
    if (!existsSync(dir)) return;
    const cutoff = Date.now() - ANCHOR_CACHE_TTL_MS;
    const tmpCutoff = Date.now() - 60 * 60 * 1000;
    for (const name of readdirSync(dir)) {
      const p = join(dir, name);
      try {
        if (name.includes(".json.tmp-")) {
          if (statSync(p).mtimeMs < tmpCutoff) unlinkSync(p);
        } else if (name.endsWith(".json")) {
          if (statSync(p).mtimeMs < cutoff) unlinkSync(p);
        }
      } catch {
        /* races are fine */
      }
    }
  } catch {
    /* best effort */
  }
}

/**
 * Read the persisted anchor for a session, validating it still matches
 * this invocation: same launch cwd (a session's cwd is stable for its
 * lifetime — a different cwd means a different/stale session id) and the
 * recorded root still exists on disk.
 */
export function readSessionAnchor(
  sessionId: string,
  launchCwd: string,
): AnchorInfo | null {
  const sessionPath = sessionAnchorPath(sessionId);
  if (!sessionPath || !existsSync(sessionPath)) return null;
  try {
    const entry = JSON.parse(
      readFileSync(sessionPath, "utf-8"),
    ) as SessionAnchorEntry;
    if (!isValidAnchor(entry.anchor)) return null;
    if (entry.launchCwd !== launchCwd) return null;
    if (!existsSync(entry.anchor.root)) return null;
    return entry.anchor;
  } catch {
    return null;
  }
}

/**
 * Anchor lookup for hooks: session cache first (sub-ms file read), then
 * the binary (authoritative, ~10ms), then null — the caller's last resort
 * is the TS walk via findProjectRoot.
 *
 * AIDE_PROJECT_ROOT is deliberately NOT special-cased here: both the Go
 * resolver (recorded as source "env") and the TS walk honor it, so every
 * path below already agrees.
 */
export function getAnchor(opts: {
  sessionId?: string;
  cwd: string;
  binary?: string | null;
}): AnchorInfo | null {
  if (opts.sessionId) {
    const cached = readSessionAnchor(opts.sessionId, opts.cwd);
    if (cached) return cached;
  }
  if (opts.binary) {
    return resolveAnchorViaBinary(opts.binary, opts.cwd);
  }
  return null;
}

/**
 * Resolve just the root, with the full fallback ladder ending at the TS
 * walk. The common consumer shape for hooks that only need a root.
 */
export function getAnchoredRoot(opts: {
  sessionId?: string;
  cwd: string;
  binary?: string | null;
}): { root: string; hasMarker: boolean; anchor: AnchorInfo | null } {
  const anchor = getAnchor(opts);
  if (anchor) {
    return { root: anchor.root, hasMarker: anchor.hasMarker, anchor };
  }
  const walked = findProjectRoot(opts.cwd);
  return { root: walked.root, hasMarker: walked.hasMarker, anchor: null };
}
