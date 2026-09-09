# Trace reference (reviewer only)

Ground truth: untouched files in `common/template`, commit `58816d991ef7172304f3eef594844d79e995a5e1`. References below are paths relative to that template. This answer does not imply complete backend/RPC or whole-repository coverage.

## Host routes and scans

`src/opencode/hooks.ts:createHooks` (154, 197) builds the public event hook with `createEventHandler(state)` (432). The handler creates one `recordUsage = createOpenCodeUsageRecorder()` (435), retaining its closure across events. Only its `message.part.updated` branch (461–465), when `state.binary` is present, passes `state.binary`, `state.cwd`, and `event.properties.part` positionally into that closure. There is no usage route for `message.updated`. `handleMessagePartUpdated` (738) is a separate text-part skill path; its text-only guard excludes step-finish. It is not the usage writer and its `processedMessageParts` cache is unrelated.

`src/core/model-usage.ts:openCodeUsageEvent` (203) requires type step-finish, valid nonempty/non-unknown bounded session/part/message IDs and a tokens object. `createOpenCodeUsageRecorder` (350) normalizes first, skips null, then uses a full normalized-event plus cwd fingerprint and calls its injected `write` (default `recordModelUsage`). This is a dynamic function-parameter edge, not a literal call named recordModelUsage inside the closure.

`src/hooks/session-summary.ts:main` (108; invoked at module end) reads stdin JSON, derives cwd/session, and for Stop with transcript_path finds a binary, calls `collectTranscriptUsage(path, detectPlatform(), sessionId)`, then `recordModelUsage(binary,cwd,usage.events)` (135). It ignores the boolean and logs scan status/limited/malformed/records, not acknowledgment. `stop_hook_active` guards only later summary capture; it does not suppress usage scanning. `src/lib/hook-utils.ts:detectPlatform` (145) selects Codex only for `AIDE_PLATFORM === "codex"`, otherwise Claude Code. OpenCode uses its separate adapter.

`collectTranscriptUsage` (245–316) accepts an explicit absolute path and valid session only, requires a regular file both before and after opening, uses at most 4 MiB and 1,000 events (caller limits clamped to [1, maximum]), reads a tail, discards a partial leading line when needed and an unfinished trailing line, and reads valid complete rows newest first before reversing collected events. It counts JSON parse errors as malformed; valid JSON of an irrelevant type/session simply yields no event. It marks truncation/event overflow/unfinished tail limited. Successful scans remain partial, including empty/no-matching-row scans; missing/unreadable/unsupported sources remain unavailable. There is no cursor; later Stop rescans can retry failed writes or delayed flushes. No discovery of other transcript paths or complete provider/subagent coverage follows.

## Normalized records

All three events have kind `session`, name `model_usage`, `model_usage_version="1"`, and `usage_coverage="partial"`; counters are strings.

| Field | OpenCode | Claude | Codex |
|---|---|---|---|
| session / usage_id | oc-s / part-7 | cc-s / cc-r | cx-s / cx-r |
| host | opencode | claude-code | codex |
| usage_source | opencode.step_finish.v1 | claude.assistant_usage.v1 | codex.token_usage_record.v1 |
| time basis | observed | source | source |
| ts / usage_source_time | absent | supplied timestamp, both | supplied timestamp, both |
| input_tokens | 31 | 30 | 12 |
| uncached_input_tokens | 11 | 10 | 7 |
| cache_read_input_tokens | 17 | 20 | 5 |
| cache_write_input_tokens | 3 | 0 | 0 |
| output_tokens | absent | absent | 8 |
| reported_output_tokens | 13 | absent | absent |
| reasoning_output_tokens | 5 | absent | 3 |
| total_tokens | absent | absent | 20 |
| model | absent | m | absent |

No provider is supplied. OpenCode does not use its `tokens.total`; ambiguous raw output is kept as reported_output_tokens, not normalized output or total, and must not be added to reasoning. Claude input is the sum of three disjoint fields; its output_tokens is deliberately omitted because it may be a message-start placeholder. Codex uses per-response payload.usage and ignores thread_token_usage; uncached input is input minus cache read minus cache write only when all three are supplied and valid. Sources: `src/core/model-usage.ts:claudeUsageEvent` (121), `codexUsageEvent` (153), `openCodeUsageEvent` (203), `event` (92).

