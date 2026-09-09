Implemented the request/result API, migrated both adapters and original Vitest consumers, and added Stop write-status logging.

Validation: `rtk proxy bun test tests` passed all 11 tests: 4 unchanged visible checks and 7 added contract checks. Six migrated TypeScript files also parsed successfully. Vitest execution and TypeScript type-checking were not performed.
