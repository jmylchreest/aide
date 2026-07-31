/**
 * The version of the plugin package this code was loaded from.
 *
 * Anchored to `import.meta.url`, never to AIDE_PLUGIN_ROOT/CLAUDE_PLUGIN_ROOT:
 * when `bunx @jmylchreest/aide-plugin@latest install` runs inside an OpenCode
 * session, those env vars point at the *session's* (older) plugin cache, while
 * this module lives in the package the user just fetched. Pinning config to
 * the env var's version would re-pin users to the version they are trying to
 * leave.
 */

import { readFileSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";

/** Root of the package containing this module (src/lib/../.. → package root). */
export function getRunningPluginRoot(): string {
  return join(dirname(fileURLToPath(import.meta.url)), "..", "..");
}

/**
 * Version from that package's package.json, or null for an unversioned dev
 * checkout (`0.0.0`), which must never be written into a config pin.
 */
export function getRunningPluginVersion(): string | null {
  try {
    const pkg = JSON.parse(
      readFileSync(join(getRunningPluginRoot(), "package.json"), "utf-8"),
    ) as { version?: string };
    return pkg.version && pkg.version !== "0.0.0" ? pkg.version : null;
  } catch {
    return null;
  }
}
