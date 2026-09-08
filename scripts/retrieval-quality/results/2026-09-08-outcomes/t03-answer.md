Updated only `src/core/retrieval-evidence.ts`: recognized `rg`/`grep` searches now accept exit status 1 as search evidence. Existing error, interruption, timeout, metadata validation, and baseline safeguards remain intact.

Verification: visible reproducer passes. Six additional temporary tests also passed—142 assertions total—covering wrappers, exit-status locations, malformed/conflicting metadata, baseline prevention, and file/range verification. Temporary tests removed.