`count`, `counter`, `summedInput` require nonnegative safe integers. Explicit zero is preserved; absent fields remain absent and do not become zero. Invalid supplied numeric fields are omitted with usage_invalid=1; unsafe sums are omitted and flagged. Source timestamps must pass explicit calendar/zone validation (`sourceTime`), not merely Date.parse. Invalid supplied time omits ts/source_time and uses observed plus usage_invalid=1; absent time uses observed without an invalid flag. This differs from rejecting an entire row for missing stable identity or unsupported shape.

## Writes, retries and persistence

`recordModelUsage` (319) returns true for empty events without spawn; otherwise uses execFileSync(binary,["observe","record","--stdin"], {cwd,input: JSONL plus final newline,timeout:10000,stdio:["pipe","pipe","pipe"]}). It accepts case-insensitive matching `Recorded N event(s)` with optional `, skipped M`, only if N matches input length and M is zero. Malformed/mismatch/skipped output returns false; exceptions log a nonfatal acknowledgment failure and return false. There is no fallback to a generic individual-event writer, because source timestamps must survive.

Four OpenCode deliveries produce three writer attempts: call 1 fails and is not cached; call 2 succeeds and caches; call 3 is skipped; call 4 changes normalized input from 31 to 32 (uncached 11 to 12), changes fingerprint, succeeds and caches. Two distinct variants persist in the stipulated store. A delivery of the revised part at /work/b attempts a write because cwd belongs to `JSON.stringify([cwd,next])`; binary does not. The cache holds only acknowledged fingerprints, deletes the oldest inserted entry when size exceeds 1,024, and an ordinary cache hit does not refresh insertion order. Cache is local to that factory invocation; durable dedup is separate.

`aide/cmd/aide/cmd_observe.go:cmdObserveDispatcher` routes record to `cmdObserveRecord`, whose --stdin branch calls `cmdObserveRecordBatch` (237). Batch parsing creates an observe.Event including optional ts and calls `backend.Store().AddObserveEvent` for each valid line; malformed/missing-kind/name/zero timestamp and store errors increment skipped. Each nil store error increments recorded, including an already stored origin. Therefore CLI acknowledgment counts accepted lines, not necessarily newly inserted rows. It prints recorded and optional skipped. The supplied file invokes NewBackend but does not establish its implementation or a full RPC route; do not invent those links.

`aide/pkg/store/observe_events.go:AddObserveEvent` (140) assigns IDs/times if absent and honors valid usage_source_time. `token_model_usage.go:usageIdentity` uses session+host+usage_id, while `usageFingerprint` contains version/source/time-basis/model/provider/invalid and presence/value of counters, excluding actual timestamp differences. `usageOrigin` combines identity and fingerprint. Equal origins deduplicate, retaining the earliest time; counter changes create a separate durable variant with the same identity. `AddObserveEvent` returns nil on equal retries, which explains the acknowledgment caveat.

`aide/pkg/store/token_events.go:TokenStats` constructs `newModelUsage`, observes all retained events before date/session result selection, then sets `stats.Accounting.ModelUsage` from `resultWithSessions`. `token_model_usage.go:modelUsage.observe` marks distinct fingerprints for one identity as conflict. `resultWithSessions` counts that conflict if any selected variant lies in the query and excludes all its counters. The scenario yields version=1, observations=0, conflicts=1, invalid=0, by_source=[] (no contributed counters). A window containing t1 but excluding t2 still reports the conflict because evidence was collected before filtering; it cannot rehabilitate the earlier variant. Do not describe the conflict as an invalid row or as a sum of 31 and 32.

`observe_events.go:observeToTokenEvent` returns nil for KindSession, so these observations do not enter legacy tool event counts, delivered-text estimates or savings totals. `aide/pkg/memory/token_model_usage.go` defines counters with their own observation counts and explicitly separates captured partial host counters from text estimates/savings, complete usage, billing/cost or task quality. They demonstrate captured reports, not those broader conclusions.
