Fixed the cause: shortened calls were incorrectly recorded as full-output history.

History now stores the finalized emitted output, and dedup selects the newest matching full result, skipping shortened entries. Argument identity also preserves all options.

Verification: reproducer passes; all 8 tests pass, covering recovery, persistence, eviction, byte accounting, and side-effect protection.
