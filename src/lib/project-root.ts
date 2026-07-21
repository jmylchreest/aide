/**
 * Project root resolution for the AIDE plugin.
 *
 * Mirrors the Go binary's findProjectRoot() in aide/cmd/aide/main.go so that
 * the TypeScript hook layer and the Go binary always agree on where `.aide/`
 * lives. Without this, the hook would plant a sibling `.aide/` in whatever
 * subdirectory `claude` was launched from, while the Go binary would walk up
 * and use the real one at the repo root.
 *
 * Resolution order, matching main.go:findProjectRoot:
 *   1. AIDE_PROJECT_ROOT env override (must be an existing directory).
 *   2. Walk the full ancestry from cwd to /, collecting candidates, then
 *      prefer:
 *        a. Closest ancestor with a VCS marker (.git/.hg/.svn/.bzr/.fossil).
 *           A VCS boundary — including a submodule, which is its own
 *           repository — is the project boundary: starting inside a
 *           submodule anchors the submodule, starting in the superproject
 *           anchors the superproject. Worktree .git files resolve to the
 *           main repo root (worktrees share one store); submodule .git
 *           files anchor the submodule directory itself.
 *        b. Closest ancestor with .aide/ only — standalone projects with
 *           no VCS.
 *      ~/.aide/ is skipped as a project marker unless cwd is $HOME.
 *   3. No marker found: return { root: cwd, hasMarker: false }.
 *
 * A stray child .aide/ (created by an accidental CLI invocation from a
 * subdir) cannot shadow the real project root: it has no VCS marker, so
 * any real repository above it takes priority.
 */

import { basename, dirname, isAbsolute, join, resolve } from "path";
import { existsSync, readFileSync, statSync } from "fs";
import { homedir } from "os";

export interface ProjectRootResult {
  root: string;
  hasMarker: boolean;
}

/**
 * Resolve the AIDE project root for a given cwd.
 *
 * `hasMarker` is true when an actual `.aide/` or `.git/` marker was found
 * (or when AIDE_PROJECT_ROOT is set to an existing directory). When false,
 * `root` is just the input cwd — callers should decide whether to fall
 * back to it (e.g. via the `requireGit` config) or refuse to bootstrap.
 */
export function findProjectRoot(cwd: string): ProjectRootResult {
  const override = process.env.AIDE_PROJECT_ROOT;
  if (override) {
    try {
      const abs = resolve(override);
      const stat = statSync(abs);
      if (stat.isDirectory()) {
        // An override pointing at an unmarked directory is almost always
        // stale (repo moved, leaked env from another project) — require a
        // marker, or AIDE_FORCE_INIT to say "yes, really". Mirrors the Go
        // resolver's validation.
        if (overrideAllowed(abs)) {
          return { root: abs, hasMarker: true };
        }
        process.stderr.write(
          `aide: AIDE_PROJECT_ROOT=${JSON.stringify(override)} has no .aide/ or VCS marker (set AIDE_FORCE_INIT=1 to use it anyway); falling back to walk-up\n`,
        );
      } else {
        process.stderr.write(
          `aide: AIDE_PROJECT_ROOT=${JSON.stringify(override)} is not a directory; falling back to walk-up\n`,
        );
      }
    } catch {
      process.stderr.write(
        `aide: AIDE_PROJECT_ROOT=${JSON.stringify(override)} is not a directory; falling back to walk-up\n`,
      );
    }
  }

  const startCwd = resolve(cwd);
  const home = homedir();

  interface Candidate {
    dir: string;
    hasAide: boolean;
    hasVCS: boolean;
    vcsResolved: string;
  }
  const path: Candidate[] = [];

  let dir = startCwd;
  for (;;) {
    const cand: Candidate = { dir, hasAide: false, hasVCS: false, vcsResolved: "" };

    if (existsSync(join(dir, ".aide"))) {
      // Skip ~/.aide/ unless cwd is $HOME itself.
      const isHomeAide = home && dir === home && startCwd !== home;
      if (!isHomeAide) cand.hasAide = true;
    }

    const vcs = vcsMarkerAt(dir);
    if (vcs.ok) {
      cand.hasVCS = true;
      cand.vcsResolved = vcs.resolved || dir;
    }

    if (cand.hasAide || cand.hasVCS) path.push(cand);

    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }

  for (const c of path) {
    if (c.hasVCS) return { root: c.vcsResolved, hasMarker: true };
  }
  for (const c of path) {
    if (c.hasAide) return { root: c.dir, hasMarker: true };
  }
  return { root: startCwd, hasMarker: false };
}

