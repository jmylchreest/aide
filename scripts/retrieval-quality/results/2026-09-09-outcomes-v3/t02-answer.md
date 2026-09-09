Fixed the tracker recording shortened calls as full results. History now preserves the finalized emitted output, and dedup selects the most recent retained matching full result.

Verified: reproducer passes; eight additional tests pass covering eviction, reload, recovery rejection, retained originals, byte accounting, argument/output differences, and side-effecting tools.
