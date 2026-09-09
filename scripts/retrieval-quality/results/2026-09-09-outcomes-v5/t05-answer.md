Implemented the request/result API, migrated both adapters and original Vitest consumers, and added write status to Stop debug logging. Recorder retries unacknowledged writes while preserving cache behavior.

Validation: `rtk proxy bun test tests` passed all 4 visible tests plus 5 added contract tests. Original Vitest tests were migrated but not executed; TypeScript type-checking was not performed.
