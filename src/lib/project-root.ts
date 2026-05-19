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
 *   2. Walk up from cwd. At each level:
 *      a. .aide/ — return this dir. Skip ~/.aide/ unless cwd === $HOME.
 *      b. .git/ directory — return this dir.
 *      c. .git/ file (worktree pointer) — resolve to the main repo root.
 *   3. No marker found: return { root: cwd, hasMarker: false }.
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

  let dir = startCwd;
  for (;;) {
    const aidePath = join(dir, ".aide");
    if (existsSync(aidePath)) {
      // Skip ~/.aide/ unless cwd is $HOME itself. ~/.aide/ is the global
      // config dir, not a project marker.
      if (!(home && dir === home && startCwd !== home)) {
        return { root: dir, hasMarker: true };
      }
    }

    const gitPath = join(dir, ".git");
    if (existsSync(gitPath)) {
      try {
        const stat = statSync(gitPath);
        if (stat.isDirectory()) {
          return { root: dir, hasMarker: true };
        }
        if (stat.isFile()) {
          const mainRoot = resolveWorktreeGitFile(gitPath);
          if (mainRoot) {
            return { root: mainRoot, hasMarker: true };
          }
          return { root: dir, hasMarker: true };
        }
      } catch {
        return { root: dir, hasMarker: true };
      }
    }

    const parent = dirname(dir);
    if (parent === dir) {
      return { root: startCwd, hasMarker: false };
    }
    dir = parent;
  }
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
