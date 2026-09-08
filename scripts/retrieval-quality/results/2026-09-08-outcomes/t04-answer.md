Fixed `src/core/retrieval-evidence.ts`: recognized `rg`/`grep` searches accept exit status 1, including existing wrappers and host status locations. Explicit failures, interruptions, timeouts, and valid error statuses still fail; conflicting or malformed metadata remains unverified.

Verification: the required reproducer passes. All 30 tests in a temporary regression suite passed, covering wrappers, metadata, failures, baseline exclusion, and existing file/range verification. Scratch tests were removed.
