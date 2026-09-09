The migration is confined to the TypeScript writer, its recorder closure, the OpenCode event call, Stop’s writer call/logging, and affected tests. The supplied snapshot shows **one direct named production call to `recordModelUsage`, one dependency invocation inside the returned recorder, and one production call to that recorder**.

All evidence below refers to the supplied snapshot identified as commit `58816d991ef7172304f3eef594844d79e995a5e1`. No code was edited and no tests were run.

**1. Production callers and required migration**

| Site and edge | Existing call | Required migration and consequence if missed |
|---|---|---|
| Stop: `main()`, inside `hook_event_name === "Stop" && transcript_path`, with a found binary | `recordModelUsage(binary, cwd, usage.events)` | Pass `{ binary, cwd, events: usage.events }`, retain the result, and append its status to the debug message. Leaving positional arguments violates the new signature and cannot reliably write at runtime. The current call discards the boolean result. Evidence: [session-summary.ts:108](/tmp/aide-outcomes-v5-trials/t10/root/src/hooks/session-summary.ts:108), particularly lines 125–139. |
| Anonymous closure returned by `createOpenCodeUsageRecorder`: dependency invocation | `write(binary, cwd, [next])` inside a boolean guard | Invoke `write({ binary, cwd, events: [next] })` and admit the fingerprint only when `result.status === "acknowledged"`. Missing the argument migration breaks the dependency contract; missing the status check suppresses retries after failures. Evidence: [model-usage.ts:350](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:350), lines 350–362. |
| OpenCode: anonymous event callback returned by `createEventHandler`, `"message.part.updated"` branch, guarded by `state.binary` | `recordUsage(state.binary, state.cwd, event.properties.part)` | Call `recordUsage({ binary: state.binary, cwd: state.cwd, part: event.properties.part })`. Leaving positional arguments violates the returned closure’s new signature; with object destructuring, the old string argument supplies no `part`, so normalization can return null and usage disappears. Evidence: [hooks.ts:432](/tmp/aide-outcomes-v5-trials/t10/root/src/opencode/hooks.ts:432), lines 461–464. |

The factory edge needs separate treatment. At [model-usage.ts:350](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:350), `write = recordModelUsage` selects a dependency; creating the recorder does **not** perform a write. The returned closure captures that dependency and a private acknowledgment set, then invokes the dependency only for a valid, uncached normalized event. The default dependency resolves to `recordModelUsage`; an injected dependency is a dynamic function value required to implement the new request/result contract.

The host path visible in source is:

`createHooks(...)` → `createEventHandler(state)` → returned `event` callback → `"message.part.updated"` → returned `recordUsage` closure → selected writer.

`createHooks` initializes state and installs the handler as `Hooks.event` at [hooks.ts:154](/tmp/aide-outcomes-v5-trials/t10/root/src/opencode/hooks.ts:154), lines 181–197. `createEventHandler` creates its recorder once at line 435. Its no-argument factory call remains valid. Neither `createHooks`’s public arguments nor the event callback’s `{ event }` signature needs migration; the edit belongs to the nested `recordUsage` call. The recorder still returns `void`.

For Stop, `installHookSafetyNet(SOURCE)` and module-level `main()` establish the supplied entry path at [session-summary.ts:155](/tmp/aide-outcomes-v5-trials/t10/root/src/hooks/session-summary.ts:155). Their signatures remain unchanged. `captureSessionSummary` is separate summary work, called after usage collection/writing; its implementation does not call the changed writer ([session-summary.ts:54](/tmp/aide-outcomes-v5-trials/t10/root/src/hooks/session-summary.ts:54), lines 54–105, 141–145).

**2. Behavior to preserve and nonconsumers**

The existing guard is:

```ts
if (acknowledged.has(fingerprint) || !write(binary, cwd, [next])) return;
```

Both new result variants are objects, hence truthy. Keeping `!write(...)` after updating the arguments makes an **unacknowledged** result pass the guard and enter the cache. Subsequent identical broadcasts would be suppressed despite the failed acknowledgment. Evidence: [model-usage.ts:357](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:357).

Preserve these existing recorder behaviors:

- Invalid/non-step parts return before writing.
- Either unacknowledged reason leaves the fingerprint uncached, allowing a later broadcast to retry.
- Acknowledged identical snapshots are suppressed.
- Changed normalized counters produce a different fingerprint and reach the writer, preserving conflicting revisions for downstream handling.
- Changed `cwd` forwards the event again because the fingerprint is `JSON.stringify([cwd, next])`.
- After the 1,025th distinct acknowledged fingerprint, delete the oldest entry. Cache hits do not refresh insertion order.
- Each factory invocation owns its own cache; `binary` is currently absent from the fingerprint.

