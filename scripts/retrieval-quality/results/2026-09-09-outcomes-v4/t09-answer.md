The supplied snapshot contains **two production writer invocation sites**: Stop’s direct `recordModelUsage` call and the call through the writer captured by the OpenCode recorder closure. It contains **one production invocation of that returned recorder**, in OpenCode’s `message.part.updated` branch.

This is a read-only assessment of the supplied snapshot identified as commit `58816d991ef7172304f3eef594844d79e995a5e1`. No code was changed and no tests were run.

**Production calls and required migrations**

| Site and evidence | Existing call | Required migration and consequence of omission |
|---|---|---|
| `main()`, Stop branch with an explicit transcript and available binary, [session-summary.ts:125](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:125) | `recordModelUsage(binary, cwd, usage.events)` at line 135; return value discarded | Capture `const result = recordModelUsage({ binary, cwd, events: usage.events })`, then extend the debug message. Keeping positional arguments violates the new signature and cannot supply the required request fields. Migrating only the arguments leaves the requested write-status logging absent. |
| Returned closure inside `createOpenCodeUsageRecorder`, [model-usage.ts:350](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:350) | `write(binary, cwd, [next])` inside a boolean condition at line 358 | Call `write({ binary, cwd, events: [next] })`; cache only when `result.status === "acknowledged"`. Keeping positional arguments breaks the writer contract; retaining boolean evaluation incorrectly caches unacknowledged writes. |
| Returned async handler from `createEventHandler`, `message.part.updated` branch, [hooks.ts:461](/tmp/aide-outcomes-v4-trials/t09/root/src/opencode/hooks.ts:461) | `recordUsage(state.binary, state.cwd, event.properties.part)` | Call `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`, preserving the binary guard. The old call violates the recorder signature and fails to deliver the part through its required object field. |

The factory edge is significant: `createOpenCodeUsageRecorder(write = recordModelUsage)` selects and captures a writer; constructing the recorder does **not** write an event. Its returned closure normalizes a part, checks the cache, and later invokes that captured writer. With no injection, this reaches `recordModelUsage`; with injection, it reaches the supplied function. The factory must accept the new writer request/result type, while its returned recorder changes from three positional arguments to one object and still returns `void`. [model-usage.ts:350](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:350)

The visible OpenCode entry path is `createHooks(...)` → returned `Hooks.event` from `createEventHandler(state)` → event callback → returned usage recorder → captured writer. `createHooks` and `createEventHandler` keep their existing signatures, and `createOpenCodeUsageRecorder()` at line 435 remains a valid no-argument factory call. Only the recorder invocation inside that handler requires the argument migration. [hooks.ts:154](/tmp/aide-outcomes-v4-trials/t09/root/src/opencode/hooks.ts:154), [hooks.ts:197](/tmp/aide-outcomes-v4-trials/t09/root/src/opencode/hooks.ts:197), [hooks.ts:432](/tmp/aide-outcomes-v4-trials/t09/root/src/opencode/hooks.ts:432)

Stop enters through the module’s `main()` invocation. Neither `main(): Promise<void>` nor `captureSessionSummary(cwd, sessionId, transcriptPath): boolean` needs a signature change. `captureSessionSummary` performs summary work; it is not the usage-writer caller. Usage recording happens before the `stop_hook_active` summary guard. [session-summary.ts:54](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:54), [session-summary.ts:108](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:108), [session-summary.ts:141](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:141), [session-summary.ts:157](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:157)

**Behavior that must survive**

Both result variants are objects and therefore truthy. Merely replacing the arguments in the existing `!write(...)` condition would make an unacknowledged result pass through to `acknowledged.add(fingerprint)`. Subsequent broadcasts of that snapshot would be suppressed instead of retried. [model-usage.ts:358](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:358)

Preserve these existing recorder properties:

- Invalid/non-step parts return without writing.
- Either unacknowledged reason leaves the snapshot eligible for a later retry; successful retries then deduplicate.
- The fingerprint remains `JSON.stringify([cwd, next])`: changed counters and changed cwd reach the writer, even for an already-seen part identity.
- Cache hits skip writing. Successful insertion beyond 1,024 entries removes the oldest insertion; this is not a cache whose hits refresh recency.
- The closure remains `void`. Stop remains able to rescan and retry on later invocations.

