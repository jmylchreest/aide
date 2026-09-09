The migration is confined to the usage writer, its recorder closure, and two host call sites in this snapshot. No code was edited and no tests were run. Evidence below refers to the supplied snapshot pinned to `58816d991ef7172304f3eef594844d79e995a5e1`.

1. **Production callers and required migrations**

| Site | Existing call and required migration | Consequence if missed |
|---|---|---|
| Stop: `main()`, inside `hook_event_name === "Stop" && transcript_path`, then `if (binary)` | `recordModelUsage(binary, cwd, usage.events)` → `const result = recordModelUsage({ binary, cwd, events: usage.events })`. The existing call discards its boolean result. [session-summary.ts:108–139](source/src/hooks/session-summary.ts:108) | The positional call violates the new signature; without type checking, it supplies the wrong runtime request. Failing to capture the result also prevents the required logging change. |
| Returned closure inside `createOpenCodeUsageRecorder` | `write(binary, cwd, [next])` → `write({ binary, cwd, events: [next] })`, followed by an explicit acknowledged-status check. Its injected writer type becomes `(request: UsageWriteRequest) => UsageWriteResult`. [model-usage.ts:350–361](source/src/core/model-usage.ts:350) | An unmigrated invocation passes the wrong request. An unmigrated boolean check incorrectly caches unacknowledged writes. |
| OpenCode: callback returned by `createEventHandler`, `"message.part.updated"` branch | `recordUsage(state.binary, state.cwd, event.properties.part)` → `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. Preserve the binary guard. [hooks.ts:432–470](source/src/opencode/hooks.ts:432) | This is the indirect production consumer of the writer. Its old call violates the returned recorder’s signature and fails to supply the intended part through the new object contract. |

The edges matter: `createOpenCodeUsageRecorder()` **does not write at construction**. It selects the default `recordModelUsage` or supplied writer, creates a cache, and returns a closure. Only a later closure invocation with a valid, uncached normalized part invokes that writer. Thus the closure’s `write(...)` is a dynamic call whose target is statically identifiable as `recordModelUsage` for the supplied production factory call, which passes no injection.

The visible OpenCode entry path is `createHooks(cwd, worktree, client, pluginRoot?, options?)` → `initializeAide(state)` → returned `event: createEventHandler(state)` → host invocation of that callback → the branch above. `createHooks`, `createEventHandler(state)`, and the public event callback retain their signatures; the no-argument recorder factory call remains valid. [hooks.ts:154–197](source/src/opencode/hooks.ts:154), [Hooks.event:187–189](source/src/opencode/types.ts:187)

Stop’s entry remains the module-level `main()` invocation, and `main(): Promise<void>` remains unchanged. Its separate `captureSessionSummary(cwd, sessionId, transcriptPath): boolean` does summary storage, not usage writing, and needs no API migration. Usage recording precedes the `stop_hook_active` summary-recursion guard. [session-summary.ts:54–105](source/src/hooks/session-summary.ts:54), [session-summary.ts:125–157](source/src/hooks/session-summary.ts:125)

2. **Behavior and unchanged consumers**

Both new result variants are objects and therefore truthy. Keeping the existing `!write(...)` condition would treat `{ status: "unacknowledged", ... }` as success, add its fingerprint, and suppress later identical retries. Cache insertion must require `result.status === "acknowledged"`.

Preserve the closure’s existing behavior: invalid parts do not write; unsuccessful writes remain retryable; an acknowledged identical snapshot is skipped; changed normalized counters and changed `cwd` are forwarded because the fingerprint is `JSON.stringify([cwd, next])`; and the oldest inserted fingerprint is evicted when acknowledged entries exceed 1,024. Duplicate cache hits do not refresh insertion order. The closure still returns `void`. [model-usage.ts:350–362](source/src/core/model-usage.ts:350)

The writer must preserve the existing process boundary: `observe record --stdin`, one JSON event per line plus a trailing newline, supplied `cwd`, 10,000-ms timeout, and piped stdio. Preserve the case-insensitive, fully matched acknowledgment grammar and its recorded/skipped validation. Return:

- Empty input: `{ status: "acknowledged", recorded: 0 }`, without spawning.
- Complete matching acknowledgment: `{ status: "acknowledged", recorded: events.length }`.
- Malformed acknowledgment, wrong count, or nonzero skipped count: `{ status: "unacknowledged", reason: "invalid-ack" }`.
- Thrown process write: `{ status: "unacknowledged", reason: "write-error" }`.

The existing catch logs `Usage batch was not acknowledged: …`; invalid acknowledgment output currently returns failure without that catch log. Preserve that distinction and the absence of a per-event fallback. [recordModelUsage:318–346](source/src/core/model-usage.ts:318)

Stop currently ignores the result and logs scan status, limitation, malformed count, and event count. Its message must become exactly:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

The following supplied code does **not** consume the changed TypeScript API:

| Code | Evidence and reason no migration is needed |
|---|---|
| Normalization helpers | `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` produce `ObserveBatchEvent | null`; they do not invoke the writer or consume its result. Preserve validation, counters, identity, timestamps, and output handling. [model-usage.ts:121–231](source/src/core/model-usage.ts:121) |
| Transcript collector | `collectTranscriptUsage` reads the explicit bounded file, invokes the Claude/Codex normalizers, and returns `UsageCollection`. It does not write. Preserve its cursor-free rescanning and duplicate/revision retention. [model-usage.ts:234–315](source/src/core/model-usage.ts:234) |
| Generic batch/single-event writers | `recordObserveEventsBatch(binary, cwd, events): void` independently spawns the CLI and falls back to `recordObserveEvent`; the latter independently builds flag arguments. Neither calls `recordModelUsage`. Their signatures and fallback behavior remain unchanged. Sharing `ObserveBatchEvent` or the CLI command is insufficient to make them consumers. [read-tracking.ts:196–239](source/src/core/read-tracking.ts:196), [recordObserveEvent:283–325](source/src/core/read-tracking.ts:283) |
| Go CLI batch ingestion | `cmdObserveRecord` dispatches `--stdin` to `cmdObserveRecordBatch`, which parses `observeBatchLine`, calls `backend.Store().AddObserveEvent`, and prints recorded/skipped counts. It consumes unchanged JSONL, not `UsageWriteRequest` or `UsageWriteResult`. [cmd_observe.go:218–294](source/aide/cmd/aide/cmd_observe.go:218) |
| Go observe types/recorder | `observe.Event` defines the existing event fields; `Recorder.Record` fills identity/time and calls its sink. These are separate Go contracts. [observe.go:22–36](source/aide/pkg/observe/observe.go:22), [Recorder.Record:176–193](source/aide/pkg/observe/observe.go:176) |
| Durable storage | `BoltStore.AddObserveEvent` preserves source time and performs origin-based deduplication in a Bolt transaction. `usageOrigin` includes usage identity and fingerprint, allowing changed revisions to survive. Neither handles the new TypeScript result. [observe_events.go:140–217](source/aide/pkg/store/observe_events.go:140), [token_model_usage.go:36–68](source/aide/pkg/store/token_model_usage.go:36) |
| Go accounting | `modelUsage.observe` detects conflicting revisions; `resultWithSessions` excludes conflicts/invalid records from valid aggregation. `TokenStats` reads stored events and assigns `Accounting.ModelUsage`. The memory structs expose those aggregates, not writer acknowledgments. [token_model_usage.go:135–198](source/aide/pkg/store/token_model_usage.go:135), [token_events.go:153–196](source/aide/pkg/store/token_events.go:153), [token_model_usage.go:5–28](source/aide/pkg/memory/token_model_usage.go:5), [TokenAccounting:32–44](source/aide/pkg/memory/token_accounting.go:32) |

An acknowledgment’s `recorded` count means accepted input records, not necessarily newly inserted durable rows: the CLI increments after a nil storage error, and storage can successfully deduplicate an existing observation.

3. **Original tests and additional checks**

- [model-usage-recording.test.ts:6–20](source/src/test/model-usage-recording.test.ts:6) tests empty **output**, recorded-count mismatches, nonzero skipped count, and a matching acknowledgment. Migrate positional writer calls and boolean assertions to request objects and exact result objects. This test does not exercise empty **input**, thrown writes, spawn arguments, or fallback absence.
- [model-usage.test.ts:44–62](source/src/test/model-usage.test.ts:44) injects a positional boolean writer, fails once, retries successfully, suppresses the next identical snapshot, and forwards changed counters. Migrate its injected callback, return values, and all returned-recorder calls. It asserts three batches; it does not test changed `cwd`, eviction, either named failure reason, or durable store behavior.
- [model-usage-hooks.test.ts:6–18](source/src/test/model-usage-hooks.test.ts:6) uses a boolean writer mock both as Stop’s replacement writer and as the real recorder factory’s injected writer. Migrate that mock and positional call assertions at lines 79 and 136. The OpenCode test exercises the real `createHooks` event handler and confirms `message.updated` causes no additional write; Stop’s parameterized test exercises explicit Claude/Codex transcripts. These tests do not cover the default writer’s process execution, failure retries through the host, or Stop logging. [host tests:60–144](source/src/test/model-usage-hooks.test.ts:60)
- The remaining tests in [model-usage.test.ts](source/src/test/model-usage.test.ts:19) should retain their calls and assertions: timestamp validation/precision, Claude/Codex/OpenCode normalization, absent versus invalid counters, identity rejection, unsafe sums, and bounded transcript scanning. The transcript test exercises malformed JSON, byte-tail limiting, duplicate retention, and a directory source—not every scanner boundary.

Meaningful additional checks:

- Empty event input returns acknowledged/zero with no process call.
- Exact result reasons for invalid acknowledgment and thrown writes; preserve exception logging and no fallback.
- Exact CLI arguments, `cwd`, timeout, stdio, and JSONL payload—including timestamps, attrs, multiple events, and trailing newline. Exercise lowercase CLI output, optional `skipped 0`, and rejection of extra output.
- Parameterize recorder retry tests over both unacknowledged reasons, then acknowledge and confirm deduplication. Assert normalized payload and `void` return.
- Confirm identical parts in different directories write separately; changed counters reach the writer; invalid parts do not.
- Fill 1,024 acknowledged entries, confirm cache hits remain skipped, add the 1,025th, and verify the oldest is forwarded again while retained entries remain cached. Confirm failed writes do not evict successful entries.
- Exercise host routing with the new shapes, missing-binary guard, repeated events, and failure recovery. Separately exercise the default factory writer with a mocked process boundary, since existing host tests inject it.
- Capture Stop debug calls for both statuses and assert the exact suffix while preserving prior fields. Retain both transcript hosts and verify usage recording still occurs when summary recursion is suppressed.

4. **Completeness and unresolved edges**

The supplied-source searches identify one named direct production call to `recordModelUsage`, one dynamic writer invocation inside the returned recorder, and one production invocation of that recorder. The production default-writer binding is visible. The injected implementations supplied by the original tests are also visible.

This is completeness within the supplied files, not the whole repository. Explicit unresolved edges are:

- The application/plugin entry that invokes exported `createHooks` and the external host’s callback delivery are absent; only the returned hook wiring and test invocation are shown.
- The executable launcher supplying Stop’s stdin is absent; module-level `main()` is shown.
- The exported factory permits injected writers, but their possible external implementations and callers are outside the snapshot.
- The actual executable selected by `binary`, its deployed version, and top-level process dispatch are not established by these files.
- `NewBackend` and its concrete `Store()` binding are not supplied. The CLI’s storage call and `BoltStore` implementation are separately visible, but the complete runtime connection is not proven here.

Verification consisted of read-only file inventory, source searches, and inspection of the relevant implementations and original tests. No runtime or test-pass claims are made.
