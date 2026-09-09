The supplied snapshot contains **one direct named production call to `recordModelUsage`, one invocation of its default/injected writer inside the recorder closure, and one production call to that returned recorder**. The required production changes are confined to the writer/recorder implementation, OpenCode’s event branch, and Stop’s usage-writing/logging branch.

All references below are relative to `/tmp/aide-outcomes-v5-trials/t04/root`.

1. **Production callers and migration**

| Site and evidence | Existing behavior | Required migration and consequence of omission |
|---|---|---|
| `main`, Stop branch, [session-summary.ts:125](source/src/hooks/session-summary.ts:125), particularly lines 130–138 | Collects transcript usage, then calls `recordModelUsage(binary, cwd, usage.events)`. Discards the boolean result. | Capture `const result = recordModelUsage({ binary, cwd, events: usage.events })` and append the specified status to the log. Leaving positional arguments violates the new contract; a normal object-destructuring implementation receives no `events`, preventing the write. An exception reaches `main`’s catch at lines 149–151, potentially skipping the subsequent summary capture. |
| Returned closure in [createOpenCodeUsageRecorder:350](source/src/core/model-usage.ts:350), lines 354–359 | Normalizes `part`, checks its fingerprint, then dynamically invokes `write(binary, cwd, [next])`; a true result permits caching. | Type the injected writer as `(request: UsageWriteRequest) => UsageWriteResult`; invoke `write({ binary, cwd, events: [next] })`. Accept `{ binary, cwd, part }` in the returned closure and cache only acknowledged results. Omitting argument migration breaks the injected/default writer contract; omitting status checking suppresses retries after failures. |
| `createEventHandler`’s `"message.part.updated"` branch, [hooks.ts:461](source/src/opencode/hooks.ts:461) | With `state.binary` present, calls `recordUsage(state.binary, state.cwd, event.properties.part)`, then calls `handleMessagePartUpdated`. | Call `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. Leaving positional arguments violates the recorder contract; with object destructuring, `part` becomes undefined and normalization rejects it, silently losing usage recording. |

The factory is **not itself a write**. `createOpenCodeUsageRecorder(write = recordModelUsage)` captures the selected writer and creates a private acknowledgment set; only a later invocation of its returned closure can invoke that writer. The default points to `recordModelUsage`; an explicitly injected function is a dynamic dependency whose implementation must satisfy the new contract.

The visible OpenCode entry path is `createHooks` → `createEventHandler(state)` → returned asynchronous event handler → `"message.part.updated"` → retained `recordUsage` closure → selected writer. `createHooks` initializes state and registers the event handler at [hooks.ts:154](source/src/opencode/hooks.ts:154), lines 161–197; the recorder is constructed once per event-handler factory at line 435.

`createHooks(cwd, worktree, client, pluginRoot?, options?)`, `createEventHandler(state)`, and the host event signature remain unchanged. The latter is also declared in [types.ts:187](source/src/opencode/types.ts:187). Their contextual involvement does not require migrating their callers. Stop’s `main()` invocation at `session-summary.ts:157` also remains unchanged.

2. **Behavior and nonconsumers**

The current `!write(...)` guard is at `model-usage.ts:358`. **Both new result variants are objects and therefore truthy.** Retaining that check would add unacknowledged writes to the cache, causing identical later broadcasts to be skipped even though acknowledgment failed.

Preserve the behavior established by `model-usage.ts:353–361`:

- Invalid/non-step parts produce no write.
- Unacknowledged results remain uncached and retryable, for either reason.
- An acknowledged identical normalized event in the same cwd is skipped.
- Changed counters produce a different fingerprint and reach the writer, retaining conflicting revisions for durable handling.
- Changed cwd produces a different fingerprint and forwards the current cwd.
- The set belongs to the retained recorder instance. After more than 1,024 acknowledged fingerprints, evict the oldest inserted entry. This is insertion-order eviction, not access-order LRU.
- The returned recorder remains `void`.

The writer’s current implementation is [model-usage.ts:319](source/src/core/model-usage.ts:319). Preserve:

- Empty input avoids spawning and becomes `{ status: "acknowledged", recorded: 0 }`.
- `execFileSync(binary, ["observe", "record", "--stdin"], …)`, cwd, newline-delimited JSON with a final newline, 10,000 ms timeout, and piped stdio.
- The existing case-insensitive, anchored acknowledgment grammar after trimming output. Matching count and omitted/zero skipped count acknowledge the complete request.
- Malformed acknowledgment, mismatched count, and nonzero skipped count become `unacknowledged/invalid-ack`.
- Caught write failures become `unacknowledged/write-error`, retaining the current error-name debug message at lines 341–344.
- No per-event fallback, as explicitly explained at line 318.

Stop currently ignores the write outcome. Its exact revised message must be:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

Preserve its existing branch placement: usage writing requires Stop, a supplied transcript path, and a found binary; it occurs before the `stop_hook_active` summary-recursion check (`session-summary.ts:125–144`).

These supplied components **do not consume the changed TypeScript API**:

| Component | Source-grounded reason no API migration is needed |
|---|---|
| Normalization helpers | `event`, `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` construct or reject events; they do not invoke the writer. See [model-usage.ts:92](source/src/core/model-usage.ts:92), lines 92–232. Preserve counter validation, source time, identity, and host-specific normalization. |
| Transcript collector | [collectTranscriptUsage:245](source/src/core/model-usage.ts:245) reads the bounded explicit file, normalizes rows, and returns `UsageCollection`; it does not write. Lines 241–243 explicitly describe rescanning without a cursor so subsequent Stops can retry writes and delayed flushes. |
| Generic observe writers | [recordObserveEventsBatch:218](source/src/core/read-tracking.ts:218) independently spawns the CLI, returns void, and falls back to `recordObserveEvent` on failure. `recordObserveEvent` at line 283 independently builds CLI flags. Neither calls `recordModelUsage`; their signatures and fallback behavior stay unchanged. Shared `ObserveBatchEvent` at line 197 does not establish a call dependency. |
| OpenCode text-part handler | [handleMessagePartUpdated:738](source/src/opencode/hooks.ts:738) accepts text parts, separately tracks text-part deduplication, and emits a generic `user_prompt` event at line 781. It is a sibling call after usage recording, not a consumer of the recorder or its result. |
| Go CLI | [cmdObserveRecord:292](source/aide/cmd/aide/cmd_observe.go:292) selects `cmdObserveRecordBatch`. Its JSONL structure and parser are at lines 218–275, and acknowledgment output at 281–285. It receives events through stdin, not `UsageWriteRequest` or `UsageWriteResult`. |
| Durable storage | [BoltStore.AddObserveEvent:140](source/aide/pkg/store/observe_events.go:140) preserves source timestamps and persists/deduplicates events. Identity and fingerprint construction are in [token_model_usage.go:36](source/aide/pkg/store/token_model_usage.go:36), lines 36–68; unchanged fingerprints deduplicate while revisions can survive. No TypeScript API crosses this boundary. |
| Go accounting | `modelUsage.observe` and `resultWithSessions`, `token_model_usage.go:135–199`, handle validation/conflicts. [BoltStore.TokenStats:153](source/aide/pkg/store/token_events.go:153) reads persisted events and assigns the resulting model usage at line 196. [TokenAccounting:32](source/aide/pkg/memory/token_accounting.go:32) and [TokenModelUsage:23](source/aide/pkg/memory/token_model_usage.go:23) are downstream accounting structures, not writer-result types. |

An acknowledgment count describes accepted input records, not newly inserted distinct identities: the supplied CLI increments `recorded` after a successful store call, including successful durable deduplication.

3. **Original tests and additional checks**

- [model-usage-recording.test.ts:6](source/src/test/model-usage-recording.test.ts:6): tests empty output, too-low/too-high recorded counts, nonzero skipped count, and a matching acknowledgment. Migrate positional calls at lines 16/19 and boolean expectations to exact result objects. This test does **not** exercise an empty event array, thrown writes, spawn arguments, or no-fallback behavior.
- [model-usage.test.ts:44](source/src/test/model-usage.test.ts:44): injects a positional writer returning false once and then true; invokes the recorder positionally four times and asserts three batches. Migrate both contracts while preserving failure → successful retry → duplicate suppression → changed-counter forwarding. Its assertion is a batch count; it does not verify changed cwd, eviction, or persisted conflicts.
- Other tests in that file remain behaviorally unchanged: malformed/precise timestamps (20–42), Codex per-response counters (63–88), Claude normalization and invalid/missing fields (89–132), OpenCode normalization (133–160), unsafe sums (161–177), and bounded transcript scanning/malformed rows/directory rejection (178–200).
- [model-usage-hooks.test.ts:6](source/src/test/model-usage-hooks.test.ts:6): migrate `mocks.write` from boolean results and update positional call assertions at lines 79–88 and 136–141. The mock at lines 16–18 replaces Stop’s writer and injects the same mock into the real recorder factory. OpenCode coverage exercises the real handler with a step part and ignores `"message.updated"`; Stop coverage scans an explicit transcript for each host. Neither asserts Stop logging or real CLI execution.

Additional meaningful checks should verify:

- Empty events return acknowledged/zero with no subprocess.
- Exact result reasons for malformed/count/skipped acknowledgments and thrown errors; valid lowercase acknowledgments, explicit skipped-zero, and multi-event recorded counts.
- Exact subprocess command, cwd, timeout, stdio, and unchanged JSONL—including timestamps, string attributes, escaping, event order, and trailing newline—with no fallback after failures.
- Retry after **each** unacknowledged reason, successful dedup, counter revisions, changed cwd, invalid parts, and a void recorder result.
- Acknowledged-cache capacity at 1,024/1,025 entries, oldest-entry retransmission, retained-entry suppression, and failed writes not consuming cache capacity.
- Host wiring using the object contracts, repeated OpenCode broadcasts, and the existing no-binary gate.
- Stop’s exact appended log suffix for acknowledged and unacknowledged results, preserving all existing scan fields and hook continuation.

4. **Evidence limits and unresolved edges**

Source-wide literal searches and indexed reference candidates were checked against current source. They support the inventory above **within the supplied files**, including the closure edge that named-call references alone omit.

Unresolved external edges are:

- Runtime OpenCode registration/invocation above the exported `createHooks` boundary; no production caller of `createHooks` is supplied.
- External callers or injected writer implementations outside this snapshot.
- The selected executable’s runtime identity/version and actual subprocess behavior.
- `NewBackend`/`Store()` concrete runtime wiring, whose implementations are not supplied. The CLI’s store call and the supplied BoltStore implementation are visible separately, but that binding is not proved here.

No code was edited and no tests were run. This is source-based impact analysis, not whole-repository caller-completeness or runtime verification.