These behaviors follow from [model-usage.ts:203](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:203), [model-usage.ts:350](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:350), and the cursor-free collector at [model-usage.ts:241](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:241).

For `recordModelUsage`, replace the boolean outcomes with the specified discriminated results: empty input acknowledges zero without spawning; a complete matching acknowledgment acknowledges `events.length`; malformed/mismatched/skipped acknowledgments produce `invalid-ack`; a thrown write produces `write-error`. Preserve the current `observe record --stdin` invocation, newline-terminated JSONL, 10,000 ms timeout, piped stdio, case-insensitive acknowledgment regex, caught-error debug message, and absence of a fallback. [model-usage.ts:319](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:319)

Stop currently ignores the boolean and logs collection status and attempted record count, regardless of acknowledgment. Its existing message must become exactly:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

The `records` value remains the collected event count, and the collector’s `usage.status` remains distinct from write acknowledgment. [session-summary.ts:130](/tmp/aide-outcomes-v4-trials/t09/root/src/hooks/session-summary.ts:130)

**Code that does not require this API migration**

| Component | Source-grounded reason |
|---|---|
| Normalization helpers | `event`, `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` construct `ObserveBatchEvent` values; they do not invoke the writer or inspect its return value. Keep their identities, counters, timestamp handling, and output semantics unchanged. [model-usage.ts:92](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:92), [model-usage.ts:121](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:121), [model-usage.ts:153](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:153), [model-usage.ts:203](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:203) |
| `collectTranscriptUsage` | Reads a bounded explicit file, normalizes records, and returns `UsageCollection`; it does not write. Its partial/unavailable status, limits, malformed count, and revision retention are independent of the new result type. [model-usage.ts:245](/tmp/aide-outcomes-v4-trials/t09/root/src/core/model-usage.ts:245) |
| `ObserveBatchEvent`, `recordObserveEventsBatch`, `recordObserveEvent` | Sharing the event type is not a call edge. The generic batch writer independently invokes the CLI and falls back to its own per-event writer; both remain positional, fire-and-forget APIs. They neither call `recordModelUsage` nor consume its result. [read-tracking.ts:196](/tmp/aide-outcomes-v4-trials/t09/root/src/core/read-tracking.ts:196), [read-tracking.ts:218](/tmp/aide-outcomes-v4-trials/t09/root/src/core/read-tracking.ts:218), [read-tracking.ts:283](/tmp/aide-outcomes-v4-trials/t09/root/src/core/read-tracking.ts:283) |
| Go CLI | `observeBatchLine` consumes event JSON, not the TypeScript request wrapper. `cmdObserveRecord` routes `--stdin` to `cmdObserveRecordBatch`, which calls `backend.Store().AddObserveEvent` and prints the existing acknowledgment. The request/result objects must remain on the TypeScript side of this boundary. [cmd_observe.go:219](/tmp/aide-outcomes-v4-trials/t09/root/aide/cmd/aide/cmd_observe.go:219), [cmd_observe.go:237](/tmp/aide-outcomes-v4-trials/t09/root/aide/cmd/aide/cmd_observe.go:237), [cmd_observe.go:292](/tmp/aide-outcomes-v4-trials/t09/root/aide/cmd/aide/cmd_observe.go:292) |
| Durable store and accounting | `BoltStore.AddObserveEvent` works with `observe.Event`, timestamps, and durable origin deduplication. `usageOrigin` includes identity and fingerprint; `modelUsage.observe` retains conflicting revisions. `TokenStats` reads stored events and assigns aggregated model usage. None consumes the TypeScript writer result. [observe_events.go:140](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/store/observe_events.go:140), [token_model_usage.go:36](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/store/token_model_usage.go:36), [token_model_usage.go:135](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/store/token_model_usage.go:135), [token_events.go:153](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/store/token_events.go:153) |

The Go `TokenModelUsage` and `TokenAccounting.ModelUsage` types likewise represent reporting data, not write acknowledgments. They require no corresponding type migration. [token_model_usage.go:23](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/memory/token_model_usage.go:23), [token_accounting.go:32](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/memory/token_accounting.go:32)

