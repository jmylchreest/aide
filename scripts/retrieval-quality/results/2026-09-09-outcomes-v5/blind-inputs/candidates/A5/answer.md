Implemented exported request/result types, migrated the writer, recorder, adapters, and original Vitest consumers. Stop debug output now includes write status.

`rtk proxy bun test tests`: 9 passed—4 unchanged visible tests and 5 added contract tests.

Original Vitest tests were migrated but not executed. Bun runtime checks do not type-check TypeScript.
