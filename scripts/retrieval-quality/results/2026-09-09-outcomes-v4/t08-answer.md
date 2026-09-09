Implemented the request/result API, acknowledgment-only recorder caching, both adapter migrations, and Stop debug write status. Updated the original Vitest consumers and mocks.

Validation: `bun test tests` passed all 4 visible checks plus 5 added contract checks. All six changed source files passed syntax transpilation. Vitest and TypeScript type checking were not run.
