This trace is limited to the supplied snapshot at `58816d991ef7172304f3eef594844d79e995a5e1`. No code was changed.

1. **Host entry points and writing**

`createHooks` initializes state and returns `event: createEventHandler(state)`. The event handler creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, it passes `event.properties.part` to that recorder only when `state.binary` exists, then calls the skill handler. `skipInit` leaves the initial binary null; failed or incomplete initialization can also prevent collection. **`message.updated` is ignored** by the switch. See [createHooks / initializeAide / createEventHandler]( source/src/opencode/hooks.ts:152).

The recorder calls `openCodeUsageEvent`. It requires a `step-finish` part, valid `sessionID`, part `id`, and `messageID`, plus a non-array tokens object. The TypeScript ID check requires a nonblank string, excludes `"unknown"`, and caps length at 1,024. `messageID` is a guard, not the emitted usage identity. The text skill handler accepts only nonempty `text` parts and performs skill matching; it does **not** collect model usage. Its optional reflected `user_prompt` event is separate. See [openCodeUsageEvent]( source/src/core/model-usage.ts:203) and [handleMessagePartUpdated]( source/src/opencode/hooks.ts:738).

For Claude Code/Codex, `session-summary.main` reads and parses JSON stdin, uses `cwd || process.cwd()` and `session_id || "unknown"`, and checks for `hook_event_name === "Stop"` plus a supplied `transcript_path`. If binary discovery succeeds, it calls `collectTranscriptUsage(path, detectPlatform(), sessionId)`, then `recordModelUsage(binary, cwd, usage.events)`. `detectPlatform` returns Codex only for exact `AIDE_PLATFORM=codex`; otherwise it returns Claude Code. This main function does not invoke `normalizeHookInput`. See [Stop main]( source/src/hooks/session-summary.ts:108) and [detectPlatform]( source/src/lib/hook-utils.ts:145).

**`stop_hook_active` suppresses summary capture only, not usage scanning. Stop ignores the writer’s boolean.** There is no scan cursor or successful-write cache on this path, so a later Stop can retry a failed write or capture subsequently flushed rows, provided they remain in the scanned tail.

`collectTranscriptUsage` reads only the explicitly supplied absolute regular-file path, with a valid session. Missing, unreadable, relative, or unsupported sources yield `status: "unavailable"`; a readable scan becomes `"partial"` even with zero matching events or no truncation. It never establishes complete coverage.

- It retains a tail of at most **4 MiB**, plus a one-byte boundary check, and at most **1,000 matching events**. Optional limits are floored and clamped.
- It drops a leading fragment when the tail begins inside a line.
- It drops the unfinished trailing line, even if that fragment is parseable JSON, and marks the scan limited.
- It scans newest complete lines first, then reverses retained events into file order.
- Blank lines are ignored; malformed JSON increments `malformed`; valid but irrelevant or mismatched-session rows are ignored.
- `limited` marks truncation, short reads, unfinished trailing data, or encountering another matching event beyond the event cap. Malformed JSON alone does not set it.

See [collectTranscriptUsage]( source/src/core/model-usage.ts:241).

The shared writer synchronously executes the binary with **`observe record --stdin`**, using the supplied cwd, newline-terminated JSON Lines, piped stdin/stdout/stderr, and a **10-second timeout**. Empty batches return true without spawning. Success requires a case-insensitive acknowledgment matching `Recorded N event(s)`, optionally `, skipped 0`, with exactly the submitted count. CLI errors, missing/mismatched acknowledgments, or nonzero skipped counts return false; there is no fallback writer. See [recordModelUsage]( source/src/core/model-usage.ts:319).

2. **Normalized records**

All three emit `kind: "session"`, `name: "model_usage"`, `model_usage_version: "1"` and `usage_coverage: "partial"`. Token attributes are **decimal strings**, shown numerically below.

| Field | OpenCode | Claude Code | Codex |
|---|---|---|---|
| `session` | `oc-s` | `cc-s` | `cx-s` |
| `usage_id` | `part-7` | `cc-r` | `cx-r` |
| `host` | `opencode` | `claude-code` | `codex` |
| `usage_source` | `opencode.step_finish.v1` | `claude.assistant_usage.v1` | `codex.token_usage_record.v1` |
| `usage_time_basis` | `observed` | `source` | `source` |
| `ts`, `usage_source_time` | Both omitted | Both `2026-09-08T12:00:00Z` | Both `2026-09-08T12:00:00Z` |
| `model` | Omitted | `m` | Omitted |

No record emits `provider` or `usage_invalid` for these inputs.

| Emitted counter | OpenCode | Claude Code | Codex |
|---|---:|---:|---:|
| `input_tokens` | 31 | 30 | 12 |
| `uncached_input_tokens` | 11 | 10 | 7 |
| `cache_read_input_tokens` | 17 | 20 | 5 |
| `cache_write_input_tokens` | 3 | 0 | 0 |
| `output_tokens` | Omitted | Omitted | 8 |
| `reported_output_tokens` | 13 | Omitted | Omitted |
| `reasoning_output_tokens` | 5 | Omitted | 3 |
| `total_tokens` | Omitted | Omitted | 20 |

OpenCode sums the three disjoint input counters, preserves raw output separately, and ignores supplied `tokens.total: 49`. It does not add output and reasoning because their overlap is uncertain. Claude ignores supplied `output_tokens: 1`, which may be a message-start placeholder, and derives no output or total. Codex uses only per-response `payload.usage`; cumulative `thread_token_usage.input_tokens: 999` is ignored. Its explicit cache-write zero permits deriving uncached input as `12 − 5 − 0 = 7`. Reasoning is not added again to output. See the [three normalizers]( source/src/core/model-usage.ts:121).

