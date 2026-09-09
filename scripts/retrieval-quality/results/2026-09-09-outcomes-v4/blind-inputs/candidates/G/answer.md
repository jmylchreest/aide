The supplied snapshot demonstrates partial host-reported usage collection, acknowledgment-based OpenCode retries, and durable exclusion of conflicting revisions. These conclusions apply only to the supplied files at commit `58816d991ef7172304f3eef594844d79e995a5e1`.

**1. Host entry points and writer**

`createHooks` initializes state and returns `event: createEventHandler(state)` (`src/opencode/hooks.ts:154–197`). Initialization discovers `state.binary`; `skipInit` returns before discovery (`initializeAide`, lines 351–408).

`createEventHandler` creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, it passes `event.properties.part` to that recorder if `state.binary` exists, then calls the separate skill handler (`src/opencode/hooks.ts:432–470`).

The recorder calls `openCodeUsageEvent`. Its guards require:

- An object with `type === "step-finish"`.
- Valid `sessionID`, part `id`, and `messageID`.
- An object-valued `tokens`.

The TypeScript ID predicate requires a nonblank string, other than `"unknown"`, of at most 1,024 characters. `messageID` is required but is not emitted or used as the usage identity. Successful normalization proceeds to the shared writer unless already acknowledged in memory. See [model-usage.ts, `openCodeUsageEvent`](source/src/core/model-usage.ts:203).

`message.updated` has no dispatch case and is ignored. `handleMessagePartUpdated` only accepts nonempty text parts for skill matching; it does not collect step usage (`src/opencode/hooks.ts:738–750`). Usage dispatch occurs before that handler, independently of its text-part deduplication.

For Claude Code/Codex, `session-summary.ts` reads stdin, parses the canonical snake-case fields directly, chooses `data.cwd || process.cwd()` and `data.session_id || "unknown"`, and scans only when `hook_event_name === "Stop"`, `transcript_path` is supplied, and a binary is found. It does not call the available `normalizeHookInput` helper. `detectPlatform()` returns Codex only when `AIDE_PLATFORM === "codex"`; otherwise it returns Claude Code (`src/lib/hook-utils.ts:145–148`).

The Stop path calls `collectTranscriptUsage(path, host, session)` and then `recordModelUsage(binary, cwd, usage.events)`. **It ignores the writer’s boolean. `stop_hook_active` suppresses summary capture only, after usage collection.** Errors fail open with `{continue:true}`. See [Stop `main`](source/src/hooks/session-summary.ts:108).

`collectTranscriptUsage`:

- Requires an absolute path, valid session, and a regular file, checked both before and after opening.
- Starts as `unavailable`; rejected, missing, unreadable, or unsupported sources remain unavailable. A successfully read file becomes `partial`, including one with no matching records.
- Reads a bounded tail: default and maximum **4 MiB**, and default/maximum **1,000 accepted events**. Ordinary supplied limits are floored and clamped to at least one.
- Drops an initial cut line unless the tail begins immediately after a newline.
- Drops a final non-newline-terminated line, even if it happens to contain valid JSON, and marks the scan limited.
- Scans complete lines newest-first, keeps matching records, then reverses retained events into file order. Encountering another matching event beyond the event cap marks it limited.
- Skips blank lines; JSON parse failures increment `malformed`. Valid JSON with the wrong shape/session is ignored without incrementing that count.
- Marks byte truncation or a short read limited. Even an unlimited successful scan remains partial coverage.
- Keeps no cursor or acknowledgment cache. A later Stop can retry failed writes or capture delayed transcript flushes, provided those rows remain within its bounded scan.

See [collector implementation](source/src/core/model-usage.ts:245).

The shared [writer, `recordModelUsage`](source/src/core/model-usage.ts:319), synchronously spawns the discovered binary with:

```text
observe record --stdin
```

It supplies newline-delimited JSON plus a final newline, the supplied cwd, piped stdio, and a 10-second timeout. Empty batches return true without spawning. Success requires a case-insensitive acknowledgment matching `Recorded N event(s)` with the exact batch length and zero skipped events. CLI errors, missing/mismatched acknowledgments, or nonzero skipped counts return false. There is no older-CLI fallback.

