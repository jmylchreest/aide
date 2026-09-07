import { ContextPruningTracker } from "./tracker.js";
import type { PruneResult } from "./types.js";

/** In-memory adapter state: restarting the adapter cannot establish continuity
 * with old outputs. Compaction requests suspend references until resolved. */
export class SessionPruningTrackers {
  private trackers = new Map<string, ContextPruningTracker>();
  private suspended = new Set<string>();
  constructor(private cwd?: string) {}

  process(
    session: string,
    call: string,
    tool: string,
    args: Record<string, unknown>,
    output: string,
  ): PruneResult {
    if (!session || session === "unknown" || this.suspended.has(session))
      return { output, modified: false, bytesSaved: 0 };
    let tracker = this.trackers.get(session);
    if (!tracker) {
      tracker = new ContextPruningTracker(this.cwd);
      this.trackers.set(session, tracker);
    }
    return tracker.process(call, tool, args, output);
  }

  pending(session: string): void {
    this.suspended.add(session);
  }
  hasPruned(session?: string): boolean {
    return (
      !!session &&
      !this.suspended.has(session) &&
      (this.trackers.get(session)?.getStats().prunedCalls ?? 0) > 0
    );
  }
  failed(session: string): void {
    this.suspended.delete(session);
  }
  complete(session: string): void {
    this.suspended.delete(session);
    this.trackers.delete(session);
  }
}
