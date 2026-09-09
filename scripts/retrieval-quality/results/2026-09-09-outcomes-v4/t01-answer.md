This trace applies only to the supplied snapshot, identified by the task as commit `58816d991ef7172304f3eef594844d79e995a5e1`. The retry and accounting results below are deductions from that source under your stated assumptions.

**1. Host entry points and scanning**

OpenCode’s `createHooks` initializes state and returns `event: createEventHandler(state)`. The handler creates one `createOpenCodeUsageRecorder` closure. On `message.part.updated`, it passes `event.properties.part` to that recorder when `state.binary` exists, then calls the separate skill handler. `message.updated` has no case and is ignored. See [createHooks](/tmp/aide-outcomes-v4-trials/t01/root/src/opencode/hooks.ts:154) and [createEventHandler](/tmp/aide-outcomes-v4-trials/t01/root/src/opencode/hooks.ts:432).

`openCodeUsageEvent` requires a `step-finish` part, valid session/part/message IDs, and an object-valued `tokens`. Its ID validator requires a nonblank string, not exactly `"unknown"`, with length at most 1,024. The message ID is a guard, but is not emitted as usage identity. The recorder normalizes the part, checks its acknowledged cache, and invokes the shared writer. The text-part skill handler returns for anything other than a nonempty text part, so it does not collect step usage. Initialization with `skipInit` leaves the binary unset and therefore prevents usage dispatch. See [normalizer and guards](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:203), [ID validator](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:22), [skill handler](/tmp/aide-outcomes-v4-trials/t01/root/src/opencode/hooks.ts:738), and [initialization](/tmp/aide-outcomes-v4-trials/t01/root/src/opencode/hooks.ts:351).

