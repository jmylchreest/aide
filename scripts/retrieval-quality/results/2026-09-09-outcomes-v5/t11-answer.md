This trace applies only to the supplied snapshot at commit `58816d991ef7172304f3eef594844d79e995a5e1`. No code was edited. Scenario results below are deductions from the source, not observed production executions.

**1. Host entry points and the shared writer**

OpenCode’s `createHooks` initializes state and returns `event: createEventHandler(state)`. Initialization discovers the binary; `skipInit` returns before discovery, leaving the initial binary null. `createEventHandler` creates one usage recorder and dispatches **`message.part.updated`** to `recordUsage(state.binary, state.cwd, event.properties.part)` when a binary exists, then invokes the separate text-part handler. See `src/opencode/hooks.ts`: `createHooks` lines 154–207, `initializeAide` 351–403, and `createEventHandler` 432–471.

The recorder normalizes through `openCodeUsageEvent`, which requires:

- An object with `type: "step-finish"`.
- Valid `sessionID`, part `id`, and `messageID`.
- A non-array object `tokens`.

The TypeScript ID check requires a nonblank string, not exactly `"unknown"`, at most 1,024 characters. `messageID` is required but is not emitted or used as `usage_id`; the part ID is. Invalid individual counters mark the event invalid rather than preventing normalization. See `src/core/model-usage.ts`: `id` 22–29, `openCodeUsageEvent` 203–231.

**`message.updated` does not collect usage**: it falls through the event switch’s default. The skill handler does not collect it either: `handleMessagePartUpdated` immediately rejects non-text parts; its separate optional reflection observation is a `hook/user_prompt`, not model usage. See `src/opencode/hooks.ts` lines 461–468 and 738–788.

For Claude Code/Codex, `src/hooks/session-summary.ts:108–151` parses stdin, defaults cwd to `process.cwd()` and session to `"unknown"`, and proceeds only for `hook_event_name === "Stop"` with a truthy `transcript_path`. If binary discovery succeeds, it calls:

```text
collectTranscriptUsage(transcript_path, detectPlatform(), sessionId)
→ recordModelUsage(binary, cwd, usage.events)
```

