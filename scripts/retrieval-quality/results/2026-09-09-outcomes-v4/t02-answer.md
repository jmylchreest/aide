This trace applies only to the supplied snapshot pinned to `58816d991ef7172304f3eef594844d79e995a5e1`. The scenario results below are deductions from the source, not observed production behavior.

1. **Host entry points and recording**

   OpenCode’s `createHooks` initializes state and returns `event: createEventHandler(state)`. That handler creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, it passes `event.properties.part` to the recorder when `state.binary` exists, then separately calls the text-part handler. `message.updated` has no switch case and is ignored. See [`createHooks`, line 154, and `createEventHandler`, line 432](</tmp/aide-outcomes-v4-trials/t02/root/src/opencode/hooks.ts:154>).

   `openCodeUsageEvent` accepts only a `step-finish` object with valid `sessionID`, part `id`, `messageID`, and an object-valued `tokens`. Its ID check requires a nonblank string, not exactly `"unknown"`, at most 1,024 characters. `messageID` is required but does not become the usage identity. Accepted, unacknowledged variants reach `recordModelUsage`. The separate `handleMessagePartUpdated` requires a nonempty **text** part; it performs skill matching and optionally records a `user_prompt` hook event, not model usage. See [`openCodeUsageEvent`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:203>) and [`handleMessagePartUpdated`](</tmp/aide-outcomes-v4-trials/t02/root/src/opencode/hooks.ts:738>).

   The shared writer synchronously spawns the selected binary with arguments **`observe record --stdin`**, the supplied cwd, newline-delimited JSON plus a final newline, piped stdio, and a 10-second timeout. Empty batches return `true` without spawning. Otherwise success requires an acknowledgment matching `Recorded N event(s)` case-insensitively, with `N` equal to batch length and no skipped events. CLI errors, malformed acknowledgments, and count mismatches return `false`; there is no older-CLI fallback. See [`recordModelUsage`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:319>).

   Claude Code/Codex’s Stop entry point reads and directly parses stdin, uses `data.cwd || process.cwd()` and `data.session_id || "unknown"`, and scans only when `hook_event_name === "Stop"`, an explicit `transcript_path` is present, and binary discovery succeeds. It does not call the available alias-normalization helper. `detectPlatform()` selects Codex only for `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. See [`main`](</tmp/aide-outcomes-v4-trials/t02/root/src/hooks/session-summary.ts:108>) and [`detectPlatform`](</tmp/aide-outcomes-v4-trials/t02/root/src/lib/hook-utils.ts:145>).

   **`stop_hook_active` does not suppress usage scanning.** Its check comes afterward and suppresses only summary capture. Stop calls the shared writer but ignores its boolean and ultimately emits `continue: true`. There is no transcript cursor or acknowledged-record cache on this path, so a later Stop can rescan and retry failed writes, subject to the bounded tail still containing those records.

   [`collectTranscriptUsage`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:245>) behaves as follows:

   - Relative paths, invalid sessions, missing/unreadable files, and nonregular files produce `unavailable`; there is no transcript discovery.
   - A successfully scanned file produces `partial`, even if every byte was read or no usage records matched. It cannot establish complete provider/subagent coverage.
   - Defaults and hard ceilings are **4 MiB of tail bytes** and **1,000 matching usage events**. Ordinary supplied numeric limits are floored and clamped to at least one.
   - Tail truncation or a short read sets `limited`. An incomplete first line at the tail boundary is discarded.
   - Only newline-terminated lines are parsed. An unfinished trailing line—even valid JSON without its newline—is deferred and sets `limited`.
   - Scanning proceeds newest-first, retains the newest matching events, then reverses them into file order. Finding another matching event beyond the event limit sets `limited`.
   - Blank lines are ignored; malformed JSON increments `malformed` and scanning continues. Well-formed nonmatching rows are ignored. The scanner preserves repeated identities/revisions for downstream handling.

2. **Normalized records**

   Every result has `kind: "session"`, `name: "model_usage"`, `model_usage_version: "1"`, and `usage_coverage: "partial"`. Counters are emitted as **string-valued attributes**. These examples produce no `usage_invalid`.

   | Field | OpenCode | Claude Code | Codex |
   |---|---|---|---|
   | Session | `oc-s` | `cc-s` | `cx-s` |
   | `usage_id` | `part-7` | `cc-r` | `cx-r` |
   | `host` | `opencode` | `claude-code` | `codex` |
   | `usage_source` | `opencode.step_finish.v1` | `claude.assistant_usage.v1` | `codex.token_usage_record.v1` |
   | Time basis | `observed` | `source` | `source` |
   | `ts` and `usage_source_time` | Both absent | Both `2026-09-08T12:00:00Z` | Both `2026-09-08T12:00:00Z` |
   | Model | Absent | `m` | Absent |
   | `input_tokens` | `"31"` | `"30"` | `"12"` |
   | `uncached_input_tokens` | `"11"` | `"10"` | `"7"` |
   | `cache_read_input_tokens` | `"17"` | `"20"` | `"5"` |
   | `cache_write_input_tokens` | `"3"` | `"0"` | `"0"` |
   | `output_tokens` | Absent | Absent | `"8"` |
   | `reported_output_tokens` | `"13"` | Absent | Absent |
   | `reasoning_output_tokens` | `"5"` | Absent | `"3"` |
   | `total_tokens` | Absent | Absent | `"20"` |

   OpenCode sums its three input components. It deliberately ignores supplied `tokens.total: 49` and preserves raw output under `reported_output_tokens`; output/reasoning overlap is uncertain, so neither their sum nor a normalized output/total is invented. Claude deliberately omits `output_tokens: 1` because assistant rows can contain a message-start placeholder; it emits no reasoning or total counters. Codex uses per-response `payload.usage`, derives uncached input as `12 − 5 − 0 = 7`, and ignores cumulative `thread_token_usage.input_tokens: 999`. Its reasoning counter is not added again to output or total. See [`claudeUsageEvent`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:121>), [`codexUsageEvent`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:153>), and [`openCodeUsageEvent`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:203>).

   For counters the adapters actually inspect, absence means omission without invalidation. Explicit zero becomes `"0"`. Negative, noninteger, unsafe, or otherwise nonnumeric supplied values are omitted and set `usage_invalid: "1"`. Derived counters require all prerequisites and a safe, nonnegative result; unsafe sums or invalid subtraction also set that flag. Deliberately ignored fields are not validated as emitted counters.

   A supplied invalid timestamp yields `usage_time_basis: "observed"` and `usage_invalid: "1"`, with neither `ts` nor `usage_source_time`. An absent timestamp uses observed timing without invalidation. Valid timestamps retain their original offset and fractional precision. See [`counter`, `summedInput`, `sourceTime`, and `event`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:30>). The store subsequently supplies an observation timestamp when none was provided.

3. **Four OpenCode deliveries and the cache**

   | Delivery at `/work/a` | Writer attempted? | Result |
   |---|---|---|
   | Original part; CLI error | Yes | Nothing persisted; fingerprint not cached |
   | Original part; matching acknowledgment | Yes | Original variant persisted and cached |
   | Original part unchanged | No | Skipped because acknowledged fingerprint matches |
   | Input changed to `12`; matching acknowledgment | Yes | Revised variant persisted and cached; derived input becomes `"32"` |

   Therefore: **3 writer attempts, 1 skipped delivery, 2 distinct persisted variants**.

   Delivering the revised part at `/work/b` **attempts another write**: the in-memory fingerprint is `JSON.stringify([cwd, normalizedEvent])`, so cwd changes its key. Binary path is not part of that key.

   The cache holds at most **1,024 acknowledged fingerprints**, evicting the oldest insertion after a successful addition exceeds the bound. Failed writes never enter it; duplicate hits do not refresh insertion order. Eviction permits later attempts, leaving durable deduplication to storage. See [`createOpenCodeUsageRecorder`](</tmp/aide-outcomes-v4-trials/t02/root/src/core/model-usage.ts:350>).

4. **CLI, durable storage, and accounting**

   `cmdObserve` dispatches `record`; `cmdObserveRecord` selects `cmdObserveRecordBatch` for `--stdin`. The batch opens one backend and scans stdin with a 1 MiB line ceiling. Blank lines are ignored. JSON errors, missing kind/name, explicit zero timestamps, and store errors increment `skipped`; successful `AddObserveEvent` returns increment `recorded`. A scanner error returns an error rather than the final acknowledgment. See [`cmdObserveRecordBatch`](</tmp/aide-outcomes-v4-trials/t02/root/aide/cmd/aide/cmd_observe.go:237>).

   **Boundary:** the supplied CLI calls `NewBackend(dbPath)` and `backend.Store().AddObserveEvent`, but the backend implementation and any RPC connection are absent. The supplied `BoltStore.AddObserveEvent` demonstrates the storage contract; this snapshot does not prove which backend transport connects that CLI call to it.

   Durable **usage identity** is the JSON tuple `[session, host, usage_id]`. The **fingerprint** distinguishes version, source, time basis, model/provider presence and values, invalid marker, and each recognized counter’s presence and value. For source timing it includes timestamp validity, not the actual timestamp. Cwd, generated event ID, actual timestamp, and coverage are not fingerprint components. Thus missing counters differ from explicit zeros. See [`usageIdentity`, `usageFingerprint`, and `usageOrigin`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/store/token_model_usage.go:36>).

   `BoltStore.AddObserveEvent` assigns missing IDs/timestamps and indexes an origin derived from identity **plus fingerprint**. Equal retries reuse the retained event, preserving the earliest timestamp; an earlier equal retry can update that retained event. Conflicting fingerprints survive as separate events. Hence a successful CLI acknowledgment counts accepted calls, **not necessarily newly inserted events**: deduplication also returns success. See [`AddObserveEvent`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/store/observe_events.go:140>).

   `TokenStats` feeds all retained events into `modelUsage.observe` before applying the usage query’s session/time selection. Equal fingerprints collapse; different fingerprints under one identity mark that identity conflicted. `resultWithSessions` excludes conflicting identities entirely from observations and token aggregation. Nonconflicting invalid records are counted separately; valid records group by host/source/model/provider, with per-counter token sums and observation counts. Explicit zero still increments that counter’s observation count. See [`TokenStats`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/store/token_events.go:67>) and [`modelUsage.observe` / `resultWithSessions`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/store/token_model_usage.go:135>).

   For the **first four deliveries only**, the empty store retains two individually valid variants at `t1 < t2`, sharing identity `["oc-s","opencode","part-7"]`. Accounting reports:

   ```json
   {
     "version": 1,
     "observations": 0,
     "conflicts": 1,
     "invalid": 0,
     "by_source": []
   }
   ```

   Selecting `t1` while excluding `t2` **does not remove the conflict**. Conflict discovery considers all retained variants first; a conflicted identity counts once when any retained variant matches the query. No source group or counters are emitted for that identity.

   These session events do **not** become legacy token events: `observeToTokenEvent` returns `nil` for session kind. Consequently they add no legacy event count, tool-call count, text-token estimates, or savings, and do not pass through `TokenAccounting.Add`. Model usage is attached separately to `Accounting.ModelUsage`. See [`observeToTokenEvent`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/store/observe_events.go:82>) and [`TokenAccounting.Add`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/memory/token_accounting.go:69>).

   Valid partial counters establish only the captured host-reported quantities under their source semantics. They do not establish billing, complete session/provider/subagent usage, savings, or task quality; sources must remain separate. This is also explicit in [`TokenModelUsage`](</tmp/aide-outcomes-v4-trials/t02/root/aide/pkg/memory/token_model_usage.go:20>).

Verification was read-only source tracing, cross-checked against the supplied usage-test assertions. No code was edited or tests executed; the fixture’s [README](</tmp/aide-outcomes-v4-trials/t02/root/README.md:1>) identifies it as a navigation snapshot rather than a complete application build.
