Fixed shortened calls being recorded as full-output baselines. History now preserves the finalizer’s emitted output, and dedup selects only the most recent matching full result. Argument matching also preserves all arguments.

Verified the required reproducer and seven additional regression tests: 8 passed, 0 failed. Checks cover recovery rejection, retained originals, history reload/eviction, byte accounting, and side-effecting tools.
