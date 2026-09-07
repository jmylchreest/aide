import { dirname, join, resolve } from "path";

/**
 * Real filesystem, except project markers at or above the fixture temp root.
 * Resolver tests must see markers inside their fixtures, but not a developer's
 * /tmp/.aide, /tmp/.git, or a repository containing a custom TMPDIR. Mask only
 * those ancestor marker probes; file contents and all writes remain real.
 * Use a synchronous builtin lookup so this factory also works with bun test.
 */
export function isolatedTmpFs(
  tempRoot = process.getBuiltinModule("os").tmpdir(),
): typeof import("fs") {
  const fs = process.getBuiltinModule("fs");
  const hidden = new Set<string>();
  // macOS /var -> /private/var and other aliases can give fixtures either
  // spelling, depending on whether the suite calls realpathSync.
  for (const root of [resolve(tempRoot), fs.realpathSync(tempRoot)]) {
    let dir = root;
    for (;;) {
      for (const marker of [".aide", ".git", ".hg", ".svn", ".bzr", ".fossil"]) {
        hidden.add(join(dir, marker));
      }
      const parent = dirname(dir);
      if (parent === dir) break;
      dir = parent;
    }
  }
  return {
    ...fs,
    existsSync: (path) => {
      if (typeof path === "string" && hidden.has(resolve(path))) return false;
      return fs.existsSync(path);
    },
  };
}
