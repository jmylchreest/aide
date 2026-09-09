Fixed the cause: shortened calls were recorded as full-output history entries.

The tracker now stores finalized emitted output, and deduplication selects the latest matching full result, skipping shortened entries. Call identity includes all arguments.

Verification: the visible reproducer and seven added regression tests pass, covering eviction, recovery rejection, persistence, UTF-8 byte accounting, argument differences, and side-effecting tools.
