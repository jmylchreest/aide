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
 *        a. Closest ancestor with BOTH .aide/ and a VCS marker
 *           (.git/.hg/.svn/.bzr/.fossil) — handles the common case where
 *           the canonical root sits at the git repo root.
 *        b. Closest ancestor with a VCS marker only — .aide/ will be
 *           created there if needed.
 *        c. Closest ancestor with .aide/ only — standalone projects with
 *           no VCS.
 *      ~/.aide/ is skipped as a project marker unless cwd is $HOME.
 *   3. No marker found: return { root: cwd, hasMarker: false }.
 *
 * The "both markers wins" priority is what stops a stray child .aide/ from
 * shadowing the real project root: a sibling .aide/ created by an
 * accidental CLI invocation lives in a subdir with no .git/, so the walk
 * keeps going until it finds the parent that has both.
 */

import { basename, dirname, join, resolve } from "path";
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
        return { root: abs, hasMarker: true };
      }
    } catch {
      // Fall through to the walk-up.
    }
    process.stderr.write(
      `aide: AIDE_PROJECT_ROOT=${JSON.stringify(override)} is not a directory; falling back to walk-up\n`,
    );
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
    if (c.hasAide && c.hasVCS) return { root: c.vcsResolved, hasMarker: true };
  }
  for (const c of path) {
    if (c.hasVCS) return { root: c.vcsResolved, hasMarker: true };
  }
  for (const c of path) {
    if (c.hasAide) return { root: c.dir, hasMarker: true };
  }
  return { root: startCwd, hasMarker: false };
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
 * result rather than the {root,hasMarker} shape (e.g. the OpenCode plugin
 * which has its own fallback chain).
 */
export function walkUpForProjectRoot(startDir: string): string | null {
  const { root, hasMarker } = findProjectRoot(startDir);
  return hasMarker ? root : null;
}

/**
 * Read a .git worktree file ("gitdir: <path>") and return the main repo root.
 *
 * Mirrors aide/cmd/aide/main.go:resolveWorktreeRoot(). The file's gitdir
 * normally points at "<main>/.git/worktrees/<name>"; we walk up that path
 * until we find a component named ".git" and return its parent.
 */
export function resolveWorktreeGitFile(gitFilePath: string): string | null {
  try {
    const content = readFileSync(gitFilePath, "utf-8").trim();
    if (!content.startsWith("gitdir:")) return null;

    let gitdir = content.slice("gitdir:".length).trim();
    if (!gitdir.startsWith("/")) {
      gitdir = resolve(dirname(gitFilePath), gitdir);
    }

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
