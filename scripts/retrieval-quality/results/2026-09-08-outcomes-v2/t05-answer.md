Fixed the fixture. History now stores the finalized emitted output, and dedup selects the most recent matching full result, skipping shortened records. If no full result remains, the current output becomes the new baseline.

Verification: the required reproducer passes; all seven tests pass, including recovery rejection, eviction, persistence, Unicode byte accounting, and side-effect exclusions. No commit created.
