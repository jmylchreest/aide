The supplied snapshot has **one direct production call to `recordModelUsage`, one call through the factory’s captured writer, and one production invocation of the returned recorder**. The required production edits are confined to `src/core/model-usage.ts`, OpenCode’s event-handler call, and Stop’s write/logging block.

1. **Caller inventory and migrations**

| Production site | Existing call and evidence | Required migration and consequence if missed |
|---|---|---|
| Stop: `main()`, branch requiring `hook_event_name === "Stop"`, a transcript path, and an available binary | `recordModelUsage(binary, cwd, usage.events)` at [session-summary.ts:125–139](source/src/hooks/session-summary.ts:125). This is the only visible direct production invocation by that name. | Use `const result = recordModelUsage({ binary, cwd, events: usage.events })`, then append the specified status to the log. Leaving positional arguments violates the new signature and cannot provide the expected request fields. |
| Closure returned by `createOpenCodeUsageRecorder` | The factory defaults `write` to `recordModelUsage`, captures it, and later calls `write(binary, cwd, [next])`; [model-usage.ts:350–362](source/src/core/model-usage.ts:350). | Give the injected writer the request/result contract, return a recorder accepting `{ binary, cwd, part }`, and invoke `write({ binary, cwd, events: [next] })`. Cache only an acknowledged result. Missing either argument migration breaks the corresponding edge; retaining boolean testing suppresses retries after failures. |
| OpenCode: closure returned by `createEventHandler`, `"message.part.updated"` branch | Creates `recordUsage = createOpenCodeUsageRecorder()` once, then—when `state.binary` exists—calls `recordUsage(state.binary, state.cwd, event.properties.part)`; [hooks.ts:432–465](source/src/opencode/hooks.ts:432). | Call `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. Its return remains unused and `void`. Missing this migration supplies no valid request object, potentially dropping usage before the writer is reached. |

The factory call itself **does not write**. With no injected argument, it captures the default writer; subsequent recorder calls normalize a part and conditionally invoke that writer. With an injected argument, that supplied function replaces the default. The supplied production host uses the default; explicit injected writers occur in tests.

The visible OpenCode entry path is `createHooks(cwd, worktree, client, pluginRoot?, options?)`, which initializes state and returns `event: createEventHandler(state)`; [hooks.ts:154](source/src/opencode/hooks.ts:154), lines 164–184 and 196–206. `initializeAide` obtains the binary at [hooks.ts:380](source/src/opencode/hooks.ts:380). Those wrapper signatures, the no-argument factory call, and `Hooks.event(input: { event }) => Promise<void>` stay unchanged; [types.ts:187–189](source/src/opencode/types.ts:187). Only the event branch’s recorder invocation needs migration.

Stop’s top-level `main()` invocation and `captureSessionSummary(cwd, sessionId, transcriptPath)` signature also stay unchanged. The latter handles summary persistence, not usage writes; [session-summary.ts:54–105](source/src/hooks/session-summary.ts:54), with entry invocation at line 157.

2. **Behavioral requirements and nonconsumers**

The existing writer returns `true` for empty input, otherwise runs the batch command and returns an acknowledgment predicate; exceptions are logged and return `false`. The entire implementation is at [model-usage.ts:319–346](source/src/core/model-usage.ts:319).

The replacement must map those outcomes precisely:

- Empty events: `{ status: "acknowledged", recorded: 0 }`, without spawning.
- Matching complete acknowledgment: `{ status: "acknowledged", recorded: events.length }`.
- Malformed acknowledgment, count mismatch, or nonzero skipped count: `{ status: "unacknowledged", reason: "invalid-ack" }`.
- Thrown write: `{ status: "unacknowledged", reason: "write-error" }`.

Preserve `observe record --stdin`, newline-terminated JSONL containing the **events themselves**, `cwd`, the 10,000 ms timeout, piped stdio, and the case-insensitive anchored acknowledgment grammar at lines 326–338. Preserve the exception debug message at lines 341–344 and the absence of fallback. The new request envelope and result must not enter the JSONL wire format.

Both result variants are objects and therefore truthy. Keeping `!write(...)` at line 358 would evaluate to false even for an unacknowledged result, allowing that fingerprint into the cache. Later identical broadcasts would be discarded despite the failed acknowledgment.

The recorder must preserve all existing behavior at lines 353–361:

- Invalid/non-step parts produce no write.
- An unacknowledged snapshot remains retryable.
- An acknowledged identical snapshot is skipped.
- Changed normalized counters produce a different fingerprint and reach storage.
- Changed `cwd` produces a different fingerprint and is forwarded with the current binary and directory.
- Only acknowledged fingerprints occupy the cache.
- After insertion exceeds 1,024 entries, the oldest entry is evicted. This is insertion-order eviction; a duplicate hit does not refresh its position.
- Every path still returns `void`.

Stop currently **discards the boolean result** and logs only scan status, limits, malformed count, and number of collected events. Preserve that existing message and append exactly:

```ts
`; write=${result.status}`
```

Thus `records` remains the collected count, not a claim of successful persistence. The scan/write precedes the `stop_hook_active` summary-recursion guard, so preserve that placement; [session-summary.ts:125–145](source/src/hooks/session-summary.ts:125).

The following are nonconsumers of the changed TypeScript API:

| Supplied code | Why it needs no API migration |
|---|---|
| `claudeUsageEvent`, `codexUsageEvent`, `openCodeUsageEvent`, and their normalization helpers | They construct `ObserveBatchEvent` values; they neither call the writer nor consume its result. Preserve counter validation, identity checks, timestamps, and host-specific output semantics; [model-usage.ts:17–231](source/src/core/model-usage.ts:17). |
| `collectTranscriptUsage` | Returns `UsageCollection`, invoking the Claude/Codex normalizers while scanning. It does not write. Preserve bounded explicit-file scanning, malformed/partial reporting, and cursor-free rescanning for later retries/flushes; [model-usage.ts:234–315](source/src/core/model-usage.ts:234). |
| `ObserveBatchEvent`, `recordObserveEventsBatch`, `recordObserveEvent` | The shared type stays unchanged. The generic batch writer independently invokes the CLI and falls back to the separate per-event writer on exceptions; neither calls `recordModelUsage` nor consumes its result. Their positional APIs and `void` returns remain; [read-tracking.ts:196–239](source/src/core/read-tracking.ts:196), lines 283–325. |
| OpenCode `handleMessagePartUpdated` | This separately handles text parts and can emit a generic `hook/user_prompt` event. It is not the step-finish usage recorder, and its separate 1,000-entry text-part cache is unrelated; [hooks.ts:738–788](source/src/opencode/hooks.ts:738). |
| Go CLI `observeBatchLine`, `cmdObserveRecordBatch`, `cmdObserveRecord` | They consume unchanged JSONL, call `backend.Store().AddObserveEvent`, and print the existing count/skipped acknowledgment. They never receive `UsageWriteRequest` or `UsageWriteResult`; [cmd_observe.go:218–295](source/aide/cmd/aide/cmd_observe.go:218). |
| Go `observe.Event`, `BoltStore.AddObserveEvent`, usage identity/fingerprint logic | These consume event data. Preserve source timestamps and durable identity/revision handling; [observe.go:22–37](source/aide/pkg/observe/observe.go:22), [observe_events.go:140–217](source/aide/pkg/store/observe_events.go:140), [token_model_usage.go:30–68](source/aide/pkg/store/token_model_usage.go:30). |
| Go usage/accounting aggregation | `modelUsage.observe` detects conflicting revisions; `resultWithSessions` excludes conflicting/invalid identities. `TokenStats` reads stored events and assigns `Accounting.ModelUsage`. These are downstream data consumers, not callers of the changed API; [token_model_usage.go:135–244](source/aide/pkg/store/token_model_usage.go:135), [token_events.go:153–198](source/aide/pkg/store/token_events.go:153). The output types remain unchanged; [token_accounting.go:32–44](source/aide/pkg/memory/token_accounting.go:32), [token_model_usage.go:5–28](source/aide/pkg/memory/token_model_usage.go:5). |

The CLI increments `recorded` after a successful store call, including a successful durable deduplication. Consequently, acknowledged `recorded` is not necessarily the number of newly inserted rows; CLI lines 272–276 and store lines 188–205 establish that distinction.

3. **Original tests and meaningful additions**

- [model-usage-recording.test.ts:8–19](source/src/test/model-usage-recording.test.ts:8) checks empty stdout, zero/oversized recorded counts, nonzero skipped count, and one successful acknowledgment. Migrate its positional writer calls and boolean assertions to exact request/result objects. It does **not** test empty event input, thrown writes, spawn arguments, timestamp wire preservation, or fallback absence.
- [model-usage.test.ts:44–62](source/src/test/model-usage.test.ts:44) injects a positional boolean writer, fails the first write, retries successfully, skips an identical snapshot, and forwards changed counters. Migrate the injected callback and all four recorder calls. Its sole final assertion is three batches: it does **not** exercise changed cwd, eviction, or assert forwarded arguments.
- The remaining tests in that file should retain their normalization/collection calls and expectations: malformed/precise source times at lines 20–43; Codex response counters and cumulative-record exclusion at 63–88; Claude input/output semantics at 89–108; missing/invalid values and identity at 109–132; OpenCode output ambiguity and text rejection at 133–160; unsafe sums at 161–177; bounded scanning, malformed rows, and directory rejection at 178–200.
- [model-usage-hooks.test.ts:6–19](source/src/test/model-usage-hooks.test.ts:6) uses one boolean mock both as the Stop export replacement and as an explicitly injected writer inside the real recorder factory. Change it to return the new result, with a matching count. The OpenCode test at lines 61–103 exercises the real handler, validates a step event write, and checks that a later `message.updated` causes no additional write. Migrate its positional assertion at lines 79–88. The Claude/Codex Stop tests at lines 105–144 migrate the positional assertion at lines 136–141. They do not assert debug logging or failed-write behavior.

Additional checks should cover:

- Empty events returning acknowledged/zero with zero process calls.
- Exact reasons for invalid acknowledgment versus thrown execution, including preserved exception logging and no second/fallback spawn.
- Accepted lowercase CLI output, explicit `skipped 0`, surrounding whitespace, and rejection of trailing non-whitespace text.
- Exact command/options and multi-event newline-terminated JSONL, including unchanged `ts` precision and attrs, with no request/result envelope.
- Both unacknowledged reasons remaining retryable; successful retry then dedup; exact forwarding of changed counters and cwd; invalid parts causing no calls; all recorder returns remaining undefined.
- Capacity boundary at 1,024, oldest eviction after the 1,025th acknowledged entry, duplicate hits not refreshing age, and failed writes not occupying capacity.
- The real host handler forwarding the object contract, retrying repeated step broadcasts after failure, and retaining its missing-binary/non-usage behavior. Separately test the factory’s actual default writer path: the original host test substitutes an explicitly injected writer.
- Stop’s exact full debug string for acknowledged and unacknowledged results, including empty collections; continued hook completion and unchanged summary-recursion behavior.

4. **Completeness and verification limits**

I searched the supplied files for writer/factory references and followed the concrete `write` and `recordUsage` bindings. That supports the caller inventory **within this snapshot**. Its [README.md:3–10](source/README.md:3) identifies it as an incomplete navigation fixture copied from `58816d9`, with original Vitest tests included as evidence.

Unresolved edges are:

- Production OpenCode plugin registration and the external runtime invocation of `Hooks.event`; the snapshot proves the returned handler and its branch, not external registration/delivery.
- External Stop registration/invocation; the supplied script proves `main()` and its branch.
- Runtime implementations supplied through the injected-writer parameter beyond the visible test injections.
- The actual executable selected by `binary`, and the concrete backend returned by `NewBackend`/`Store()`. The supplied files show the CLI interface and BoltStore implementation, but do not establish the deployed binary version or concrete runtime backend wiring.

No whole-repository caller-completeness claim follows from these files. A matching acknowledgment also cannot establish complete provider coverage or guarantee every captured event contributes valid accounting counters.

This was a read-only source assessment. **No code was edited and no tests were run.**