**2. Normalized records**

All three emit `kind:"session"`, `name:"model_usage"`, `model_usage_version:"1"`, and `usage_coverage:"partial"`. Counters are strings in `attrs`, not the event’s top-level `tokens`.

| Field | OpenCode | Claude Code | Codex |
|---|---|---|---|
| Session | `oc-s` | `cc-s` | `cx-s` |
| `usage_id` | `part-7` | `cc-r` | `cx-r` |
| `host` | `opencode` | `claude-code` | `codex` |
| `usage_source` | `opencode.step_finish.v1` | `claude.assistant_usage.v1` | `codex.token_usage_record.v1` |
| `usage_time_basis` | `observed` | `source` | `source` |
| `ts` and `usage_source_time` | Both omitted | Both `2026-09-08T12:00:00Z` | Both `2026-09-08T12:00:00Z` |
| Model/provider | Both omitted | `model:"m"`; provider omitted | Both omitted |

OpenCode’s persisted timestamp is assigned when the store receives the untimed event.

| Emitted counter | OpenCode | Claude Code | Codex |
|---|---:|---:|---:|
| `input_tokens` | `"31"` | `"30"` | `"12"` |
| `uncached_input_tokens` | `"11"` | `"10"` | `"7"` |
| `cache_read_input_tokens` | `"17"` | `"20"` | `"5"` |
| `cache_write_input_tokens` | `"3"` | `"0"` | `"0"` |
| `output_tokens` | omitted | omitted | `"8"` |
| `reported_output_tokens` | `"13"` | omitted | omitted |
| `reasoning_output_tokens` | `"5"` | omitted | `"3"` |
| `total_tokens` | omitted | omitted | `"20"` |

OpenCode sums its three disjoint input components. It deliberately ignores `tokens.total:49` and keeps raw output as `reported_output_tokens`; output/reasoning overlap is uncertain, so neither `output_tokens` nor a derived total is produced.

Claude sums input, cache-read, and cache-creation input. It deliberately drops assistant `output_tokens:1`, which may be a message-start placeholder. No output, reasoning, or total counter is synthesized.

Codex uses per-response counters, derives uncached input as `12−5−0=7`, and ignores cumulative `thread_token_usage.input_tokens:999`. Its output and reasoning remain separate; reasoning is not added again to output or total. See [the three normalizers](source/src/core/model-usage.ts:121).

For **mapped counters**, absence produces no attribute and no invalid marker. Explicit zero produces `"0"`. Negative, noninteger, unsafe, or otherwise invalid supplied values are omitted and set `usage_invalid:"1"`. Derived counters require all relevant components; unsafe sums or invalid differences also mark the event invalid. Deliberately ignored fields are not validated as counters.

Invalid supplied timestamps yield `usage_time_basis:"observed"` and `usage_invalid:"1"`, with both `ts` and `usage_source_time` omitted. An absent timestamp selects observed time without itself marking invalid. Validation requires an explicit timezone and valid calendar/time components. See [counter and timestamp helpers](source/src/core/model-usage.ts:30).

**3. OpenCode delivery scenario**

The recorder caches `JSON.stringify([cwd, normalizedEvent])` **only after a successful acknowledgment**.

| Delivery at `/work/a` | Writer attempted? | Result |
|---|---|---|
| 1: original, CLI error | Yes | No persistence under the stated assumption; no cache entry |
| 2: original, matching acknowledgment | Yes | Original variant persisted and cached |
| 3: unchanged | No | Skipped because acknowledged |
| 4: input changed to 12, matching acknowledgment | Yes | Revised variant persisted and cached |

Thus: **three writer attempts, one skipped delivery, two distinct persisted variants**. The revision changes both `uncached_input_tokens` from 11 to 12 and derived `input_tokens` from 31 to 32.

Delivering the revised part at `/work/b` **does attempt another write**, because cwd participates in the in-memory key. This does not establish whether `/work/b` resolves to the same durable store.

