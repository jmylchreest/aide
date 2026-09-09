Implemented the request/result API, acknowledgment-only recorder caching, both adapter migrations, and original Vitest consumer updates. Stop logging now includes write status while preserving summary capture.

`rtk proxy bun test tests`: 10 passed—4 unchanged visible checks and 6 added contract checks.

Vitest tests were migrated and reviewed but not executed. Bun runtime checks do not type-check TypeScript.
