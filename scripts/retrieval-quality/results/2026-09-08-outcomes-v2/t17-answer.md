Fixed the pointer chain: history now records the finalized emitted output, and dedup selects only the latest matching full result. Evicted baselines cause a new full output; recovery rejection remains authoritative.

Updated tracker.ts, dedup.ts, and the types.ts comment. Recovery behavior remains intact.

Verification: visible reproducer passes; all 7 tests pass, covering reloads, eviction, recovery failures, UTF-8 accounting, argument/output differences, and side-effecting tools.
