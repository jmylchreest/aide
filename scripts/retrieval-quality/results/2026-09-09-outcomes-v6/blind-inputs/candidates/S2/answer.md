Implemented the request/result API, migrated both adapters and original Vitest consumers, and added Stop write-status logging. Recorder caching now requires acknowledgment.

Validation: `rtk proxy bun test tests` passed all 4 visible regression tests and 6 added contract tests. Original Vitest tests were migrated but not executed; Bun did not type-check TypeScript.
