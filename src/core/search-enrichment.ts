/**
 * Search Enrichment — platform-agnostic core logic.
 *
 * Enriches Grep tool calls with structural context from the code index.
 * When an agent greps for a symbol name, this appends metadata about
 * matching symbol definitions (file, kind, ref count) so the agent
 * knows where the symbol is defined and how widely it's used — without
 * making additional tool calls.
 *
 * Behaviour:
 *   - Triggers on Grep tool calls where the pattern looks like a symbol name
 *   - Calls `aide code search <pattern> --json --limit=5` to find definitions
 *   - For each match, calls `aide code references <name> --json --limit=0` for ref count
 *   - Returns a compact enrichment string (~50-150 tokens)
 *   - Never blocks — purely additive context
 *
 * Gated behind AIDE_CODE_WATCH=1 (requires code index to be populated).
 *
 * Used by both Claude Code hooks (PreToolUse) and OpenCode plugin.
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";

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
  if (process.env.AIDE_CODE_WATCH !== "1") {
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

  // Skip patterns with spaces (likely searching for phrases, not symbols)
  if (pattern.includes(" ")) {
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

  for (const sym of symbols) {
    const refCount = countReferences(binary, cwd, sym.name);
    const refs = refCount > 0 ? `, ${refCount} refs` : ", 0 refs";
    lines.push(`  ${sym.kind} ${sym.name} — ${sym.file}:${sym.start}${refs}`);
  }

  if (symbols.length > 0) {
    lines.push(
      `Use code_read_symbol for source, code_references for call sites.`,
    );
  }

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

    return parsed.map(
      (s: Record<string, unknown>): SymbolHit => ({
        name: (s.name as string) || "",
        kind: (s.kind as string) || "",
        file: (s.file as string) || "",
        start: (s.start as number) || 0,
        end: (s.end as number) || 0,
        signature: (s.signature as string) || "",
        lang: (s.lang as string) || "",
      }),
    );
  } catch (err) {
    debug(SOURCE, `Symbol search failed: ${err}`);
    return [];
  }
}

/**
 * Count references to a symbol name in the code index.
 * Returns the count, or 0 on error.
 */
function countReferences(
  binary: string,
  cwd: string,
  symbolName: string,
): number {
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
    if (!trimmed || trimmed.startsWith("No references")) {
      return 0;
    }

    const parsed = JSON.parse(trimmed);
    return Array.isArray(parsed) ? parsed.length : 0;
  } catch {
    return 0;
  }
}