/**
 * Whether an AIDE_PROJECT_ROOT override may anchor dir: it carries a
 * project marker, or AIDE_FORCE_INIT is set.
 */
function overrideAllowed(dir: string): boolean {
  const force = process.env.AIDE_FORCE_INIT;
  if (force === "1" || force?.toLowerCase() === "true") return true;
  if (existsSync(join(dir, ".aide"))) return true;
  for (const marker of [".git", ".hg", ".svn", ".bzr", ".fossil"]) {
    if (existsSync(join(dir, marker))) return true;
  }
  return false;
}

function vcsMarkerAt(dir: string): { ok: boolean; resolved: string } {
  const gitPath = join(dir, ".git");
  if (existsSync(gitPath)) {
    try {
      const st = statSync(gitPath);
      if (st.isDirectory()) return { ok: true, resolved: dir };
      if (st.isFile()) {
        const mainRoot = resolveWorktreeGitFile(gitPath);
        return { ok: true, resolved: mainRoot || dir };
      }
      return { ok: true, resolved: dir };
    } catch {
      return { ok: true, resolved: dir };
    }
  }
  for (const marker of [".hg", ".svn", ".bzr", ".fossil"]) {
    if (existsSync(join(dir, marker))) return { ok: true, resolved: dir };
  }
  return { ok: false, resolved: "" };
}

/**
 * Walk up from `startDir` looking for `.aide/` or `.git/` markers.
 * Returns the resolved root directory, or null when nothing is found.
 *
 * Thin wrapper around findProjectRoot for callers that want a nullable
 * result rather than the {root,hasMarker} shape. No in-repo production
 * callers remain (the OpenCode plugin now uses findProjectRoot directly);
 * kept as published API for external consumers. Prefer findProjectRoot
 * in new code.
 */
export function walkUpForProjectRoot(startDir: string): string | null {
  const { root, hasMarker } = findProjectRoot(startDir);
  return hasMarker ? root : null;
}

/**
 * Reports whether a resolved gitdir path points into a superproject's
 * modules tree, i.e. the .git file belongs to a submodule checkout rather
 * than a linked worktree. Matches both plain (`.git/modules/<p>`) and
 * worktree-hosted (`.git/worktrees/<wt>/modules/<p>`) shapes — the latter
 * was previously misclassified as a worktree, anchoring the superproject
 * instead of the submodule. Mirrors aide/cmd/aide/main.go.
 */
function isSubmoduleGitdir(gitdir: string): boolean {
  const parts = gitdir.split(/[\\/]+/);
  for (let i = 0; i < parts.length - 1; i++) {
    if (parts[i] !== ".git") continue;
    let j = i + 1;
    if (parts[j] === "worktrees" && j + 2 < parts.length) j += 2;
    if (j < parts.length && parts[j] === "modules") return true;
  }
  return false;
}

/**
 * Read a .git worktree file ("gitdir: <path>") and return the main repo root.
 *
 * Mirrors aide/cmd/aide/main.go:resolveWorktreeRoot(). The file's gitdir
 * normally points at "<main>/.git/worktrees/<name>"; we walk up that path
 * until we find a component named ".git" and return its parent.
 * Submodule .git files (gitdir under .git/modules/) return null so the
 * submodule directory itself becomes the root.
 */
export function resolveWorktreeGitFile(gitFilePath: string): string | null {
  try {
    const content = readFileSync(gitFilePath, "utf-8").trim();
    if (!content.startsWith("gitdir:")) return null;

    let gitdir = content.slice("gitdir:".length).trim();
    if (!isAbsolute(gitdir)) {
      gitdir = resolve(dirname(gitFilePath), gitdir);
    }

    if (isSubmoduleGitdir(gitdir)) return null;

    // A dangling gitdir (repo moved or deleted) must not resurrect state
    // at the dead path — fall back to the checkout directory itself.
    if (!existsSync(gitdir)) return null;

    let candidate = gitdir;
    for (;;) {
      const parent = dirname(candidate);
      if (parent === candidate) break;
      if (basename(candidate) === ".git") {
        return parent;
      }
      candidate = parent;
    }
    return null;
  } catch {
    return null;
  }
}
