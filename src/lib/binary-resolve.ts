/**
 * Locate the aide Go binary that npm installed as an optionalDependency.
 *
 * The per-arch packages (`@jmylchreest/aide-binary-<platform>-<arch>`) are
 * optionalDependencies of `@jmylchreest/aide-plugin`. npm, bun and arborist
 * all *hoist* dependencies to the install root, so the binary lands next to
 * the plugin, not inside it:
 *
 *   node_modules/@jmylchreest/aide-plugin/              <- plugin root
 *   node_modules/@jmylchreest/aide-binary-linux-amd64/bin/aide
 *
 * Looking under `<plugin-root>/node_modules/` therefore misses in every
 * normal install; that layout only appears when a version conflict forces
 * npm to nest. Resolution has to follow node's own algorithm instead, which
 * is what esbuild does for its per-arch binaries.
 */

import { createRequire } from "module";
import { existsSync } from "fs";
import { dirname, join, resolve, sep } from "path";

export const BINARY_PACKAGE_PREFIX = "@jmylchreest/aide-binary-";

/** platform-arch (node names) → per-arch npm package suffix (Go names). */
const ARCH_PACKAGE_SUFFIX: Record<string, string> = {
  "linux-x64": "linux-amd64",
  "linux-arm64": "linux-arm64",
  "darwin-x64": "darwin-amd64",
  "darwin-arm64": "darwin-arm64",
  "win32-x64": "windows-amd64",
  "win32-arm64": "windows-arm64",
};

export interface PlatformTarget {
  platform?: NodeJS.Platform | string;
  arch?: string;
}

/** Per-arch package suffix for a platform, or null if we ship no binary for it. */
export function getArchPackageSuffix(target: PlatformTarget = {}): string | null {
  const platform = target.platform ?? process.platform;
  const arch = target.arch ?? process.arch;
  return ARCH_PACKAGE_SUFFIX[`${platform}-${arch}`] ?? null;
}

/** Full npm package name holding the binary for a platform. */
export function getBinaryPackageName(target: PlatformTarget = {}): string | null {
  const suffix = getArchPackageSuffix(target);
  return suffix ? `${BINARY_PACKAGE_PREFIX}${suffix}` : null;
}

/** Binary file name — Windows keeps the .exe suffix. */
export function getBinaryFileName(target: PlatformTarget = {}): string {
  const platform = target.platform ?? process.platform;
  return platform === "win32" ? "aide.exe" : "aide";
}

/**
 * Every path where the bundled binary could plausibly live, in priority
 * order, excluding node's own resolution (which `resolveBundledBinary`
 * tries first). Exported for tests and for diagnostics in the wrapper log.
 */
export function bundledBinaryCandidates(
  pluginRoot: string,
  target: PlatformTarget = {},
): string[] {
  const pkg = getBinaryPackageName(target);
  if (!pkg) return [];
  const file = getBinaryFileName(target);
  const subPath = join(...pkg.split("/"), "bin", file);

  const candidates: string[] = [];
  let dir = resolve(pluginRoot);
  // Walk up from the plugin root. At each level check `<dir>/node_modules/<pkg>`
  // and — when the level *is* a node_modules dir — `<dir>/<pkg>` directly,
  // which is where hoisting actually puts it.
  for (;;) {
    candidates.push(join(dir, "node_modules", subPath));
    if (dir.endsWith(`${sep}node_modules`)) {
      candidates.push(join(dir, subPath));
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return candidates;
}

export interface ResolveOptions extends PlatformTarget {
  /** Plugin package root, used for the filesystem walk-up fallbacks. */
  pluginRoot?: string;
  /** Module URL/path node resolution starts from. Defaults to this module. */
  from?: string;
  /** Override for tests. */
  exists?: (path: string) => boolean;
}

/**
 * Path to the npm-provided binary for the current platform, or null when
 * it isn't installed (`--omit=optional`, unsupported platform, Claude Code's
 * marketplace layout, or a dev checkout without the packages).
 */
export function resolveBundledBinary(options: ResolveOptions = {}): string | null {
  const pkg = getBinaryPackageName(options);
  if (!pkg) return null;
  const fileExists = options.exists ?? existsSync;
  const file = getBinaryFileName(options);

  // Node resolution first: follows the real module graph, so it finds the
  // hoisted package wherever the package manager actually put it.
  const isCurrentPlatform =
    (options.platform ?? process.platform) === process.platform &&
    (options.arch ?? process.arch) === process.arch;
  if (isCurrentPlatform && !options.exists) {
    try {
      const req = createRequire(options.from ?? import.meta.url);
      const resolved = req.resolve(`${pkg}/bin/${file}`);
      if (fileExists(resolved)) return resolved;
    } catch {
      // Not resolvable — fall through to the filesystem walk.
    }
  }

  if (options.pluginRoot) {
    for (const candidate of bundledBinaryCandidates(options.pluginRoot, options)) {
      if (fileExists(candidate)) return candidate;
    }
  }

  return null;
}
