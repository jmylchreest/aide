Within the supplied snapshot, the migration affects **two production writer invocation sites and one OpenCode recorder invocation site**, across three TypeScript files. No code was edited and no tests were run.

The writer implementation, [recordModelUsage](source/src/core/model-usage.ts:319), must change from three positional arguments and a boolean result to `UsageWriteRequest` and `UsageWriteResult`.

| Production consumer | Existing call and required migration | Consequence if missed |
|---|---|---|
| `main()`, Stop branch in [session-summary.ts:125](source/src/hooks/session-summary.ts:125) | Line 135 calls `recordModelUsage(binary, cwd, usage.events)`. Change to `const result = recordModelUsage({ binary, cwd, events: usage.events })` and append the required status to its debug message. | The positional call violates the new signature; unchecked execution supplies no valid request fields. Omitting result handling also loses the requested write-status diagnostic. |
| Returned closure inside [createOpenCodeUsageRecorder:350](source/src/core/model-usage.ts:350) | Line 358 calls captured `write(binary, cwd, [next])` and tests its boolean value. Change to `write({ binary, cwd, events: [next] })`, accepting only `result.status === "acknowledged"` for caching. The injected writer parameter must have the new contract. | The old argument shape breaks the writer contract. Merely fixing arguments while retaining the truthiness test incorrectly caches failed writes. |
| Returned event handler from `createEventHandler`, `message.part.updated` branch in [hooks.ts:461](source/src/opencode/hooks.ts:461) | Line 463 calls `recordUsage(state.binary, state.cwd, event.properties.part)`. Change to `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. | The call violates the new recorder signature; unchecked execution can supply an undefined `part` to normalization and silently drop usage. |

The OpenCode edge has two distinct stages. [createEventHandler:435](source/src/opencode/hooks.ts:435) calls `createOpenCodeUsageRecorder()` once, capturing the returned recorder. The factory’s default parameter binds `write` to `recordModelUsage`; **factory construction itself does not write**. Later, the event branch invokes the recorder, whose closure normalizes the part, checks its cache, and dynamically invokes the captured writer. An explicitly injected writer replaces that default. Supplied tests exercise injection, but no production injection is visible.

For entry context, [createHooks:154](source/src/opencode/hooks.ts:154) initializes state and exposes `event: createEventHandler(state)` at line 197. Its public signature stays unchanged, as do `createEventHandler(state)` and the host callback shape `{ event } => Promise<void>` declared in [Hooks:187](source/src/opencode/types.ts:187). The factory call with no arguments also remains valid. Only the returned recorder’s input changes; its output remains `void`.

Stop enters through the module’s [main() invocation:157](source/src/hooks/session-summary.ts:157). `main(): Promise<void>` and `HookInput` stay unchanged. Usage collection requires a Stop event, an explicit transcript path and a discovered binary. It occurs before the `stop_hook_active` guard, which guards summary capture, not usage recording.

Both new result variants are objects and therefore truthy. Leaving `!write(...)` in the recorder makes an `unacknowledged` response pass through to `acknowledged.add(...)`. An identical later broadcast then gets suppressed, losing the retry opportunity. The existing [cache logic:353–361](source/src/core/model-usage.ts:353) establishes these behaviors to preserve:

- Invalid or irrelevant parts produce no write.
- Either unacknowledged reason leaves the attempted fingerprint uncached, allowing a later broadcast to retry.
- An acknowledged identical normalized event in the same cwd is suppressed.
- Changed counters produce a different normalized event and must reach the writer, even with the same usage identity.
- Changed cwd must forward the event because the fingerprint includes cwd.
- Successful insertions beyond 1,024 entries evict the oldest entry. Duplicate hits do not refresh insertion order.
- Cache state belongs to each factory-created recorder. It is an optimization; durable deduplication remains downstream.

For Stop, [lines 135–138](source/src/hooks/session-summary.ts:135) currently **discard the boolean result** and report only scan status, limits, malformed rows and collected-record count. The message must become exactly:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

The writer’s existing [transport and acknowledgment logic:324–345](source/src/core/model-usage.ts:324) maps to the proposed outcomes as follows:

| Condition | Required result |
|---|---|
| Empty events | `{ status: "acknowledged", recorded: 0 }`, without spawning |
| Matching complete acknowledgment | `{ status: "acknowledged", recorded: events.length }` |
| Malformed acknowledgment, count mismatch, nonzero skipped count | `{ status: "unacknowledged", reason: "invalid-ack" }` |
| Thrown write | `{ status: "unacknowledged", reason: "write-error" }` |

Preserve `observe record --stdin`, JSONL event serialization with a trailing newline, cwd, 10,000 ms timeout, piped stdio, and the case-insensitive acknowledgment regex with optional skipped count. Preserve the existing exception debug message and no-fallback behavior. An invalid acknowledgment currently returns failure without that exception log.

The following supplied code does **not** consume this TypeScript request/result API:

| Nonconsumer | Source evidence and reason |
|---|---|
| Usage normalization | [claudeUsageEvent:121](source/src/core/model-usage.ts:121), `codexUsageEvent:153`, and `openCodeUsageEvent:203` produce events; they do not call the writer. Their validation, timestamps, counter normalization and omission rules remain unchanged. |
| Transcript collector | [collectTranscriptUsage:245](source/src/core/model-usage.ts:245) reads the explicit file and returns `UsageCollection`. Lines 294–297 invoke normalizers, not the writer. Its bounded scanning and cursor-free rescanning remain unchanged. |
| Generic observe writers | [recordObserveEventsBatch:218](source/src/core/read-tracking.ts:218) independently spawns the CLI and falls back to `recordObserveEvent` on exceptions. [recordObserveEvent:283](source/src/core/read-tracking.ts:283) independently constructs flag arguments. Neither delegates to `recordModelUsage`; both retain positional, `void` APIs. Sharing `ObserveBatchEvent` does not require migration. |
| OpenCode text-part handler | [handleMessagePartUpdated:738](source/src/opencode/hooks.ts:738) handles text parts and uses generic `recordObserveEvent` at line 781. It is a sibling call after usage recording, not the usage recorder’s enclosing function. Its separate part cache is unrelated. |
| Summary capture | [captureSessionSummary:54](source/src/hooks/session-summary.ts:54) gathers/builds/stores summaries. The usage call belongs to `main()`, outside this function. Its signature needs no migration. |
| Go CLI | [observeBatchLine:219 and cmdObserveRecordBatch:237](source/aide/cmd/aide/cmd_observe.go:219) consume JSONL fields, call `backend.Store().AddObserveEvent`, and print recorded/skipped counts. `cmdObserveRecord:292` selects this path with `--stdin`. These wire contracts remain unchanged. |
| Go event API | [observe.Event:22](source/aide/pkg/observe/observe.go:22) and [Recorder.Record:176](source/aide/pkg/observe/observe.go:176) operate on Go events and sinks, not TypeScript result objects. |
| Durable store | [BoltStore.AddObserveEvent:140](source/aide/pkg/store/observe_events.go:140) preserves source time, assigns missing identity/time, and persists/deduplicates events. [usageOrigin:59](source/aide/pkg/store/token_model_usage.go:59) incorporates usage identity and fingerprint, retaining distinct revisions. No API migration is needed. |
| Go accounting | [modelUsage.observe:135](source/aide/pkg/store/token_model_usage.go:135) detects conflicting revisions; `resultWithSessions:172` excludes conflicts and invalid records from totals. [TokenStats:153–196](source/aide/pkg/store/token_events.go:153) reads stored events and assigns model usage. [TokenModelUsage:23](source/aide/pkg/memory/token_model_usage.go:23) and [TokenAccounting:32, Add:69](source/aide/pkg/memory/token_accounting.go:32) retain their existing data contracts. |

The supplied original tests require these specific changes:

| Test evidence | Existing purpose and migration |
|---|---|
| [model-usage-recording.test.ts:8](source/src/test/model-usage-recording.test.ts:8) | Tests empty output, too-small/too-large recorded counts, nonzero skipped count, and a matching acknowledgment. Change positional calls at lines 16/19 and boolean assertions to request objects and exact result objects. It does **not** test empty event input, thrown writes, or subprocess options. |
| [model-usage.test.ts:44](source/src/test/model-usage.test.ts:44) | Tests first-write failure, successful retry, identical-repeat suppression and changed-counter forwarding through a three-batch assertion. Change the injected writer’s positional parameters/boolean return at lines 46–48 and recorder calls at lines 57–60. It does **not** exercise changed cwd or eviction. |
| [model-usage-hooks.test.ts:6](source/src/test/model-usage-hooks.test.ts:6) | The shared mock returns `true`; change it to an acknowledged result. Lines 16–18 replace the direct export and explicitly inject the mock into the actual factory. Change positional writer expectations at lines 79 and 136 to request objects. |
| Same host tests, [line 61](source/src/test/model-usage-hooks.test.ts:61) and [line 105](source/src/test/model-usage-hooks.test.ts:105) | Preserve the real OpenCode event-handler check, ignored `message.updated` check, and parameterized Claude/Codex Stop transcript checks. These tests do not assert Stop logging or prove the production factory’s default-writer binding. |
| Remaining [model-usage.test.ts:20](source/src/test/model-usage.test.ts:20) tests | Preserve malformed/precise source-time tests (20–43), normalization and identity tests (63–176), and bounded transcript/malformed-row/directory-source test (178–200). Their calls and expectations do not consume the changed API. |

Additional meaningful checks should cover:

- Empty input returns exact acknowledged/zero and never invokes `execFileSync`.
- Both exact failure reasons; exceptions preserve error logging and never trigger per-event fallback.
- Exact CLI arguments/options and multi-event JSONL payload, including source timestamp precision and unchanged attrs; accepted lowercase output and optional `skipped 0`; rejected extra output.
- Both failure variants remain retryable, followed by successful caching; changed counters and cwd forward exact requests; invalid parts do not write; recorder returns `undefined`.
- A cache containing 1,024 acknowledgments retains them; the 1,025th evicts the oldest. Repeating a cached entry does not refresh its age, and unacknowledged writes do not consume capacity.
- A factory constructed without injection reaches the real writer using a mocked subprocess boundary. Host tests should assert request fields, repeated-event behavior and binary gating.
- Stop emits the exact appended status for acknowledged and unacknowledged writes, including an empty collection, while preserving scan fields and usage recording when `stop_hook_active` is true.

Completeness is limited to the supplied files. Literal searches for the writer, factory, module imports and `recordUsage`, followed by source inspection, found the sites listed above; they do not establish whole-repository caller completeness. The [fixture README](source/README.md:1) explicitly describes a bounded navigation fixture, not a complete application build.

The unresolved edges are:

- External OpenCode plugin registration and actual host event delivery are outside the supplied production entry path.
- External users of the exported writer/factory and runtime-injected writer implementations cannot be enumerated or validated here.
- The actual executable selected by `binary`, its deployed version, and top-level executable routing are not proved by this snapshot.
- `NewBackend` and the concrete implementation returned by `backend.Store()` are absent. The CLI’s method call and the supplied Bolt implementation are visible, but their runtime binding is not established.

Finally, CLI acknowledgment counts successful `AddObserveEvent` calls, including deduplicated retries; it does not promise that every event created a new row. An unacknowledged batch can already have partially persisted, which makes preserved retry and durable deduplication important. Neither acknowledgment nor transcript scanning establishes complete provider/session usage coverage. Verification here was source inspection only.
