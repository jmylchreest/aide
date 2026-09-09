Fixed the tracker recording shortened responses as full results. History now preserves the finalized emitted output, and dedup selects the most recent matching full result. If none remains, the current result becomes the new baseline.

Verification: the required reproducer passes; all eight tests pass, covering recovery rejection, retained originals, reload, eviction, byte accounting, distinct calls, and side-effecting tools.
