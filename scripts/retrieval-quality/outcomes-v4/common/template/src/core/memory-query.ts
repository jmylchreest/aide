/**
 * Memory query helper — platform-agnostic.
 *
 * Thin wrapper around `aide memory list … --format=json` so callers don't each
 * re-implement the spawn + parse. Returns the parsed entries; filtering,
 * sorting, and mapping are left to the caller (their needs differ — session
 * partials filter by tag, the resume path sorts by createdAt).
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";

const SOURCE = "memory-query";

/** Shape of an entry from `aide memory list --format=json`. */
export interface MemoryEntry {
  id: string;
  category?: string;
  content: string;
  tags?: string[];
  createdAt?: string;
}

export interface ListMemoriesOptions {
  /** Comma-separated tag filter passed to `--tags=`. */
  tags?: string;
  /** Include memories tagged `forget` / `partial` (passes `--all`). */
  all?: boolean;
  /** Max rows to return (passes `--limit=`). Defaults to the CLI default. */
  limit?: number;
}

/**
 * Run `aide memory list` with the given filters and return parsed entries.
 * Returns an empty array on any failure (missing binary, parse error, etc.).
 */
export function listMemoriesJson(
  binary: string,
  cwd: string,
  opts: ListMemoriesOptions = {},
): MemoryEntry[] {
  try {
    const args = ["memory", "list", "--format=json"];
    if (opts.tags) args.push(`--tags=${opts.tags}`);
    if (opts.all) args.push("--all");
    if (opts.limit !== undefined) args.push(`--limit=${opts.limit}`);

    const output = execFileSync(binary, args, {
      cwd,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 5000,
    }).trim();

    if (!output || output === "[]" || output === "null") return [];

    const parsed: MemoryEntry[] = JSON.parse(output);
    return Array.isArray(parsed) ? parsed : [];
  } catch (err) {
    debug(SOURCE, `memory list failed (non-fatal): ${err}`);
    return [];
  }
}
