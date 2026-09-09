Fixed the cause: shortened calls were recorded as full results, allowing pointers to point to earlier pointers.

History now records the finalized emitted output. Deduplication skips shortened records and selects the latest matching retained full result, emitting a new baseline when necessary. Recovery behavior and byte accounting remain intact after reload.

Verification: supplied reproducer passes; all 8 tests pass, including recovery rejection, eviction, reload, UTF-8 accounting, and side-effecting tools. No commit made.
