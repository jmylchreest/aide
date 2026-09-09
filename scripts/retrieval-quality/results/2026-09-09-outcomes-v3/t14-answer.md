Fixed the pointer chain: history now stores finalized shortened output, and deduplication selects only matching full-output records.

Updated tracker.ts, dedup.ts, and the types.ts contract. Added regression checks for recovery rejection, retained originals, byte accounting, history reload, eviction, and safe-call behavior.

Verification: the required reproducer passes; all 7 tests pass with 32 assertions.
