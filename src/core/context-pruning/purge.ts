/**
 * Purge-errors strategy: replace large error outputs (stack traces, build
 * failures) with a compact summary.
 *
 * When a Bash command fails and produces a large error output, most of the
 * context is stack frames that aren't useful for the model. This strategy
 * trims the output to the first meaningful error lines.
 */

import type { PruneResult, PruneStrategy, ToolRecord } from "./types.js";

/** Minimum output size to trigger purging (2KB). */
const MIN_SIZE_FOR_PURGE = 2048;

/** Max lines to keep from an error output. */
const MAX_ERROR_LINES = 30;

/** Patterns that indicate an error output. */
const ERROR_PATTERNS = [
  /^error/im,
  /^ERR!/im,
  /exit code [1-9]/i,
  /FAILED/i,
  /panic:/i,
  /Traceback/i,
  /^Exception/im,
  /compilation failed/i,
  /build failed/i,
  /TypeError:/,
  /SyntaxError:/,
  /ReferenceError:/,
];

export class PurgeErrorsStrategy implements PruneStrategy {
  name = "purge" as const;

  apply(
    toolName: string,
    _args: Record<string, unknown>,
    output: string,
    _history: ToolRecord[],
  ): PruneResult {
    const normalized = toolName.toLowerCase();

    // Only apply to Bash output
    if (normalized !== "bash") {
      return { output, modified: false, bytesSaved: 0 };
    }

    // Only purge if output is large enough to matter
    if (output.length < MIN_SIZE_FOR_PURGE) {
      return { output, modified: false, bytesSaved: 0 };
    }

    // Check if output looks like an error
    const isError = ERROR_PATTERNS.some((p) => p.test(output));
    if (!isError) {
      return { output, modified: false, bytesSaved: 0 };
    }

    // Trim to first MAX_ERROR_LINES lines + a note
    const lines = output.split("\n");
    if (lines.length <= MAX_ERROR_LINES) {
      return { output, modified: false, bytesSaved: 0 };
    }

    const kept = lines.slice(0, MAX_ERROR_LINES).join("\n");
    const trimmedCount = lines.length - MAX_ERROR_LINES;
    const replacement =
      kept +
      `\n\n[aide:purge] ... ${trimmedCount} additional error lines trimmed. Re-run the command to see full output.`;

    return {
      output: replacement,
      modified: true,
      strategy: "purge",
      bytesSaved: output.length - replacement.length,
    };
  }
}