`detectPlatform` selects Codex only when `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. It does not inspect transcript contents or require `CLAUDE_PLUGIN_ROOT` (`src/lib/hook-utils.ts:145–148`). Binary lookup can consult the session anchor before falling back to cwd-based lookup (`findAideBinary`, lines 264–277).

**`stop_hook_active` does not suppress usage scanning.** Its guard comes afterward and suppresses only summary capture. Stop ignores the writer’s boolean and emits `{continue:true}`. There is no scan cursor or success checkpoint, so a later Stop can retry a failed write or discover delayed transcript flushes—provided the row remains inside the bounded scan. This is retry opportunity, not guaranteed eventual capture (`session-summary.ts:125–148`; `model-usage.ts:241–315`).

`collectTranscriptUsage`:

- Reads only the explicitly supplied absolute path, requiring a valid session and a regular file before and after opening.
- Returns `unavailable` for rejected paths/sessions, missing/unreadable files, or unsupported sources.
- Returns `partial` after a successful scan, even for an empty file or zero matching records. A complete file scan does not establish complete provider/subagent coverage.
- Defaults to, and caps at, a **4 MiB tail** and **1,000 normalized events**. Ordinary configured limits are floored and clamped to at least one.
- Drops an incomplete first line when the tail starts inside a line. Drops a final line without a terminating newline—even if that line is valid JSON—and marks that unfinished tail limited.
- Scans complete lines newest-first, retaining the newest matching events, then reverses them into source order. Encountering another matching event beyond the cap marks the scan limited.
- Counts malformed JSON lines encountered during that scan, skips blank lines, and ignores valid JSON that does not match the selected host/session/schema. Semantic invalidity in a normalized counter is distinct from malformed JSON.
- Marks byte truncation or a short read limited. It does not deduplicate collected records.

Both adapters reach `recordModelUsage` (`src/core/model-usage.ts:319–346`). For a nonempty batch it synchronously spawns the discovered binary with **`observe record --stdin`**, supplied cwd, newline-delimited JSON plus a final newline, piped stdio, and a 10-second timeout. Success requires an explicit case-insensitive acknowledgment with the exact batch count and zero skipped events. CLI errors, missing/mismatching acknowledgments, or nonzero skipped counts return false. Empty batches return true without spawning. There is no older-CLI fallback.

**2. Normalized records**

All three emit `kind: "session"`, `name: "model_usage"`, `model_usage_version: "1"`, and `usage_coverage: "partial"`. Counters are string-valued attributes. Normalization is implemented by `event`, `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` in `src/core/model-usage.ts:92–231`.

| Field | OpenCode | Claude Code | Codex |
|---|---|---|---|
| Session | `oc-s` | `cc-s` | `cx-s` |
| `usage_id` | `part-7` | `cc-r` | `cx-r` |
| Host | `opencode` | `claude-code` | `codex` |
| `usage_source` | `opencode.step_finish.v1` | `claude.assistant_usage.v1` | `codex.token_usage_record.v1` |
| Time basis | `observed` | `source` | `source` |
| `ts` / `usage_source_time` | Both absent | Both `2026-09-08T12:00:00Z` | Both `2026-09-08T12:00:00Z` |
| Model/provider | Both absent | Model `m`; provider absent | Both absent |
| `input_tokens` | `"31"` | `"30"` | `"12"` |
| `uncached_input_tokens` | `"11"` | `"10"` | `"7"` |
| `cache_read_input_tokens` | `"17"` | `"20"` | `"5"` |
| `cache_write_input_tokens` | `"3"` | `"0"` | `"0"` |
| `output_tokens` | Absent | Absent | `"8"` |
| `reported_output_tokens` | `"13"` | Absent | Absent |
| `reasoning_output_tokens` | `"5"` | Absent | `"3"` |
| `total_tokens` | Absent | Absent | `"20"` |

OpenCode sums the three disjoint input components. It deliberately omits the supplied `tokens.total:49` and does not promote raw output to `output_tokens`, because output/reasoning overlap varies across versions. **Do not add 13 and 5.**

Claude similarly sums its three input components, preserving explicit cache-write zero. It deliberately ignores `output_tokens:1` because assistant output can be a message-start placeholder; it does not derive a total.

Codex uses per-response usage, derives uncached input as `12−5−0=7`, and ignores cumulative `thread_token_usage.input_tokens:999`. It does not add reasoning output to output again.

For recognized counters, absence means omission without an invalid marker. An explicit nonnegative safe integer—including zero—is emitted. Negative, fractional, unsafe, or other supplied invalid values are omitted and set `usage_invalid:"1"`. Derived sums/differences are also validated; missing components prevent derivation rather than becoming zero. Intentionally ignored fields are not validated by these counter calls (`count`, `counter`, `summedInput`, lines 30–51; Codex derivation 185–199).

For transcript timestamps, valid explicitly zoned source timestamps are preserved. Invalid supplied timestamps produce `usage_time_basis:"observed"`, `usage_invalid:"1"`, and neither `ts` nor `usage_source_time`. An absent timestamp has observed basis without that invalid marker (`sourceTime`/`event`, lines 53–118). The store supplies observed time when timestamp remains zero (`AddObserveEvent`, `aide/pkg/store/observe_events.go:155–165`).

**3. Four OpenCode deliveries**

The recorder caches `JSON.stringify([cwd, normalizedEvent])` only after its writer returns true (`createOpenCodeUsageRecorder`, `src/core/model-usage.ts:350–362`).

| Delivery at `/work/a` | Writer called? | Result |
|---|---:|---|
| Original; CLI error | Yes | No persistence under the stated assumption; no cache entry |
| Original; matching acknowledgment | Yes | Original variant persisted and cached |
| Original unchanged | No | Cached acknowledgment suppresses the call |
| Input changes to 12; matching acknowledgment | Yes | Revised variant persisted and cached; normalized input becomes `"32"` |

Thus there are **3 writer attempts, 1 skipped writer call, and 2 persisted distinct variants**.

Delivering the revised part at `/work/b` **does attempt another write**: cwd participates in the in-memory fingerprint. Whether that cwd targets the same durable store is not demonstrated by these files.

The cache retains at most **1,024 successfully acknowledged fingerprints**. Adding entry 1,025 deletes the oldest Set entry. Cache hits do not refresh insertion order, failed writes do not consume entries, and evicted variants can be attempted again. This cache is per recorder instance, not durable storage.

**4. CLI, durable identity, conflicts, and accounting**

The supplied CLI path is `cmdObserveDispatcher → cmdObserveRecord → cmdObserveRecordBatch` (`aide/cmd/aide/cmd_observe.go:18–25, 237–295`). Batch mode opens one backend, scans JSONL with a 1 MiB scanner limit, skips malformed rows/missing kind or name/explicit zero timestamps, transfers fields into `observe.Event`, and calls `backend.Store().AddObserveEvent`. Each nil return increments `recorded`; write failures increment `skipped`. Scanner failure returns an error, potentially after earlier writes.

The supplied concrete Bolt implementation then demonstrates persistence, but **`NewBackend`, backend selection, and any RPC transport connecting that interface call to Bolt are outside this snapshot**. The CLI path shown does not call the package-level `observe.Record`/`ObserveSink` chain.

For model usage:

- **Logical durable identity:** `(SessionID, host, usage_id)`.
- **Fingerprint:** version, usage source, time basis, model/provider, invalid marker, source-time validity when relevant, and each recognized counter’s presence and textual value.
- Actual timestamp values, event ID, cwd, and coverage are not fingerprint components.
- The durable origin combines model-usage identity and fingerprint; `AddObserveEvent` hashes the encoded origin into `observe_origins`.

See `usageIdentity`, `usageFingerprint`, and `usageOrigin`, `aide/pkg/store/token_model_usage.go:36–68`; persistence in `aide/pkg/store/observe_events.go:166–216`.

Equal retries deduplicate to the retained event. An earlier timestamp can replace that event while preserving its ID; otherwise the prior event is returned. A changed fingerprint survives as another retained event so conflict handling can see both revisions. Consequently, **CLI acknowledgment count counts successful `AddObserveEvent` calls, not necessarily new inserts**: deduplicated retries can return nil and be acknowledged.

`TokenStats` feeds all retained observe events into `modelUsage.observe` before applying result-time selection (`aide/pkg/store/token_events.go:153–196`). `observe` groups by logical identity; differing fingerprints mark that identity conflicting. `resultWithSessions` excludes conflicting identities from observations, validity accounting, source groups, and token sums, counting one conflict if any retained variant matches the query (`token_model_usage.go:135–244`).

For the first four deliveries in the otherwise empty store:

```json
{
  "version": 1,
  "observations": 0,
  "conflicts": 1,
  "invalid": 0,
  "by_source": []
}
```

Both variants are individually valid, but their shared identity conflicts. No per-source counter totals are emitted. Selecting `t1` while excluding `t2` **does not remove the conflict**: conflict detection used the full retained store, and the selected original variant is enough to count it. A time filter is not deletion or an historical snapshot of what was known at `t1`.

These `session/model_usage` events do **not** contribute legacy tool-token event counts, text estimates, read totals, or savings. `observeToTokenEvent` rejects session kinds, so `TokenStats` does not pass them through its legacy tally or `TokenAccounting.Add` (`aide/pkg/store/observe_events.go:82–109`; `token_events.go:82–150, 169–174`). Model usage occupies its separate accounting field.

Valid, nonconflicting records establish only the captured host-reported counters, grouped by host/source/model/provider, with per-counter observation counts preserving zero versus absence. They do not establish billing, complete session/provider/subagent usage, savings, or task quality (`aide/pkg/memory/token_model_usage.go:3–28`; `token_accounting.go:8–9, 30–44`).

Verification was static: I checked the current source and supplied normalization, acknowledgment, and host-hook tests. The fixture explicitly describes itself as an incomplete navigation snapshot; I did not execute those tests or infer missing backend behavior.
