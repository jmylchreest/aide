/**
 * Search Enrichment — platform-agnostic core logic.
 *
 * Enriches Grep tool calls with structural context from the code index.
 * When an agent greps for a symbol name, this appends metadata about
 * matching symbol definitions (file, kind, ref count) so the agent
 * gets definition and reference candidates without additional agent tool calls.
 * The synchronous index lookups still incur latency and local work.
 *
 * Behaviour:
 *   - Triggers on Grep tool calls where the pattern looks like a symbol name
 *   - Calls `aide code search <pattern> --json --limit=5` to find definitions
 *   - For each match, requests up to 100 indexed references by name
 *   - Returns a bounded list of matches and retrieval guidance
 *   - Never denies the request; up to six synchronous lookups can each wait 3s
 *
 * Gated on code.watch (default on; requires code index to be populated).
 *
 * Used by both Claude Code hooks (PreToolUse) and OpenCode plugin.
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";
import { codeWatchEnabled } from "../lib/hook-utils.js";

const SOURCE = "search-enrichment";

/** Minimum pattern length to attempt enrichment (avoid single-char patterns) */
const MIN_PATTERN_LENGTH = 3;

/** Maximum time to wait for aide binary responses */
const EXEC_TIMEOUT_MS = 3000;

/**
 * Patterns that are clearly regex, not symbol names.
 * Skip enrichment for these — the code index won't have useful matches.
 */
const REGEX_INDICATORS = /[.*+?^${}()|[\]\\]/;

export interface SearchEnrichmentResult {
  /** Whether to inject enrichment context */
  shouldEnrich: boolean;
  /** Enrichment context to append */
  enrichment?: string;
}

interface SymbolHit {
  name: string;
  kind: string;
  file: string;
  start: number;
  end: number;
  signature: string;
  lang: string;
}

/**
 * Check whether a Grep tool call should receive code index enrichment.
 *
 * Extracts the search pattern, looks it up in the code index, and returns
 * a compact summary of matching symbol definitions with ref counts.
 */
export function checkSearchEnrichment(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
  binary: string | null,
): SearchEnrichmentResult {
  const normalizedTool = toolName.toLowerCase();

  // Only enrich Grep tool calls
  if (normalizedTool !== "grep") {
    return { shouldEnrich: false };
  }

  // Require code watcher to be enabled (implies code index exists)
  if (!codeWatchEnabled(cwd)) {
    return { shouldEnrich: false };
  }

  if (!binary) {
    return { shouldEnrich: false };
  }

  // Extract the search pattern
  const pattern =
    (toolInput.pattern as string) ||
    (toolInput.query as string) ||
    (toolInput.search as string);

  if (!pattern || pattern.length < MIN_PATTERN_LENGTH) {
    return { shouldEnrich: false };
  }

  // Skip patterns that are clearly regex (not symbol names)
  if (REGEX_INDICATORS.test(pattern)) {
    return { shouldEnrich: false };
  }

  // Phrases, import paths and quoted literals belong in text search.
  if (/[\s/"'`]/.test(pattern)) {
    return { shouldEnrich: false };
  }

  // Look up matching symbols in the code index
  const symbols = searchSymbols(binary, cwd, pattern);
  if (symbols.length === 0) {
    return { shouldEnrich: false };
  }

  // Build compact enrichment string
  const lines: string[] = [];
  lines.push(`[aide:code-index] Symbol definitions matching "${pattern}":`);

  const referenceCounts = new Map<string, number | null>();
  for (const sym of symbols) {
    if (!referenceCounts.has(sym.name)) {
      referenceCounts.set(sym.name, countReferences(binary, cwd, sym.name));
    }
    const refCount = referenceCounts.get(sym.name)!;
    const refs =
      refCount === null
        ? ", refs unavailable"
        : `, ${refCount >= 100 ? "100+" : refCount} indexed refs by name`;
    lines.push(`  ${sym.kind} ${sym.name} — ${sym.file}:${sym.start}${refs}`);
  }

  lines.push(
    `Use the file/name above directly; code_search is only needed for other definitions. ` +
      `For callers or change impact, use code_references. ` +
      `For source, batch code_read_symbol with symbols (up to 10 names); add file to disambiguate. ` +
      `Reuse current bodies already in context. Keep Grep for literals and imports. Indexed matches are candidates; verify current source.`,
  );

  const enrichment = lines.join("\n");
  debug(SOURCE, `Enriching grep for "${pattern}": ${symbols.length} symbols`);

  return { shouldEnrich: true, enrichment };
}

/**
 * Search the code index for symbol definitions matching a pattern.
 */
function searchSymbols(
  binary: string,
  cwd: string,
  pattern: string,
): SymbolHit[] {
  try {
    const output = execFileSync(
      binary,
      ["code", "search", pattern, "--json", "--limit=5"],
      {
        cwd,
        encoding: "utf-8",
        timeout: EXEC_TIMEOUT_MS,
        stdio: ["pipe", "pipe", "pipe"],
      },
    );

    const trimmed = output.trim();
    if (!trimmed || trimmed.startsWith("No matching")) {
      return [];
    }

    const parsed = JSON.parse(trimmed);
    if (!Array.isArray(parsed)) return [];

    return parsed.map((s: Record<string, unknown>): SymbolHit => ({
      name: (s.name as string) || "",
      kind: (s.kind as string) || "",
      file: (s.file as string) || "",
      start: (s.start as number) || 0,
      end: (s.end as number) || 0,
      signature: (s.signature as string) || "",
      lang: (s.lang as string) || "",
    }));
  } catch (err) {
    debug(SOURCE, `Symbol search failed: ${err}`);
    return [];
  }
}

/**
 * Count references to a symbol name in the code index.
 * Returns the bounded result count, or null when the lookup is unavailable.
 */
function countReferences(
  binary: string,
  cwd: string,
  symbolName: string,
): number | null {
  try {
    const output = execFileSync(
      binary,
      ["code", "references", symbolName, "--json", "--limit=100"],
      {
        cwd,
        encoding: "utf-8",
        timeout: EXEC_TIMEOUT_MS,
        stdio: ["pipe", "pipe", "pipe"],
      },
    );

    const trimmed = output.trim();
    if (trimmed.startsWith("No references")) {
      return 0;
    }

    if (!trimmed) return null;

    const parsed = JSON.parse(trimmed);
    return Array.isArray(parsed) ? parsed.length : null;
  } catch {
    return null;
  }
}
