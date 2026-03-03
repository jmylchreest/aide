/**
 * Supersede strategy: when a Write or Edit completes, mark prior Read outputs
 * of the same file as stale so the model doesn't rely on outdated content.
 *
 * This doesn't replace the current tool output — it annotates previous Read
 * records so that if they're re-read (dedup check), the stale content is flagged.
 *
 * For now, this strategy only adds a note to the current Write/Edit output
 * reminding the model that prior reads of this file are now stale.
 */

import type { PruneResult, PruneStrategy, ToolRecord } from "./types.js";

/** Tools that supersede prior reads. */
const WRITE_TOOLS = new Set(["write", "edit"]);

/** Extract the file path from tool args. */
function getFilePath(args: Record<string, unknown>): string | undefined {
  return (
    (args.filePath as string) ??
    (args.file_path as string) ??
    (args.path as string) ??
    undefined
  );
}

export class SupersedeStrategy implements PruneStrategy {
  name = "supersede" as const;

  apply(
    toolName: string,
    args: Record<string, unknown>,
    output: string,
    history: ToolRecord[],
  ): PruneResult {
    const normalized = toolName.toLowerCase();

    if (!WRITE_TOOLS.has(normalized)) {
      return { output, modified: false, bytesSaved: 0 };
    }

    const filePath = getFilePath(args);
    if (!filePath) {
      return { output, modified: false, bytesSaved: 0 };
    }

    // Check if there are prior Read calls for this same file
    const priorReads = history.filter((rec) => {
      if (rec.toolName.toLowerCase() !== "read") return false;
      const recPath = getFilePath(rec.args);
      return recPath === filePath;
    });

    if (priorReads.length === 0) {
      return { output, modified: false, bytesSaved: 0 };
    }

    // Annotate: prior reads of this file are now stale
    const note = `\n[aide:supersede] Note: ${priorReads.length} prior Read(s) of "${filePath}" are now stale after this ${toolName}. Re-read if you need current content.`;
    return {
      output: output + note,
      modified: true,
      strategy: "supersede",
      bytesSaved: 0, // We're adding, not saving bytes
    };
  }
}
