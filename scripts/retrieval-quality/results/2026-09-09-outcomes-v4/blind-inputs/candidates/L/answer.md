Implemented the request/result API, acknowledgment-based recorder caching, adapter migrations, and Stop debug status. Updated original Vitest consumers and mocks.

`bun test tests`: 10 passed—4 visible regressions plus 6 added contract checks. Original Vitest tests were not executed; Bun checks runtime behavior, not TypeScript types.