For Claude Code/Codex, `session-summary.main` reads stdin, parses JSON directly, defaults cwd to `process.cwd()` and session to `"unknown"`, and requires `hook_event_name === "Stop"` plus a truthy `transcript_path`. After binary discovery, it calls `collectTranscriptUsage(path, detectPlatform(), sessionId)` and then `recordModelUsage`. `detectPlatform` selects Codex only when `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. This entry point does not itself invoke `normalizeHookInput`. See [Stop main](/tmp/aide-outcomes-v4-trials/t01/root/src/hooks/session-summary.ts:108) and [platform selection](/tmp/aide-outcomes-v4-trials/t01/root/src/lib/hook-utils.ts:145).

`stop_hook_active` suppresses only summary capture, **not usage scanning**. Stop ignores the writer’s boolean and normally emits `{continue:true}` regardless. There is no transcript cursor or acknowledged-record cache in this path: a later Stop can retry a failed write or capture newly flushed rows, provided those rows remain within the scan limits. See [Stop ordering](/tmp/aide-outcomes-v4-trials/t01/root/src/hooks/session-summary.ts:125) and [collector](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:241).

The collector:

- Requires an absolute path, valid session, and a regular file checked both before and after opening. Missing, unreadable, unsupported sources or failed initial guards produce `status:"unavailable"`.
- Reads a bounded tail: default and maximum **4 MiB**, default and maximum **1,000 matching usage events**. Ordinary supplied numeric limits are floored and clamped to at least one.
- Discards an initial cut line unless the preceding byte is a newline. It processes only newline-terminated rows; an unfinished trailing row is discarded and marks the scan limited.
- Scans newest lines first, retaining the newest matching events, then reverses them into file order. Encountering another matching event beyond the event limit marks `limited:true`.
- Skips blank lines. JSON parse failures increment `malformed`; valid JSON that does not match the host/session/identity requirements contributes no event.
- Reports `status:"partial"` after a successful read, even for an empty scan or an entire file read. Byte truncation, short reads, unfinished trailing content, and event truncation can set `limited`; none establishes complete coverage.

These behaviors are implemented in [collectTranscriptUsage](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:245).

The shared writer synchronously spawns the discovered binary with arguments `observe record --stdin`, the supplied cwd, newline-delimited JSON, piped stdio, and a **10-second timeout**. Empty batches succeed without spawning. Nonempty batches succeed only when stdout matches `Recorded N event(s)` with the exact submitted count and zero skipped events, case-insensitively. Exceptions return false; exit success alone is insufficient. There is no older-CLI fallback. See [recordModelUsage](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:318).

**2. Normalized records**

All three emit `kind:"session"`, `name:"model_usage"`, `model_usage_version:"1"` and `usage_coverage:"partial"`. All counter values below are emitted as **strings in `attrs`**, not as top-level estimated `tokens` or `saved` fields. See [event constructor](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:92).

| Record | Session / usage ID | Host / source | Time and optional metadata |
|---|---|---|---|
| OpenCode | `oc-s` / `part-7` | `opencode` / `opencode.step_finish.v1` | `usage_time_basis:"observed"`; no `ts`, source time, model or provider |
| Claude | `cc-s` / `cc-r` | `claude-code` / `claude.assistant_usage.v1` | `usage_time_basis:"source"`; both `ts` and `usage_source_time` are `2026-09-08T12:00:00Z`; `model:"m"` |
| Codex | `cx-s` / `cx-r` | `codex` / `codex.token_usage_record.v1` | Same source timestamp fields as Claude; no model or provider |

| Counter | OpenCode | Claude | Codex |
|---|---:|---:|---:|
| `input_tokens` | `"31"` | `"30"` | `"12"` |
| `uncached_input_tokens` | `"11"` | `"10"` | `"7"` |
| `cache_read_input_tokens` | `"17"` | `"20"` | `"5"` |
| `cache_write_input_tokens` | `"3"` | `"0"` | `"0"` |
| `reported_output_tokens` | `"13"` | absent | absent |
| `output_tokens` | absent | absent | `"8"` |
| `reasoning_output_tokens` | `"5"` | absent | `"3"` |
| `total_tokens` | absent | absent | `"20"` |

OpenCode sums the three disjoint input components, preserves raw output separately, and deliberately ignores `tokens.total:49`. It does not derive normalized output or total from raw output plus reasoning. Claude deliberately omits `output_tokens:1`, which may be a message-start placeholder. Codex uses per-response counters, derives uncached input as `12−5−0=7`, and ignores cumulative `thread_token_usage.input_tokens:999`. Its reasoning counter is not added to output. See [Claude normalization](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:121), [Codex normalization](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:153), and [OpenCode normalization](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:203).

For counters the normalizer actually reads, absence means omission without invalidation. Explicit zero becomes `"0"` and remains an observed counter. Negative, noninteger, unsafe, or otherwise nonnumeric supplied values are omitted and set `usage_invalid:"1"`. Derived input requires every component; unsafe sums or invalid differences also mark the event invalid. Deliberately ignored fields do not pass through this validation. See [counter helpers](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:30).

A supplied invalid timestamp produces no `ts` or `usage_source_time`, uses `usage_time_basis:"observed"`, and sets `usage_invalid:"1"`. An absent timestamp uses observed time without that invalid marker. Valid source timestamps preserve their string, including offset and fractional precision. See [sourceTime and event](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:53).

**3. Recorder retry scenario**

| Delivery at `/work/a` | Writer attempt? | Result |
|---|---|---|
| 1: original, CLI error | Yes | No persistence by assumption; no cache entry |
| 2: original, matching acknowledgment | Yes | Original variant persisted and cached |
| 3: unchanged | No | Acknowledged fingerprint already cached |
| 4: input changed to 12, matching acknowledgment | Yes | Revised variant persisted and cached; derived `input_tokens` becomes `"32"` |

Thus there are **3 writer attempts, 1 skipped recorder call, and 2 persisted distinct variants**.

Delivering the revised part at `/work/b` does attempt another write: the in-memory key is `JSON.stringify([cwd, normalizedEvent])`. The binary path is not part of that key. The cache retains at most 1,024 acknowledged keys; adding the 1,025th evicts the oldest insertion. Cache hits do not refresh insertion order, and failures are never cached. Eviction can cause another attempt, leaving durable deduplication to storage. See [createOpenCodeUsageRecorder](/tmp/aide-outcomes-v4-trials/t01/root/src/core/model-usage.ts:349).

**4. CLI, durable identity, and accounting**

The supplied CLI dispatches `observe record` to `cmdObserveRecord`; `--stdin` selects `cmdObserveRecordBatch`. It opens one backend, scans JSON lines with a 1 MiB scanner limit, ignores blank lines, and skips malformed records, missing kind/name, or an explicit zero timestamp. It constructs `observe.Event`, copies any timestamp, and calls `backend.Store().AddObserveEvent`. Store errors increment skipped; successful calls increment recorded. Scanner errors return an error instead of the final acknowledgment. See [dispatcher](/tmp/aide-outcomes-v4-trials/t01/root/aide/cmd/aide/cmd_observe.go:18), [batch parser](/tmp/aide-outcomes-v4-trials/t01/root/aide/cmd/aide/cmd_observe.go:218), and [write/acknowledgment](/tmp/aide-outcomes-v4-trials/t01/root/aide/cmd/aide/cmd_observe.go:269).

The supplied `BoltStore.AddObserveEvent` uses these distinctions:

- **Usage identity:** JSON tuple `[session, host, usage_id]`. For both scenario variants: `["oc-s","opencode","part-7"]`.
- **Fingerprint:** version, source, time basis, model/provider metadata and presence, invalid marker, source-time validity when applicable, and every supported counter’s presence/value. Actual timestamp values and coverage are excluded.
- **Durable deduplication origin:** usage identity plus fingerprint. Equal retries therefore deduplicate, while changed counter variants survive as separate stored events.

The store supplies a ULID and observed timestamp when missing, applies valid source time, and preserves the earliest retained timestamp on equal retries. See [identity/fingerprint/origin](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_model_usage.go:36) and [AddObserveEvent](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/observe_events.go:140).

The acknowledgment counts successful store calls, **not necessarily new insertions**: an equal retry can return success after returning the existing event or updating its earlier timestamp.

`TokenStats` feeds all retained events into `modelUsage.observe` before applying the requested usage time/session selection. Different fingerprints under one identity mark that identity conflicted; accounting excludes its counters. A conflict is reported if any retained variant matches the selection. Therefore selecting `t1` while excluding `t2` **does not remove the conflict**. See [TokenStats](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_events.go:153) and [observe/resultWithSessions](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_model_usage.go:135).

For the first four deliveries alone, in the otherwise empty store:

```json
{
  "version": 1,
  "observations": 0,
  "conflicts": 1,
  "invalid": 0,
  "by_source": []
}
```

Both variants individually have valid counters, but their conflicted identity contributes no source group or token totals. Generally, accepted records group by host/source/model/provider, track source versus observed timing, and count observations separately for each present counter, including explicit zero. Validation rejects invalid markers, malformed counters, inconsistent component sums and inconsistent output/total relationships. See [parseUsage](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_model_usage.go:70) and [aggregation](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_model_usage.go:172).

These session events do not map to legacy `TokenEvent`s, so they contribute no legacy tool-event count, tool calls, text estimates, or savings. Their usage accounting is attached separately. See [observeToTokenEvent](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/observe_events.go:82), [TokenStats tally](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_events.go:82), and [accounting attachment](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/store/token_events.go:191).

Partial host counters establish only the retained, valid, nonconflicting reported quantities and their timing provenance. They do not establish billing, complete session/provider/subagent usage, savings, or task quality. The source explicitly maintains that distinction in [TokenModelUsage](/tmp/aide-outcomes-v4-trials/t01/root/aide/pkg/memory/token_model_usage.go:20).

The CLI call through `NewBackend(...).Store()` and the concrete BoltStore implementation are both supplied; the backend implementation and any RPC connection between them are not. That connection is an evidence boundary, not a demonstrated direct call.

Verification was read-only source tracing and cross-checking the supplied normalizer, hook, and acknowledgment tests. I did not execute tests or modify files.
