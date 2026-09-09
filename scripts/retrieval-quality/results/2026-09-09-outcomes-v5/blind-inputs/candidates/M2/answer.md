This trace applies only to the supplied snapshot pinned to `58816d991ef7172304f3eef594844d79e995a5e1`. No code was changed.

**1. Host entry points and writer**

OpenCode’s `createHooks` initializes state and returns `event: createEventHandler(state)`. That handler creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, it calls the recorder with `event.properties.part` when `state.binary` exists, then calls the separate text-part handler. `message.updated` has no case and is ignored. `handleMessagePartUpdated` requires a nonempty `text` part; it handles skill matching and optionally records a `hook/user_prompt` estimate, not model usage. See [createHooks](source/src/opencode/hooks.ts:154), [createEventHandler](source/src/opencode/hooks.ts:432), and [handleMessagePartUpdated](source/src/opencode/hooks.ts:738).

The usage adapter requires `type: "step-finish"`, valid session/part/message IDs, and a tokens object. Its ID validator requires a nonblank string, not `"unknown"`, at most 1,024 characters. `messageID` is required but is not emitted as usage identity. The recorder skips rejected parts and previously acknowledged normalized snapshots. See [openCodeUsageEvent](source/src/core/model-usage.ts:203).

The shared `recordModelUsage` synchronously spawns the supplied binary with:

```text
observe record --stdin
```

It supplies newline-terminated JSONL, uses the supplied cwd, pipes stdin/stdout/stderr, and sets a 10-second timeout. Empty batches return `true` without spawning. Success requires a whole-output acknowledgment matching `Recorded N event(s)` case-insensitively, with `N` equal to batch length and zero skipped records. Exceptions return `false`; exit zero alone is insufficient. There is no older-CLI fallback. See [recordModelUsage](source/src/core/model-usage.ts:319).

