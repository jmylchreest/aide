Fixed `tracker.ts` and `dedup.ts`.

History incorrectly marked shortened results as full outputs. It now records the finalized emitted output, and dedup selects only the most recent matching full result. When that baseline is evicted, the current result stays full.

Verification passed: required reproducer plus six regression tests covering recovery rejection, eviction, retained originals, reload byte accounting, argument/output differences, and side-effecting tools.
