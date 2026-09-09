# Independent reviewer notes

Prepared from current production source, then verified against all SHA-256 hashes in common/provenance.json at source commit 58816d991ef7172304f3eef594844d79e995a5e1. No implementation reference solution or participant trial was inspected or run. Production files were not edited. Every rubric item maps to numbered prompt requirements. Grading should accept equivalent symbols/line references rather than exact prose.

The snapshot has 46 production source files plus three original tests. It contains the full selected TS relative-import closure but is not a complete repository or Go call closure. NewBackend and RPC behavior are intentionally not asserted. Existing source tests are reference material; the fixture's authored harness is separate evidence, not original production behavior.

Index preflight candidates (inspect index usefulness without participant trials):

* src/opencode/hooks.ts:createHooks -> createEventHandler establishes the event closure; its local recordUsage call on message.part.updated is the real indirect usage edge.
* src/core/model-usage.ts:createOpenCodeUsageRecorder default parameter write=recordModelUsage and returned anonymous closure write(...) form dynamic edges that a name-only call graph may not connect.
* src/hooks/session-summary.ts:main is invoked at module scope; detectPlatform supplies Claude/Codex routing. stop_hook_active is below usage scanning, not around it.
* Cross-language hop execFileSync([observe,record,--stdin]) -> cmdObserveRecord -> cmdObserveRecordBatch must be discovered by source, not an assumed call graph link. backend.Store().AddObserveEvent is an interface-like boundary, not proof of the concrete backend implementation.
* Durable usageIdentity/usageFingerprint/usageOrigin and modelUsage.observe/resultWithSessions are required to distinguish equal retries from revisions and filter timing. Generic observe writers are nearby but not on the model-usage write route.

Potential answer errors: call the text skill handler the usage route; use the wrong dedup cache; count unacknowledged calls as cached; add raw output to reasoning; label partial scans complete; equate accepted CLI count with newly inserted rows; filter away a conflict before identity resolution; infer cost/savings/quality from raw counters.
