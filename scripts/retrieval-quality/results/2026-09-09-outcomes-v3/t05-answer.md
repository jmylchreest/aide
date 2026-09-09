Fixed the dedup pointer chain.

- Tracker now records the finalized emitted output.
- Dedup selects the newest matching full result, skipping shortened entries. If none remains, it emits a new full baseline.
- Recovery handling, retained originals, and byte accounting remain intact.

The reproducer initially failed. After the fix, it and seven added regression tests passed: **8 tests, 35 assertions**. Checks cover history reloads, eviction, recovery rejection, argument/output differences, and side-effecting tools.