For counters actually read by an adapter, absence means omission without invalidation; explicit zero emits `"0"`. Negative, fractional, unsafe, or other supplied nonnumeric values omit that counter and set `usage_invalid: "1"`. Derived counters require all relevant components and a safe nonnegative result. Intentionally ignored fields are not validated by these counter calls.

A missing source timestamp uses observed time without invalidation. A supplied invalid timestamp omits both `ts` and `usage_source_time`, uses `usage_time_basis: "observed"`, and sets `usage_invalid: "1"`; the store subsequently supplies an observed timestamp but accounting rejects the invalid record. Validation checks timezone-bearing syntax, calendar dates, clock/offset bounds, and parseability. OpenCode does not pass a timestamp into the event constructor. See [counter / summedInput / sourceTime / event]( source/src/core/model-usage.ts:30).

3. **OpenCode delivery scenario**

The in-memory fingerprint is `JSON.stringify([cwd, normalizedEvent])`. Only acknowledged writes enter its set.

| Delivery | Writer called? | Result |
|---|---|---|
| 1: original at `/work/a` | Yes | CLI error; not cached; no persistence by assumption |
| 2: original at `/work/a` | Yes | Matching acknowledgment; original cached and persisted |
| 3: unchanged at `/work/a` | No | Cached fingerprint skips it |
| 4: input changed to 12 | Yes | New normalized fingerprint; revised variant cached and persisted |

Therefore: **3 writer attempts, 1 skipped delivery, 2 distinct persisted variants**. The revision changes `uncached_input_tokens` to `"12"` and derived `input_tokens` to `"32"`.

Delivering the revision at `/work/b` **does attempt another write**, because cwd participates in the cache key. That does not itself prove a different durable store is selected.

The set retains at most **1,024 acknowledged fingerprints**. Inserting a 1,025th removes the oldest insertion; cache hits do not refresh order. Failed writes occupy no entry. Evicted variants may be attempted again, leaving durable deduplication to storage. See [createOpenCodeUsageRecorder]( source/src/core/model-usage.ts:350).

4. **CLI, durable identity, and accounting**

The supplied CLI path is `cmdObserveDispatcher` → `cmdObserveRecord` → `cmdObserveRecordBatch`. Batch mode opens `NewBackend(dbPath)` once, scans stdin with a 1 MiB scanner token limit, decodes event fields including optional `ts`, and calls `backend.Store().AddObserveEvent`. Blank lines are ignored. Malformed JSON, missing kind/name, explicit zero timestamps, and store errors increment `skipped`; successful store calls increment `recorded`. Scanner errors return an error. See [CLI batch implementation]( source/aide/cmd/aide/cmd_observe.go:237).

The supplied `BoltStore.AddObserveEvent` implementation fills missing IDs/timestamps, honors valid source timestamps, and deduplicates model usage through an origin derived from:

- **Durable identity:** `(session, host, usage_id)`.
- **Fingerprint:** version, source, time basis, model/provider presence and values, invalid marker, and presence/value of each recognized counter. For source timing it includes source-time validity, **not the timestamp value**.
- **Origin:** model-usage marker plus identity plus fingerprint, hashed for the origin index.

Thus absent counters differ from explicit zero. Equal retries share an origin and retain one event, preserving the earliest retained timestamp; conflicting revisions have the same identity but different origins and survive as separate events. See [usageIdentity / usageFingerprint / usageOrigin]( source/aide/pkg/store/token_model_usage.go:36) and [AddObserveEvent]( source/aide/pkg/store/observe_events.go:140).

`TokenStats` first feeds **all retained events** to `modelUsage.observe`, before applying the requested time/session selection to usage results. A differing fingerprint marks the entire identity conflicting. `resultWithSessions` counts a conflict if any retained variant is selected and excludes that identity’s counters. Valid, nonconflicting records are grouped by host/source/model/provider, with per-counter token sums and observation counts; explicit zero contributes an observation. See [TokenStats]( source/aide/pkg/store/token_events.go:67) and [modelUsage.observe / resultWithSessions]( source/aide/pkg/store/token_model_usage.go:135).

For the first four deliveries in the otherwise empty store, the model-usage result is:

```json
{
  "version": 1,
  "observations": 0,
  "conflicts": 1,
  "invalid": 0,
  "by_source": []
}
```

Both variants are individually valid, but their shared identity is conflicting. A query selecting **t1 and excluding t2 still reports one conflict** and no counters, because conflict detection considers all retained variants before filtering. It does not recover the original as an accepted observation.

CLI acknowledgment counts are not insertion counts: an equal retry can make `AddObserveEvent` return nil while inserting nothing, and the CLI still increments `recorded`.

These `session/model_usage` events do **not** become legacy `TokenEvent`s: `observeToTokenEvent` returns nil for session events. Consequently they add no legacy tool-token event counts, calls, text estimates, savings, or `TokenAccounting.Add` legacy/missing-measurement counters. They populate the separate model-usage accounting field. See [observeToTokenEvent]( source/aide/pkg/store/observe_events.go:82) and [accounting contracts]( source/aide/pkg/memory/token_model_usage.go:1).

Accepted partial host counters establish only captured usage in their respective source definitions. They cannot establish billing, complete session/provider/subagent usage, savings, or task quality.

**Boundary and verification:** The snapshot demonstrates the CLI’s backend interface calls and the supplied Bolt storage/accounting implementations. It does not include `NewBackend` implementation or prove a concrete backend/RPC connection between them. I verified the trace against current source and inspected the supplied normalizer, acknowledgment, retry, and hook tests as source evidence. I did not execute those tests: the [fixture README]( source/README.md:1) identifies this as a navigation snapshot rather than a complete build.