**Original tests and additional checks**

| Supplied test | Migration and actual existing coverage |
|---|---|
| [model-usage-recording.test.ts:8](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage-recording.test.ts:8) | Migrate positional writer calls and `toBe(false/true)` assertions to request objects and exact result objects. Existing cases exercise empty output, under/over-counts, nonzero skipped count, and one successful acknowledgment. They do **not** exercise empty input, thrown writes, or spawn/payload options. |
| [model-usage.test.ts:44](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage.test.ts:44) | Migrate the injected callback’s positional parameters and boolean result, plus four recorder calls. It exercises failure → successful retry → duplicate suppression → changed-counter forwarding, asserting three batches. It does **not** exercise changed cwd or eviction. |
| [model-usage-hooks.test.ts:6](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage-hooks.test.ts:6) | Replace the boolean-returning mock with the new writer contract. The module mock replaces Stop’s writer and explicitly injects the same mock into the actual recorder factory; preserve that distinction. |
| [model-usage-hooks.test.ts:61](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage-hooks.test.ts:61) | Change expected writer arguments to one request object. Existing coverage invokes the real OpenCode handler for a step part and checks that a subsequent `message.updated` event produces no additional write. It does not exercise default-writer execution or retry/dedup through the host. |
| [model-usage-hooks.test.ts:105](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage-hooks.test.ts:105) | Migrate Stop’s expected writer arguments for both Claude Code and Codex explicit transcripts. These tests do not assert write-status logging. |

The remaining normalization and collector tests should retain their calls and assertions: malformed/valid timestamps at lines 20–42; Codex counters at 63–88; Claude normalization, invalid fields, and identity validation at 89–132; OpenCode normalization at 133–160; unsafe sums at 161–177; bounded transcript scanning, malformed records, and directory rejection at 178–200. [model-usage.test.ts](/tmp/aide-outcomes-v4-trials/t09/root/src/test/model-usage.test.ts)

Additional meaningful checks should cover:

- Empty input returns exactly acknowledged/zero with no spawn; thrown writes return `write-error`; every acknowledgment rejection returns `invalid-ack`.
- Lowercase CLI output, explicit `skipped 0`, and existing whitespace handling remain accepted. Verify exact command, cwd, timeout, and event-only JSONL including timestamps and trailing newline; failure triggers no fallback and preserves error logging.
- Both unacknowledged reasons permit retry; acknowledgment suppresses duplicates; changed counters and changed cwd forward correctly.
- After 1,025 distinct acknowledged snapshots, the oldest retries while retained entries remain suppressed; failed writes do not consume cache capacity.
- The default recorder writer path, injected path, recorder’s `undefined` return, and real host object wiring, including retry/dedup and the binary guard.
- Stop’s exact debug suffix for both statuses, preservation of existing scan fields, and repeated Stop attempts after an unacknowledged result.

**Evidence limits**

File enumeration, snapshot-wide symbol/import searches, and current-source reads support the caller inventory **within the supplied files**. Indexed implementations were checked against current source; this is not a whole-repository completeness claim.

Unresolved edges are explicit:

- The production caller that loads/invokes exported `createHooks`, and the external host that delivers events, are outside the supplied entry-path evidence.
- The captured `write` parameter is dynamically selectable. The default and supplied test injections are visible; external injected implementations are not.
- `execFileSync(binary, ...)` targets a runtime-selected executable. The supplied Go CLI shows the expected protocol, but does not prove which executable runs.
- `NewBackend` and the concrete implementation returned by `backend.Store()` are not supplied. The CLI store call and `BoltStore.AddObserveEvent` implementation are visible separately; their concrete runtime binding is not established here.

Finally, acknowledgment counts successful CLI store calls, including durable deduplicated retries; it does not prove that every count represents a newly inserted record, complete transcript coverage, or complete provider usage. The CLI increments after a nil store error, while the store can return successfully for an existing origin. [cmd_observe.go:272](/tmp/aide-outcomes-v4-trials/t09/root/aide/cmd/aide/cmd_observe.go:272), [observe_events.go:188](/tmp/aide-outcomes-v4-trials/t09/root/aide/pkg/store/observe_events.go:188)