The cache retains at most **1,024 acknowledged fingerprints**. Adding a 1,025th deletes the oldest inserted entry; cache hits do not refresh its position. Evicted variants may be attempted again, with durable deduplication handling retries. See [recorder closure](source/src/core/model-usage.ts:350).

**4. CLI, durable identity, and accounting**

`cmdObserveRecord` dispatches `--stdin` to `cmdObserveRecordBatch`. That opens a backend once, scans JSON Lines with a 1 MiB scanner-token limit, skips blank lines, and counts malformed records, missing kind/name, zero supplied timestamps, and failed `AddObserveEvent` calls as skipped. It copies accepted fields into `observe.Event`, preserving supplied timestamps. Scanner errors return an error; otherwise it prints the acknowledgment. Successful earlier writes are not rolled back by a later line failure. See [CLI batch implementation](source/aide/cmd/aide/cmd_observe.go:218).

The demonstrated call is `backend.Store().AddObserveEvent(ev)`. **`NewBackend`, its concrete Store selection, and any RPC forwarding implementation are absent from this snapshot.** The supplied BoltStore implementation establishes the following storage contract, but those missing connections cannot be asserted.

- **Durable identity:** JSON tuple `[sessionID, host, usage_id]`.
- **Fingerprint:** version, source, time basis, model/provider, invalid marker, and presence/value of recognized counters; source-time validity also participates for source-timed records.
- Actual timestamp values, coverage, cwd, and OpenCode message ID do not participate in that fingerprint.
- **Durable origin:** combines model-usage identity and fingerprint, allowing conflicting variants to survive while equal retries deduplicate.

These are defined in [identity/fingerprint functions](source/aide/pkg/store/token_model_usage.go:30).

`BoltStore.AddObserveEvent` restores valid source time, supplies missing ID/time, hashes the origin into an index, and retains equal retries as one event. An earlier timestamp for an equal variant replaces the retained timestamp; a later retry returns the existing event successfully. A changed fingerprint has a distinct origin and persists separately. Consequently, **CLI “recorded” counts successful store calls, not necessarily newly inserted events**. See [store writer](source/aide/pkg/store/observe_events.go:140).

`parseUsage` validates the usage metadata and counters, including decimal nonnegative safe integers, at least one counter, input-component consistency, reasoning ≤ output when both exist, and supplied-total consistency. Invalid identities contribute no counters. Explicit zero remains an observed counter; absent counters remain absent.

`TokenStats` scans all retained events through `usage.observe` **before applying the requested model-usage time range**. Differing fingerprints under one identity flag the identity as conflicted. `resultWithSessions` counts a conflict if any retained conflicting representative matches the query, and excludes that identity from observations and counter aggregation. Nonconflicting valid observations are grouped by host/source/model/provider, with per-counter token sums and observation counts. See [aggregation](source/aide/pkg/store/token_model_usage.go:135) and [TokenStats integration](source/aide/pkg/store/token_events.go:153).

For the first four deliveries, the two retained variants share identity `["oc-s","opencode","part-7"]`. Both are individually valid, but conflict:

```json
{
  "version": 1,
  "observations": 0,
  "conflicts": 1,
  "invalid": 0,
  "by_source": []
}
```

No by-source counters are emitted. Selecting `t1` while excluding `t2` **does not remove the conflict**: conflict detection already saw both retained variants, and the `t1` representative selects that conflicted identity.

Finally, `observeToTokenEvent` returns nil for session events, so these events do not increase legacy tool-token event counts, calls, estimates, or savings. Model usage is attached separately to `TokenAccounting.ModelUsage` ([legacy conversion](source/aide/pkg/store/observe_events.go:82)). Partial host counters establish only the retained, valid reported quantities and their coverage—not billing, complete session/provider/subagent usage, savings, or task quality ([model-usage contract](source/aide/pkg/memory/token_model_usage.go:20)).

Verification was read-only source tracing, cross-checked against the supplied normalization, retry, acknowledgment, and host-entry tests. Tests were not executed: this fixture explicitly supplies source evidence rather than a complete build. No code was edited.
