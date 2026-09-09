Implemented the request/result API, migrated both adapters and original Vitest consumers, and added write-status logging. Unacknowledged writes remain retryable.

Validation: `bun test tests` passed all 8 tests—4 existing and 4 added. All six changed source files passed syntax parsing. Vitest execution and TypeScript type-checking were not performed.
