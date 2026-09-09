The required production edits are confined to the usage writer/recorder implementation, OpenCode’s recorder invocation, and Stop’s writer invocation/logging. This assessment covers only the supplied snapshot identified as commit `58816d991ef7172304f3eef594844d79e995a5e1`; no code was edited and no tests were run.

1. **Production callers and migration**

| Site | Proven current edge and old shape | Required migration and consequence of omission |
|---|---|---|
| Stop hook, `main`, `Stop` branch | Calls `recordModelUsage(binary, cwd, usage.events)` when a transcript path and binary exist. Its return value is discarded. [session-summary.ts:125](source/src/hooks/session-summary.ts:125) | Capture `const result = recordModelUsage({ binary, cwd, events: usage.events })`, then append the required status to the debug message. Leaving positional arguments is incompatible with the new signature; unchecked execution cannot supply the expected request fields. Migrating the arguments alone leaves the required logging incomplete. |
| Closure returned by `createOpenCodeUsageRecorder` | Calls the captured `write(binary, cwd, [next])`, currently testing its boolean result before caching. [model-usage.ts:350](source/src/core/model-usage.ts:350) | Accept a writer of `(request: UsageWriteRequest) => UsageWriteResult`; return a void recorder accepting `{ binary, cwd, part }`; invoke `write({ binary, cwd, events: [next] })` and cache only acknowledged results. Missing the argument migration breaks the writer boundary; missing the result migration incorrectly caches failures. |
| OpenCode, returned event handler’s `message.part.updated` branch | Calls the returned recorder as `recordUsage(state.binary, state.cwd, event.properties.part)` under `if (state.binary)`, before `handleMessagePartUpdated`. [hooks.ts:461](source/src/opencode/hooks.ts:461) | Call `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. Leaving the old shape fails type checking; unchecked execution of a destructured-request recorder receives no `part`, so normalization rejects it and usage can silently disappear. |

The factory is **not an immediate writer invocation**. Its default parameter captures `recordModelUsage`; a supplied writer replaces that target. Calling the factory allocates an acknowledgment set and returns a closure. Only a later valid, uncached part causes that closure to invoke the captured writer. Thus there is one explicit production call by the name `recordModelUsage`, plus one production writer invocation through the `write` parameter. In this snapshot, production OpenCode uses the default; the supplied tests demonstrate injected writers. [model-usage.ts:350](source/src/core/model-usage.ts:350)

For host context, exported `createHooks(cwd, worktree, client, pluginRoot?, options?)` initializes state and returns `event: createEventHandler(state)`. `createEventHandler` creates one recorder at line 435, retaining its cache across event callbacks. These wrapper signatures, the no-argument factory call, and the public `Hooks.event({ event }): Promise<void>` interface stay unchanged. [hooks.ts:154](source/src/opencode/hooks.ts:154), [hooks.ts:432](source/src/opencode/hooks.ts:432), [types.ts:187](source/src/opencode/types.ts:187)

Stop enters through module-level `main()` after installing the safety net. Its public input shape and `main(): Promise<void>` remain unchanged. `captureSessionSummary` is a separate summary-storage path, not a usage-writer caller. Usage recording currently occurs even when `stop_hook_active` suppresses summary capture; preserve that placement. [session-summary.ts:54](source/src/hooks/session-summary.ts:54), [session-summary.ts:108](source/src/hooks/session-summary.ts:108)

2. **Behavior and boundaries**

Both new result variants are objects and therefore truthy. Keeping `!write(...)` makes an unacknowledged result pass the current guard and enter the cache, suppressing subsequent attempts for the same fingerprint. Only `result.status === "acknowledged"` may add an entry. [model-usage.ts:358](source/src/core/model-usage.ts:358)

Preserve these existing recorder properties:

- Invalid/non-step parts cause no write.
- Unacknowledged attempts remain retryable; acknowledgment suppresses subsequent identical broadcasts.
- The fingerprint is `JSON.stringify([cwd, next])`, so changed normalized counters and changed cwd must reach the writer.
- The set contains acknowledged fingerprints only. Adding entry 1,025 evicts the oldest insertion; a cache hit does not refresh insertion order. Each factory invocation owns a separate cache.
- The recorder returns void, including after writes. Durable deduplication and conflict handling remain downstream.

These follow directly from [model-usage.ts:353](source/src/core/model-usage.ts:353). Stop has no acknowledgment cache or transcript cursor; later scans can retry prior records and pick up delayed transcript flushes. [model-usage.ts:241](source/src/core/model-usage.ts:241)

The writer must map empty input to `{ status: "acknowledged", recorded: 0 }` without spawning; complete matching acknowledgment to `{ status: "acknowledged", recorded: events.length }`; invalid acknowledgment to `{ status: "unacknowledged", reason: "invalid-ack" }`; and a thrown write to `{ status: "unacknowledged", reason: "write-error" }`.

Preserve the existing `observe record --stdin` command, newline-terminated JSONL, supplied cwd, 10,000 ms timeout, piped stdio, case-insensitive anchored acknowledgment grammar, and exception debug message. The existing writer has no per-event fallback. [model-usage.ts:318](source/src/core/model-usage.ts:318)

Stop currently logs scan status and attempted event count, independently of write success. Its replacement message must be exactly:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

Do not replace `records` with the acknowledged count or add the reason to this requested suffix. [session-summary.ts:135](source/src/hooks/session-summary.ts:135)

The following are **not consumers of the changed TypeScript API**:

| Component | Source-grounded reason no API migration is required |
|---|---|
| Normalization helpers | `event`, `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` construct `ObserveBatchEvent` values; they do not invoke the writer. Identity, counters, timestamps and wire attributes remain unchanged. [model-usage.ts:92](source/src/core/model-usage.ts:92) |
| Transcript collector | `collectTranscriptUsage` reads an explicitly supplied regular file, bounds its tail, normalizes rows and returns `UsageCollection`; it neither writes events nor consumes write results. [model-usage.ts:245](source/src/core/model-usage.ts:245) |
| Generic observe writers | `recordObserveEventsBatch(binary, cwd, events): void` independently spawns the CLI and falls back to `recordObserveEvent` on error. `recordObserveEvent` independently builds flag arguments. Neither calls the usage writer. Sharing `ObserveBatchEvent` and the CLI command does not require their migration; substituting the generic batch writer would violate usage’s no-fallback policy. [read-tracking.ts:195](source/src/core/read-tracking.ts:195), [read-tracking.ts:218](source/src/core/read-tracking.ts:218), [read-tracking.ts:283](source/src/core/read-tracking.ts:283) |
| OpenCode text-part handler | `handleMessagePartUpdated` handles text parts and skill injection, with a separate `processedMessageParts` cache; usage recording occurs earlier in the enclosing event branch. Its `recordObserveEvent` call records `user_prompt`, not model usage. [hooks.ts:738](source/src/opencode/hooks.ts:738) |
| Go CLI | `cmdObserveRecord` routes `--stdin` to `cmdObserveRecordBatch`; `observeBatchLine` decodes event JSONL, then the batch handler prints the existing acknowledgment. The new request/result objects remain inside TypeScript. [cmd_observe.go:219](source/aide/cmd/aide/cmd_observe.go:219), [cmd_observe.go:237](source/aide/cmd/aide/cmd_observe.go:237), [cmd_observe.go:292](source/aide/cmd/aide/cmd_observe.go:292) |
| Go event/store/accounting | `observe.Event` carries event fields, not writer statuses. `BoltStore.AddObserveEvent` persists events and deduplicates usage origins; `usageOrigin` incorporates identity and fingerprint, preserving changed revisions. `modelUsage.observe` detects conflicts, and `TokenStats` aggregates stored observations into `Accounting.ModelUsage`. None consumes the TypeScript return value. [observe.go:22](source/aide/pkg/observe/observe.go:22), [observe_events.go:140](source/aide/pkg/store/observe_events.go:140), [token_model_usage.go:36](source/aide/pkg/store/token_model_usage.go:36), [token_model_usage.go:135](source/aide/pkg/store/token_model_usage.go:135), [token_events.go:153](source/aide/pkg/store/token_events.go:153) |

The memory-layer `TokenModelUsage` data model and `TokenAccounting.ModelUsage` field also remain unchanged. They describe accumulated observations, not transport acknowledgment. [token_model_usage.go:23](source/aide/pkg/memory/token_model_usage.go:23), [token_accounting.go:32](source/aide/pkg/memory/token_accounting.go:32)

3. **Supplied tests and needed checks**

- [model-usage-recording.test.ts:6](source/src/test/model-usage-recording.test.ts:6) tests rejection of empty output, recorded-count mismatches and nonzero skipped count, followed by one matching acknowledgment. Migrate positional calls and boolean assertions to request objects and exact result variants. It currently does **not** test empty event arrays, exceptions, spawn options, payload bytes, explicit `skipped 0`, or fallback absence.
- [model-usage.test.ts:44](source/src/test/model-usage.test.ts:44) injects a positional boolean writer and calls the recorder positionally. Migrate both contracts. Its four calls exercise initial failure, successful retry, acknowledged duplicate suppression and changed-counter forwarding; the final assertion checks three batches. It does **not** exercise changed cwd or eviction.
- The remaining tests in that file should retain their normalization/scanning calls and assertions: malformed/precise timestamps, Codex response counters and cumulative-record exclusion, Claude counter handling, missing/invalid fields, session/identity gating, OpenCode output ambiguity, unsafe sums, malformed transcript records, bounded tails and directory rejection. [model-usage.test.ts:20](source/src/test/model-usage.test.ts:20), [model-usage.test.ts:63](source/src/test/model-usage.test.ts:63), [model-usage.test.ts:178](source/src/test/model-usage.test.ts:178)
- [model-usage-hooks.test.ts:6](source/src/test/model-usage-hooks.test.ts:6) replaces the named writer and explicitly injects that mock into the real recorder factory. Change the boolean mock to a request/result writer, and migrate positional `toHaveBeenCalledWith` expectations. The OpenCode test exercises `createHooks` and the actual event handler, then verifies `message.updated` does not add another write. The parameterized Stop test verifies explicit-transcript recording for Claude and Codex. Neither asserts Stop logging; the mocked factory does not prove default-writer execution. [model-usage-hooks.test.ts:60](source/src/test/model-usage-hooks.test.ts:60), [model-usage-hooks.test.ts:105](source/src/test/model-usage-hooks.test.ts:105)

Additional meaningful checks should verify:

- Empty input’s exact result and zero subprocess calls; both failure reasons; exact acknowledged count.
- Multi-event JSONL compatibility, including source timestamps and attributes, trailing newline, command/options, accepted acknowledgment grammar, unchanged exception logging and no fallback.
- Both unacknowledged reasons remain retryable; acknowledgment suppresses duplicates; changed counters and cwd are forwarded; the recorder returns `undefined`.
- At 1,024 acknowledged entries none is prematurely evicted; entry 1,025 evicts the oldest, which can then be written again; duplicate hits do not refresh age and failed attempts do not displace entries.
- Default factory wiring using a mocked subprocess, alongside injected-writer tests; host object forwarding, duplicate/retry behavior through the real event handler, and no usage write for unrelated events.
- Stop’s exact debug string for acknowledged and unacknowledged results, including empty scans, while retaining both host adapters and the existing `stop_hook_active` placement.

4. **What completeness means here**

Literal/import searches across the supplied source, followed by current-source reads of the relevant implementations, establish the visible invocation sites above. They do not establish whole-repository completeness. The fixture explicitly describes itself as a bounded navigation snapshot rather than a complete application build. [README.md:1](source/README.md:1)

Unresolved edges are:

- External consumers of the exported writer or factory, and injected writer implementations beyond those supplied.
- The actual OpenCode loader/event delivery and external Stop-hook registration; the snapshot proves the returned handler and module entry paths, not their external invocation.
- The selected executable’s runtime version and behavior across `execFileSync`.
- Concrete `NewBackend(...).Store()` dispatch: the CLI call and Bolt implementation are supplied, but backend construction/selection is not.

The CLI’s `recorded` count increments after a successful `AddObserveEvent` call, including a successful durable deduplication; it is not proof that each submitted event created a new row. Nor does transport acknowledgment establish valid accounting counters or complete provider coverage. Those distinctions follow from the batch handler, store deduplication, and accounting validation cited above. Verification here was source inspection only.