These follow from [model-usage.ts:353](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:353), lines 353–361. The task does not introduce new exception handling for arbitrary injected writers: the default writer already catches write failures, whereas the current recorder does not catch a dependency that throws.

The writer must map empty input to acknowledged/0 without spawning; matching acknowledgments to acknowledged/`events.length`; invalid acknowledgments to `invalid-ack`; and caught writes to `write-error`. Preserve the implementation boundary at [model-usage.ts:319](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:319):

- `observe record --stdin`;
- one serialized event per line plus a trailing newline;
- supplied executable and cwd, 10,000 ms timeout, piped stdio;
- trimmed output matched against the existing case-insensitive, whole-output acknowledgment grammar, including optional `skipped 0`;
- existing catch-path debug text and absence of fallback.

Stop currently ignores the return and logs collection status/counts only. Its required message is exactly the existing text with this suffix:

```ts
`Usage scan: ${usage.status}; limited=${usage.limited}; malformed=${usage.malformed}; records=${usage.events.length}; write=${result.status}`
```

Collection status remains independent of write status. Preserve the existing branch placement: usage runs even when `stop_hook_active` is true; that flag gates summary capture afterward ([session-summary.ts:125](/tmp/aide-outcomes-v5-trials/t10/root/src/hooks/session-summary.ts:125), lines 125–145).

The following are **not consumers of this TypeScript API**:

| Supplied component | Evidence and boundary |
|---|---|
| Normalizers and validation helpers | `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` produce `ObserveBatchEvent \| null`; they do not call the writer. Preserve their identities, counters, timestamps and event attributes. [model-usage.ts:121](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:121), lines 121–231; shared construction/validation at lines 17–118. |
| `collectTranscriptUsage` | Reads an explicitly supplied file, invokes the Claude/Codex normalizers, and returns `UsageCollection`. It never invokes the writer. Preserve bounded scanning, partial/unavailable status, malformed counting and cursor-free rescanning. [model-usage.ts:245](/tmp/aide-outcomes-v5-trials/t10/root/src/core/model-usage.ts:245), lines 245–315. |
| Generic `recordObserveEventsBatch` / `recordObserveEvent` | These are independent positional, void-returning subprocess wrappers. The batch wrapper falls back to the per-event wrapper; neither delegates to `recordModelUsage`. Their APIs remain unchanged. The per-event options omit source `ts`, reinforcing why usage retains its no-fallback path. [read-tracking.ts:218](/tmp/aide-outcomes-v5-trials/t10/root/src/core/read-tracking.ts:218), lines 218–238; [read-tracking.ts:283](/tmp/aide-outcomes-v5-trials/t10/root/src/core/read-tracking.ts:283), lines 283–325. Sharing `ObserveBatchEvent` at line 197 creates no writer-call edge. |
| OpenCode `handleMessagePartUpdated` | Processes text parts for skill matching and can emit a generic `hook/user_prompt` observation. It is called alongside usage recording but does not call the usage recorder. [hooks.ts:738](/tmp/aide-outcomes-v5-trials/t10/root/src/opencode/hooks.ts:738), lines 738–815. |
| Go CLI batch ingestion | `cmdObserveRecord` dispatches `--stdin` to `cmdObserveRecordBatch`, which parses the existing JSONL schema, calls `backend.Store().AddObserveEvent`, and prints the acknowledgment. It receives events, not the new TS request/result envelope. No migration is required. [cmd_observe.go:219](/tmp/aide-outcomes-v5-trials/t10/root/aide/cmd/aide/cmd_observe.go:219), lines 219–295. |
| Durable storage and model-usage accounting | `BoltStore.AddObserveEvent` preserves timestamps and deduplicates model-usage origins; `usageOrigin` incorporates identity and counter fingerprint. `modelUsage.observe` retains conflicts, and `resultWithSessions` excludes conflicting/invalid identities from counters. These consume stored `observe.Event` data. [observe_events.go:140](/tmp/aide-outcomes-v5-trials/t10/root/aide/pkg/store/observe_events.go:140), lines 155–216; [token_model_usage.go:36](/tmp/aide-outcomes-v5-trials/t10/root/aide/pkg/store/token_model_usage.go:36), lines 36–68, 135–244. |
| Accounting projection/types | `TokenStats` reads stored events, calls `usage.observe`, and assigns the computed model usage to accounting. `TokenModelUsage` describes those totals, not writer acknowledgment. [token_events.go:153](/tmp/aide-outcomes-v5-trials/t10/root/aide/pkg/store/token_events.go:153), lines 153–196; [memory/token_model_usage.go:23](/tmp/aide-outcomes-v5-trials/t10/root/aide/pkg/memory/token_model_usage.go:23). |

