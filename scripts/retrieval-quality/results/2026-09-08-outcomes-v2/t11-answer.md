Fixed the cause: shortened calls were stored as full-output history entries.

History now preserves the final emitted output, and deduplication selects the latest matching full result, emitting a new baseline after eviction. Argument matching includes all options. Recovery decisions, retained originals, and byte accounting remain consistent after reload.

Verification: the required reproducer and eight additional regression tests passed. No commit created.
