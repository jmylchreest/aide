The supplied snapshot has **two production writer invocation sites**—Stop’s direct call and the recorder closure’s injected/default call—and **one production call to that returned recorder**, in OpenCode’s `message.part.updated` branch. The migration is confined to those sites and the writer/recorder contracts. No code was edited and no tests were run.

1. **Caller inventory and required migrations**

| Site and evidence | Existing call and edge | Required migration; consequence if missed |
|---|---|---|
| Stop hook `main`, inside `hook_event_name === "Stop"` with a transcript path and available binary: [session-summary.ts:108](/tmp/aide-outcomes-v4-trials/t04/root/src/hooks/session-summary.ts:108), call at line 135 | Direct `recordModelUsage(binary, cwd, usage.events)`. Its boolean result is currently discarded. | Use `const result = recordModelUsage({ binary, cwd, events: usage.events })` and append the specified status to the scan log. Leaving positional arguments violates the new signature and supplies the wrong runtime request. Migrating arguments alone leaves the required logging incomplete. |
| Anonymous closure returned by `createOpenCodeUsageRecorder`: [model-usage.ts:350](/tmp/aide-outcomes-v4-trials/t04/root/src/core/model-usage.ts:350) | Calls captured `write(binary, cwd, [next])` at line 358. `write` defaults to `recordModelUsage`, but an injected function replaces it. | Type the injected writer with the new request/result contract; return a recorder accepting `{ binary, cwd, part }`; call `write({ binary, cwd, events: [next] })`; cache only an acknowledged result. Missing argument migration breaks the writer contract; retaining the boolean check caches failures. |
| Returned async event handler from `createEventHandler`, `message.part.updated` branch: [hooks.ts:432](/tmp/aide-outcomes-v4-trials/t04/root/src/opencode/hooks.ts:432) | Factory creation at line 435 is `createOpenCodeUsageRecorder()`. When `state.binary` exists, line 463 calls its returned closure as `recordUsage(state.binary, state.cwd, event.properties.part)`. | Change that invocation to `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. A missed migration violates the closure signature; with destructured request handling, the old string argument supplies no `part`, so normalization rejects it and usage is lost. |

The factory does **not** write usage when called. It captures a writer and allocates the acknowledgment cache; later closure invocations normalize parts and conditionally call that writer. The no-argument production factory call resolves the default writer. Supplied tests inject replacements, but no production injection is visible.

For host context, [createHooks at hooks.ts:154](/tmp/aide-outcomes-v4-trials/t04/root/src/opencode/hooks.ts:154) constructs state, initializes it, and exposes `event: createEventHandler(state)` at line 197. Its public signature remains unchanged, as does `createEventHandler(state)` and the host event callback’s `{ event }` signature. Only the internal recorder invocation needs migration. Likewise, Stop’s `main(): Promise<void>` remains unchanged; module-level `main()` at [session-summary.ts:157](/tmp/aide-outcomes-v4-trials/t04/root/src/hooks/session-summary.ts:157) is its visible entry path.

2. **Behavior and unaffected interfaces**

Both result variants are objects and therefore truthy. Retaining line 358’s `!write(...)` after migrating the arguments makes an unacknowledged result pass through to `acknowledged.add(fingerprint)`. The next identical broadcast is then suppressed despite the failed acknowledgment. The discriminator must be checked explicitly: `result.status === "acknowledged"`.

The current closure’s behavior at [model-usage.ts:353](/tmp/aide-outcomes-v4-trials/t04/root/src/core/model-usage.ts:353) establishes these requirements:

- Invalid/non-usage parts produce no write.
- Either unacknowledged reason leaves the fingerprint uncached, allowing repeated retries.
- A successful retry caches the fingerprint; subsequent identical broadcasts are skipped.
- The fingerprint remains `JSON.stringify([cwd, next])`. Changed normalized counters and changed cwd must reach the writer.
- Each factory instance has its own cache. Beyond 1,024 acknowledged entries, remove the oldest inserted entry. Duplicate hits do not refresh insertion order.
- The returned recorder continues returning `void`; the writer’s result is consumed internally.

The writer currently implements its transport and acknowledgment check at [model-usage.ts:319](/tmp/aide-outcomes-v4-trials/t04/root/src/core/model-usage.ts:319). The new results should map directly onto those existing branches:

| Condition | New result |
|---|---|
| Empty events, before spawning | `{ status: "acknowledged", recorded: 0 }` |
| Matching recorded count and zero/absent skipped count | `{ status: "acknowledged", recorded: events.length }` |
| Malformed acknowledgment, count mismatch, or nonzero skipped count | `{ status: "unacknowledged", reason: "invalid-ack" }` |
| Thrown write | `{ status: "unacknowledged", reason: "write-error" }` |

Preserve `observe record --stdin`, cwd, newline-delimited event JSON with its trailing newline, the 10,000 ms timeout, piped stdio, and the case-insensitive anchored acknowledgment grammar. Preserve the existing catch-path debug message at lines 341–344. The writer must retain its no-fallback behavior.

Stop currently logs collection status and event count regardless of acknowledgment; it neither branches on nor stores the result. Its complete new message must be:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

The existing scan and log are inside the binary guard but precede the `stop_hook_active` summary-recursion guard ([session-summary.ts:125](/tmp/aide-outcomes-v4-trials/t04/root/src/hooks/session-summary.ts:125)). Preserve that ordering. Acknowledgment does not establish complete transcript coverage.

These supplied components **do not consume the changed TypeScript API**:

| Component and evidence | Why no API migration is required |
|---|---|
| `event`, `claudeUsageEvent`, `codexUsageEvent`, `openCodeUsageEvent` and normalization helpers: [model-usage.ts:17](/tmp/aide-outcomes-v4-trials/t04/root/src/core/model-usage.ts:17), definitions at lines 92, 121, 153 and 203 | They validate source data and construct `ObserveBatchEvent` values; they do not call the writer. Keep identities, timestamps, counter semantics and attributes unchanged. |
| `collectTranscriptUsage`: [model-usage.ts:245](/tmp/aide-outcomes-v4-trials/t04/root/src/core/model-usage.ts:245) | Reads an explicitly supplied bounded regular-file tail and invokes the Claude/Codex normalizers. Returns `UsageCollection`, without writing. Preserve limits, malformed counting, partial/unavailable status, and cursor-free rescanning for retries and delayed flushes. |
| `ObserveBatchEvent`, `recordObserveEventsBatch`, `recordObserveEvent`: [read-tracking.ts:197](/tmp/aide-outcomes-v4-trials/t04/root/src/core/read-tracking.ts:197), [read-tracking.ts:283](/tmp/aide-outcomes-v4-trials/t04/root/src/core/read-tracking.ts:283) | The generic batch writer independently spawns the CLI and falls back to the generic single-event writer. Neither calls `recordModelUsage`. Their positional, void contracts stay unchanged. Sharing JSONL or the event type does not make them consumers. Their fallback must not replace the usage writer’s timestamp-preserving, acknowledged-only path. |
| Go CLI `observeBatchLine`, `cmdObserveRecord`, `cmdObserveRecordBatch`: [cmd_observe.go:219](/tmp/aide-outcomes-v4-trials/t04/root/aide/cmd/aide/cmd_observe.go:219), definitions at lines 292 and 237 | Consume JSONL across a process boundary, not `UsageWriteRequest` or `UsageWriteResult`. `--stdin` routes to batch ingestion, which maps fields to `observe.Event`, calls `backend.Store().AddObserveEvent`, and prints recorded/skipped counts. Its lowercase `recorded` acknowledgment already matches the case-insensitive parser. |
| Go `observe.Event`: [observe.go:22](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/observe/observe.go:22) | Represents the unchanged event wire/storage fields. It does not represent the new in-process request/result objects. |
| `BoltStore.AddObserveEvent`: [observe_events.go:140](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/store/observe_events.go:140) | Persists events and preserves source time and durable origin deduplication. It has no dependency on the TypeScript signature. |
| `usageIdentity`, `usageFingerprint`, `usageOrigin`, `modelUsage.observe`, `resultWithSessions`: [token_model_usage.go:36](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/store/token_model_usage.go:36), definitions at lines 46, 59, 135 and 172 | Consume stored event identity/attributes. Fingerprints preserve counter revisions; conflicting identities are excluded from reported counters. These durable semantics must remain unchanged. |
| `BoltStore.TokenStats`, `TokenAccounting`, `TokenModelUsage`: [token_events.go:153](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/store/token_events.go:153), [token_accounting.go:32](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/memory/token_accounting.go:32), [token_model_usage.go:23](/tmp/aide-outcomes-v4-trials/t04/root/aide/pkg/memory/token_model_usage.go:23) | Statistics read persisted events and assign the aggregation to `Accounting.ModelUsage` at `token_events.go:196`. They do not receive the writer’s result. |

The CLI increments `recorded` after a nil store error, including accepted durable duplicates. Consequently, the new `recorded` result denotes acknowledged input events, not necessarily newly inserted database rows.

3. **Original tests and meaningful additional checks**

- [model-usage-recording.test.ts:6](/tmp/aide-outcomes-v4-trials/t04/root/src/test/model-usage-recording.test.ts:6): tests empty CLI output, recorded counts of zero/two for one event, nonzero skipped count, and one matching acknowledgment. Migrate positional calls and boolean assertions to request objects and exact result variants. It does **not** currently test empty event input, thrown writes, command/payload options, or explicit `skipped 0`.
- [model-usage.test.ts:44](/tmp/aide-outcomes-v4-trials/t04/root/src/test/model-usage.test.ts:44): injects a positional writer returning false once and true thereafter. It checks retry, successful deduplication, and forwarding changed counters through a three-batch count. Migrate the injected callback, its result values, and all four recorder calls. It does **not** exercise changed cwd or eviction.
- Other tests in that file should preserve their existing calls and assertions: malformed and precise source timestamps (lines 20–43); Codex per-response versus cumulative usage (63–88); Claude input normalization, missing/invalid fields and identity validation (89–132); OpenCode output ambiguity and rejected text parts (133–160); unsafe sums (161–177); transcript malformed records, byte limits, retained repetitions and a directory source (178–200). The transcript test does not exercise durable deduplication, max-event limits, or delayed flush retries.
- [model-usage-hooks.test.ts:6](/tmp/aide-outcomes-v4-trials/t04/root/src/test/model-usage-hooks.test.ts:6): migrate the shared boolean writer mock to a request-aware structured result. Preserve its explicit injection into the real recorder factory at lines 17–18. Change positional `toHaveBeenCalledWith` expectations at lines 79 and 136 to request-object expectations. Existing host coverage checks the real OpenCode event handler forwarding a step part and ignoring `message.updated`; the two Stop cases check explicit Claude/Codex transcript forwarding. They do **not** assert Stop logging, either failure result, host-level retries, or the factory’s production default writer binding.

Additional checks should cover:

- Empty events return acknowledged/zero with no spawn.
- Exact `invalid-ack` and `write-error` results; matching multiple-event acknowledgments; lowercase/whitespace and explicit `skipped 0`; malformed or extra output rejection.
- Exact binary, command, cwd, timeout, stdio, and JSONL payload—including timestamp/attributes and trailing newline—with no fallback spawn and preserved error logging.
- Retry after each unacknowledged reason; repeated failures; success followed by deduplication; changed-counter payload and changed-cwd forwarding; invalid parts; independent recorder instances; `void` return.
- Exactly 1,024 cached successes, eviction on the 1,025th, and re-forwarding the oldest entry while a retained entry remains deduplicated. Verify failures consume no cache capacity and duplicate hits do not refresh order.
- Host event wiring with structured results, retry/dedup through the event handler, the missing-binary guard, and a separate default-writer test without the factory override.
- Stop’s exact existing log prefix plus `; write=acknowledged` or `; write=unacknowledged`, for both failure reasons; empty collections; and preserved continuation/summary-recursion behavior.

4. **Completeness and unresolved edges**

The inventory is bounded to the supplied files identified by the prompt as the snapshot at `58816d991ef7172304f3eef594844d79e995a5e1`. Source-wide name/import searches and inspection of the relevant current definitions support the listed visible callers. Indexed symbol evidence was checked against current source. This is not a whole-repository caller-completeness claim.

The unresolved edges are:

- Arbitrary injected writers: the factory proves a dynamic callable edge; supplied tests provide concrete replacements, but external implementations are unavailable.
- External OpenCode registration/invocation of exported `createHooks`, and external launch/configuration of the Stop module: the internal entry paths are visible, their external callers are not.
- Runtime resolution of `binary` to the inspected Go CLI: the command and matching CLI handler are visible, but the deployed executable/version is not established.
- `NewBackend(...).Store()` resolution: the CLI’s store-interface call and a concrete `BoltStore.AddObserveEvent` implementation are supplied, but backend construction/wiring is missing. An unconditional concrete runtime edge cannot be proved.

Verification was read-only source inspection; no build, test execution, or runtime persistence verification was performed.