A CLI “recorded” count is not a count of newly inserted unique rows: the store can successfully deduplicate a retry, and the CLI increments `recorded` after any successful `AddObserveEvent`. The proposed result should preserve that acknowledgment meaning.

**3. Original tests and meaningful additional checks**

| Original supplied test | Migration versus preserved coverage |
|---|---|
| [model-usage-recording.test.ts:8](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage-recording.test.ts:8), “requires a matching explicit recorded count, not merely exit zero” | Migrate positional writer calls and boolean assertions to request objects and exact result objects. Existing cases exercise empty output, recorded counts below/above the expected count, nonzero skipped count, and one matching acknowledgment. They do **not** exercise empty event input, thrown writes or subprocess arguments. |
| [model-usage.test.ts:44](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage.test.ts:44), “retries failed OpenCode writes and retains conflicting revisions” | Migrate injected callback arguments/result and all four recorder calls. Preserve the failure → success → duplicate → changed-counter scenario. It asserts three submitted batches; it does not exercise either named failure reason, cwd changes, eviction or durable Go conflict handling. |
| [model-usage-hooks.test.ts:6](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage-hooks.test.ts:6), shared writer mock and module mock | Replace the boolean-returning writer mock with a correctly typed request/result mock. The factory mock explicitly injects it into the real recorder; this tests the injected edge, not the actual default writer. |
| [model-usage-hooks.test.ts:61](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage-hooks.test.ts:61), OpenCode host entry test | Migrate the writer argument assertion at line 79. Preserve real `createHooks`/event-handler routing and the assertion that `"message.updated"` does not create another write. It does not test repeated part broadcasts or subprocess/backend execution. |
| [model-usage-hooks.test.ts:105](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage-hooks.test.ts:105), parameterized Stop test | Migrate the writer assertion at line 136. Preserve both Claude and Codex explicit-transcript cases. It currently asserts neither debug logging nor failure/retry behavior. |
| Remaining [model-usage.test.ts:20](/tmp/aide-outcomes-v5-trials/t10/root/src/test/model-usage.test.ts:20) cases | Keep normalization assertions unchanged: invalid/precise source times, Codex per-response counters, Claude input normalization and omitted placeholder output, missing/invalid counters, session/identity validation, OpenCode output separation, and unsafe sums. The scanner test at line 178 covers malformed JSON, duplicate-row retention, a byte-limited tail and directory rejection; it does not cover every scanner limit or incomplete-tail case. |

Additional checks should cover:

- **Writer:** empty events return exactly acknowledged/0 with no spawn; both failure reasons; matching multi-event count; explicit `skipped 0`, case/whitespace acceptance, and rejection of extra output under the unchanged grammar.
- **Wire compatibility:** assert executable, arguments, cwd, timeout, stdio, exact JSONL/trailing newline, and preserved `ts`/attributes. Assert no fallback or second spawn after invalid acknowledgment or a thrown write; assert existing catch logging.
- **Recorder:** retry separately after each unacknowledged reason, then deduplicate after acknowledgment; changed counters and cwd reach the injected writer with correct request fields; invalid parts never write; return value remains void.
- **Eviction:** fill 1,024 acknowledged entries, insert one more, verify the oldest retries and a retained entry stays suppressed. Include a cache hit before overflow to distinguish insertion-order eviction from recency refresh.
- **Host wiring:** preserve the existing event-routing tests and add repeated-part failure/retry/success behavior. Separately exercise the factory’s default writer using a mocked subprocess; the existing host mock substitutes an injected writer.
- **Stop:** assert the exact existing message plus `; write=acknowledged` or `; write=unacknowledged`, including empty collection. Check later Stop invocations rescan/retry and preserve `stop_hook_active` behavior.

**4. Completeness and unresolved edges**

The inventory is bounded to the supplied files. Searches for writer/factory symbols, their imports, and the returned `recordUsage` alias, followed by source reads, establish the call sites above. They do not establish whole-repository completeness.

Explicit unresolved edges are:

- The external production caller/registration of exported `createHooks` and actual OpenCode callback delivery are absent; the supplied test invokes `createHooks` directly.
- Production registration and runtime invocation of the Stop script are outside the supplied entry module.
- Injected writers are dynamic values. The supplied tests show concrete injections, but external injections and consumers of exported functions are not enumerable from this snapshot.
- `execFileSync` crosses a process boundary. The supplied Go command establishes compatible parsing and acknowledgment behavior, but does not prove that the runtime-selected executable is built from this source.
- The CLI calls `NewBackend(...).Store().AddObserveEvent`; the backend constructor/selection implementation is not supplied. The shown `BoltStore` implementation establishes its own behavior, not the concrete backend selected in a running deployment.

Verification was source inspection only; no execution or end-to-end durability claim is made.