For Claude Code/Codex, Stop’s `main` reads and parses JSON stdin, takes `cwd` or `process.cwd()`, and `session_id` or `"unknown"`. With `hook_event_name === "Stop"`, a supplied `transcript_path`, and a found binary, it calls `collectTranscriptUsage` and then the same writer. `detectPlatform()` selects Codex only when `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. The hook itself does not call the available input-normalization helper. See [Stop main](source/src/hooks/session-summary.ts:108) and [detectPlatform](source/src/lib/hook-utils.ts:145).

**`stop_hook_active` does not suppress usage scanning**: it guards only subsequent summary capture. Stop ignores the writer’s boolean and emits `continue: true`. Scanning has no cursor or acknowledgment cache, so a later Stop can retry a failed write or discover subsequently flushed rows, provided they remain within the bounded scan.

`collectTranscriptUsage` behaves as follows:

- Requires an absolute path, valid session ID, and a regular file, checked both before and after opening. Missing, unreadable, unsupported, or rejected inputs remain `unavailable`.
- Reads only the explicitly supplied file. Defaults and caps are **4 MiB of tail bytes** and **1,000 matching usage events**; supplied ordinary numeric limits are floored and clamped to at least one.
- Discards an incomplete leading line when the tail begins inside a line. Discards any trailing content without a newline, even if that content would parse as JSON.
- Reports `limited` for tail truncation, short reads, unfinished trailing content, or encountering another matching event beyond the event cap.
- Walks complete lines backward, retains the newest matching records, then reverses them into file order. Blank lines are ignored. JSON parse failures increment `malformed`; well-formed unrelated or wrong-session rows are simply ignored.
- A successful scan is always `partial`, including a full-file scan or one producing no events. It cannot establish complete provider/subagent coverage.

See [collectTranscriptUsage](source/src/core/model-usage.ts:245).

**2. Normalized independent records**

All three emit `kind: "session"`, `name: "model_usage"`, `model_usage_version: "1"`, and `usage_coverage: "partial"`. Counter values below are emitted as **decimal strings in `attrs`**, not top-level legacy `tokens`.

| Field | OpenCode | Claude Code | Codex |
|---|---|---|---|
| Session | `oc-s` | `cc-s` | `cx-s` |
| `usage_id` | `part-7` | `cc-r` | `cx-r` |
| `host` | `opencode` | `claude-code` | `codex` |
| `usage_source` | `opencode.step_finish.v1` | `claude.assistant_usage.v1` | `codex.token_usage_record.v1` |
| `usage_time_basis` | `observed` | `source` | `source` |
| `ts` and `usage_source_time` | Both absent | Both `2026-09-08T12:00:00Z` | Both `2026-09-08T12:00:00Z` |
| Model/provider | Both absent | `model: "m"`; provider absent | Both absent |
| `input_tokens` | `31` | `30` | `12` |
| `uncached_input_tokens` | `11` | `10` | `7` |
| `cache_read_input_tokens` | `17` | `20` | `5` |
| `cache_write_input_tokens` | `3` | `0` | `0` |
| `output_tokens` | Absent | Absent | `8` |
| `reported_output_tokens` | `13` | Absent | Absent |
| `reasoning_output_tokens` | `5` | Absent | `3` |
| `total_tokens` | Absent | Absent | `20` |

OpenCode sums the three disjoint input counters. It deliberately ignores `tokens.total: 49` and keeps raw output under `reported_output_tokens` because output/reasoning overlap varies across versions. Claude deliberately ignores `output_tokens: 1`, which can be a message-start placeholder. Codex ignores cumulative `thread_token_usage.input_tokens: 999`; its uncached input is `12−5−0=7`. Nothing adds reasoning to raw output. See [Claude adapter](source/src/core/model-usage.ts:121), [Codex adapter](source/src/core/model-usage.ts:153), and [OpenCode adapter](source/src/core/model-usage.ts:203).

For counters the adapters consume:

- Absent values remain absent, without automatically invalidating the event.
- Explicit zero becomes `"0"`, preserving a measured zero.
- Negative, noninteger, unsafe, or otherwise invalid supplied values are omitted and set `usage_invalid: "1"`.
- Derived inputs require all components; unsafe sums or invalid differences set the invalid marker.

Invalid supplied source timestamps produce no `ts` or `usage_source_time`, select `usage_time_basis: "observed"`, and set `usage_invalid: "1"`. Missing timestamps select observed time without that marker. Valid timestamps retain their original string, including offsets and precision. These rules apply to timestamps passed into the common event builder; the OpenCode adapter supplies none. See [counter helpers and event builder](source/src/core/model-usage.ts:30).

The Go validator excludes invalid records from counter aggregation; it also rejects inconsistent totals, invalid counter strings, empty counter sets, and unsafe values. Explicit zero contributes a counter observation with zero tokens; absence contributes no observation for that counter. See [parseUsage](source/aide/pkg/store/token_model_usage.go:70).

**3. Four OpenCode deliveries**

The recorder caches `JSON.stringify([cwd, normalizedEvent])` only after the writer returns `true`. See [createOpenCodeUsageRecorder](source/src/core/model-usage.ts:350).

| Delivery at `/work/a` | Writer attempt? | Result |
|---|---|---|
| Original; CLI error | Yes | Not cached; no persistence by assumption |
| Original; matching acknowledgment | Yes | Original variant persisted and cached |
| Original unchanged | No | Previously acknowledged snapshot skipped |
| Input changed to 12; matching acknowledgment | Yes | Revised variant persisted and cached |

Therefore: **three writer attempts, one skipped delivery, two persisted distinct variants**. The revision changes both `uncached_input_tokens` from `11` to `12` and derived `input_tokens` from `31` to `32`.

Delivering the revised part at **`/work/b` does attempt a write**, because cwd participates in the in-memory fingerprint. This establishes an attempt, not which durable backend/store that cwd selects.

The cache retains at most **1,024 acknowledged fingerprints**. Adding a 1,025th evicts the oldest insertion; cache hits do not refresh insertion order. Evicted snapshots can be attempted again, leaving durable deduplication to the store.

**4. CLI, durable deduplication, and accounting**

The demonstrated CLI route is `cmdObserveDispatcher` → `cmdObserveRecord` → `cmdObserveRecordBatch` for `--stdin`. The batch opens one backend, scans stdin with a 1 MiB scanner-token limit, decodes each nonblank row, and calls `backend.Store().AddObserveEvent`. Malformed rows, missing kind/name, explicitly zero timestamps, and store errors increment `skipped`. Scanner errors abort with an error; earlier writes are not shown being rolled back. See [CLI dispatcher](source/aide/cmd/aide/cmd_observe.go:18) and [batch implementation](source/aide/cmd/aide/cmd_observe.go:237).

**Boundary:** `NewBackend`, its concrete store selection, and any RPC implementation are absent from the supplied snapshot. Thus the CLI interface call and the supplied `BoltStore` behavior are demonstrated separately; an actual backend/RPC connection between them is not established here.

Within the supplied store:

- **Usage identity** is `(session, host, usage_id)`. It excludes cwd, source, model, and timestamps.
- **Fingerprint** includes version, source, time basis, model/provider values and presence, invalid marker, and every recognized counter’s presence/value. Source-time validity participates, but the timestamp value itself does not.
- `usageOrigin` combines identity and fingerprint. `AddObserveEvent` hashes that origin into its origin index. Equal retries reuse the retained event, preserving the earliest timestamp; a conflicting fingerprint gets a separate stored event.

See [identity, fingerprint, and origin](source/aide/pkg/store/token_model_usage.go:36) and [AddObserveEvent](source/aide/pkg/store/observe_events.go:140). Missing event IDs receive ULIDs; missing timestamps receive `time.Now()`. Valid source-time attributes override event timestamps.

`TokenStats` first feeds **all retained events** into `modelUsage.observe`, before applying the requested usage time window. Different fingerprints under one identity mark that identity conflicted. `resultWithSessions` counts a selected conflict once and excludes all its variants from observations and counters. See [TokenStats](source/aide/pkg/store/token_events.go:153) and [modelUsage aggregation](source/aide/pkg/store/token_model_usage.go:135).

For the first four deliveries in the stipulated otherwise-empty store:

```json
{
  "version": 1,
  "observations": 0,
  "conflicts": 1,
  "invalid": 0,
  "by_source": []
}
```

There are two stored variants but one conflicted identity. Neither contributes counters. A query selecting `t1` and excluding `t2` **still reports that conflict**, because conflict detection sees both retained variants first, and selecting either variant makes the conflict relevant.

CLI `recorded` counts successful `AddObserveEvent` returns, including deduplicated retries. Consequently, acknowledgment count is not necessarily newly inserted event count.

Finally, `observeToTokenEvent` returns `nil` for session events. These usage events therefore do not increase legacy token event counts, tool-call counts, text estimates, or savings; model usage is attached separately to accounting. See [observeToTokenEvent](source/aide/pkg/store/observe_events.go:82) and [model-usage accounting types](source/aide/pkg/memory/token_model_usage.go:3).

Valid, unconflicted records establish only the captured host-reported counters and their per-counter observation coverage. They do not establish billing, complete session usage, savings, or task quality.

Verification was source-based: indexed hook/CLI excerpts were checked against current files, calculations were checked against adapter logic, and the supplied normalization, retry, acknowledgment, and hook tests were inspected. Tests were not executed.
