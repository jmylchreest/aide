/**
 * Context Pruning Tracker
 *
 * Orchestrates pruning strategies against tool outputs to reduce context usage.
 * Platform adapters (OpenCode hooks, Claude Code PostToolUse) call into this
 * tracker after each tool completes.
 *
 * The tracker maintains a history of tool invocations per session and applies
 * strategies in order: dedup → supersede → purge-errors.
 */

import type {
  PruneResult,
  PruneStrategy,
  PruningStats,
  ToolRecord,
} from "./types.js";
import { DedupStrategy } from "./dedup.js";
import { SupersedeStrategy } from "./supersede.js";
import { PurgeErrorsStrategy } from "./purge.js";

export class ContextPruningTracker {
  private history: ToolRecord[] = [];
  private strategies: PruneStrategy[];
  private stats: PruningStats = {
    totalCalls: 0,
    prunedCalls: 0,
    totalBytesSaved: 0,
    estimatedContextBytes: 0,
  };

  /** Max history entries to keep (prevents unbounded growth). */
  private maxHistory: number;

  constructor(cwd?: string, maxHistory = 200) {
    this.maxHistory = maxHistory;
    this.strategies = [
      new DedupStrategy(cwd),
      new SupersedeStrategy(),
      new PurgeErrorsStrategy(),
    ];
  }

  /**
   * Load existing history (for process-per-invocation environments like Claude Code).
   * Merges with any existing in-memory history.
   */
  loadHistory(records: ToolRecord[]): void {
    this.history = records.slice(-this.maxHistory);
    // Recompute stats from loaded history
    this.stats.totalCalls = this.history.length;
    this.stats.prunedCalls = this.history.filter(
      (r) => r.prunedOutput !== null,
    ).length;
    this.stats.estimatedContextBytes = this.history.reduce(
      (sum, r) => sum + (r.prunedOutput ?? r.originalOutput).length,
      0,
    );
    this.stats.totalBytesSaved = this.history.reduce(
      (sum, r) =>
        sum +
        (r.prunedOutput !== null
          ? r.originalOutput.length - r.prunedOutput.length
          : 0),
      0,
    );
  }

  /** Get the current history (for persistence). */
  getHistory(): ToolRecord[] {
    return [...this.history];
  }

  /**
   * Process a tool output through all pruning strategies.
   * Returns the (possibly modified) output and metadata.
   */
  process(
    callId: string,
    toolName: string,
    args: Record<string, unknown>,
    output: string,
  ): PruneResult {
    this.stats.totalCalls++;

    // Apply strategies in order — first match wins
    let result: PruneResult = { output, modified: false, bytesSaved: 0 };

    for (const strategy of this.strategies) {
      result = strategy.apply(toolName, args, output, this.history);
      if (result.modified) {
        this.stats.prunedCalls++;
        this.stats.totalBytesSaved += result.bytesSaved;
        break;
      }
    }

    // Track context size
    this.stats.estimatedContextBytes += result.output.length;

    // Record this call in history
    const record: ToolRecord = {
      callId,
      toolName,
      args,
      originalOutput: output,
      prunedOutput: result.modified ? result.output : null,
      timestamp: Date.now(),
    };

    // For Read tools, record file mtime for dedup safety
    if (toolName.toLowerCase() === "read") {
      const filePath =
        (args.filePath as string) ??
        (args.file_path as string) ??
        (args.path as string);
      if (filePath) {
        try {
          const { statSync } = require("fs");
          const { resolve, isAbsolute } = require("path");
          const resolved = isAbsolute(filePath)
            ? filePath
            : resolve(process.cwd(), filePath);
          record.fileMtime = statSync(resolved).mtimeMs;
        } catch {
          // File may not exist (e.g., directory read)
        }
      }
    }

    this.history.push(record);

    // Trim history if needed
    if (this.history.length > this.maxHistory) {
      this.history = this.history.slice(-this.maxHistory);
    }

    return result;
  }

  /** Get current pruning stats (for context pressure signal). */
  getStats(): PruningStats {
    return { ...this.stats };
  }

  /**
   * Get a context pressure value (0.0 - 1.0).
   * This is a heuristic based on estimated context bytes and pruning ratio.
   * Higher values mean more context pressure.
   */
  getContextPressure(): number {
    // Use 128K tokens ≈ 512KB as a rough "full context" estimate
    const estimatedCapacity = 512 * 1024;
    const usageRatio = Math.min(
      1.0,
      this.stats.estimatedContextBytes / estimatedCapacity,
    );

    // If we're pruning a lot, that's also a pressure signal
    const pruneRatio =
      this.stats.totalCalls > 0
        ? this.stats.prunedCalls / this.stats.totalCalls
        : 0;

    // Weighted: 70% usage, 30% prune ratio
    return Math.min(1.0, usageRatio * 0.7 + pruneRatio * 0.3);
  }

  /** Clear history (e.g., on compaction). */
  reset(): void {
    this.history = [];
    this.stats = {
      totalCalls: 0,
      prunedCalls: 0,
      totalBytesSaved: 0,
      estimatedContextBytes: 0,
    };
  }
}
