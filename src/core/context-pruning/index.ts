/**
 * Context Pruning — reduces context/token usage by deduplicating,
 * superseding, and purging tool outputs.
 *
 * Platform adapters integrate via the ContextPruningTracker:
 * - OpenCode: tool.execute.after hook modifies output.output
 * - Claude Code: PostToolUse hook returns updatedMCPToolOutput
 */

export { ContextPruningTracker } from "./tracker.js";
export { DedupStrategy } from "./dedup.js";
export { SupersedeStrategy } from "./supersede.js";
export { PurgeErrorsStrategy } from "./purge.js";
export type {
  ToolRecord,
  PruneResult,
  PruneStrategy,
  PruningStats,
} from "./types.js";
