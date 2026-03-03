/**
 * Context pruning types.
 *
 * These types define the contract between the platform-agnostic pruning logic
 * and the platform-specific adapters (OpenCode hooks, Claude Code PostToolUse).
 */

/** A recorded tool use with its output, used for dedup tracking. */
export interface ToolRecord {
  /** Unique ID for this tool invocation (callID from host). */
  callId: string;
  /** Tool name (e.g. "Read", "Glob", "mcp__aide__code_search"). */
  toolName: string;
  /** Key arguments that define the "identity" of this call. */
  args: Record<string, unknown>;
  /** The original tool output (before any pruning). */
  originalOutput: string;
  /** The pruned output (after dedup/supersede applied), or null if unchanged. */
  prunedOutput: string | null;
  /** Timestamp of the tool invocation. */
  timestamp: number;
  /** File mtime at time of call (for Read dedup safety). */
  fileMtime?: number;
}

/** Result of applying pruning strategies to a tool output. */
export interface PruneResult {
  /** The (possibly modified) output string. */
  output: string;
  /** Whether the output was modified. */
  modified: boolean;
  /** Which strategy modified it, if any. */
  strategy?: "dedup" | "supersede" | "purge";
  /** Bytes saved (original - pruned). */
  bytesSaved: number;
}

/** Strategy interface — each strategy gets the current call + history. */
export interface PruneStrategy {
  name: string;
  /**
   * Evaluate whether this strategy should prune the given tool output.
   * Returns a PruneResult with the potentially modified output.
   */
  apply(
    toolName: string,
    args: Record<string, unknown>,
    output: string,
    history: ToolRecord[],
  ): PruneResult;
}

/** Running stats for the context pressure signal. */
export interface PruningStats {
  /** Total tool invocations tracked. */
  totalCalls: number;
  /** Total tool invocations that were pruned. */
  prunedCalls: number;
  /** Total bytes saved by pruning. */
  totalBytesSaved: number;
  /** Estimated total bytes of tool output in context. */
  estimatedContextBytes: number;
}
