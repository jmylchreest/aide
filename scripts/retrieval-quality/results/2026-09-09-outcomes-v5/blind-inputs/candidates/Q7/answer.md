This trace applies only to the supplied snapshot of commit `58816d991ef7172304f3eef594844d79e995a5e1`. The scenarios below are deductions from that source; no code was changed.

1. **Host entry points and collection**

   OpenCode’s [`createHooks`](source/src/opencode/hooks.ts:154) initializes state and returns `event: createEventHandler(state)`. [`initializeAide`](source/src/opencode/hooks.ts:351) resolves `state.binary`; `skipInit` returns before resolution.

   [`createEventHandler`](source/src/opencode/hooks.ts:432) creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, if `state.binary` exists, it passes `event.properties.part`, binary and cwd to that recorder, then invokes the skill handler. There is no `message.updated` case; it reaches the ignored default.

   [`openCodeUsageEvent`](source/src/core/model-usage.ts:203) requires a `step-finish` part, valid `sessionID`, part `id`, `messageID`, and an object-valued `tokens`. The TS ID guard requires a nonblank string, not exactly `"unknown"`, at most 1,024 characters. `messageID` is required but is not the usage identity or an emitted attribute.

   [`handleMessagePartUpdated`](source/src/opencode/hooks.ts:738) accepts only nonempty text parts and performs skill matching. It does **not** collect model usage; its optional `user_prompt` observation is a different event.

   The recorder invokes [`recordModelUsage`](source/src/core/model-usage.ts:319), which synchronously spawns the supplied binary with **`observe record --stdin`**, in the supplied cwd. Input is newline-terminated JSONL; stdin/stdout/stderr are piped and timeout is 10 seconds. Empty batches return `true` without spawning. Success requires the entire trimmed output to match `Recorded N event(s)`—case-insensitively—with `N` equal to batch length and absent/zero skipped count. Errors or mismatched acknowledgment return `false`; there is no older-CLI fallback.

   Claude Code/Codex [`session-summary.main`](source/src/hooks/session-summary.ts:108) reads JSON stdin, takes `cwd` or process cwd and `session_id` or `"unknown"`. For `hook_event_name === "Stop"` with a truthy `transcript_path`, it resolves a binary, calls `collectTranscriptUsage(path, detectPlatform(), sessionId)`, and calls the shared writer. [`detectPlatform`](source/src/lib/hook-utils.ts:145) selects Codex only when `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. This main function does not invoke the available input-normalization helper.

   **`stop_hook_active` suppresses summary capture only, not usage scanning.** Stop ignores the writer’s boolean and emits `continue: true`. Because scanning has no cursor or acknowledged-record cache, a later Stop can retry failed writes and see delayed transcript flushes, provided those rows remain within its bounded scan.

   [`collectTranscriptUsage`](source/src/core/model-usage.ts:245):

   - Reads only the explicit absolute path; invalid session, relative path, missing/unreadable file or nonregular source produces `unavailable`.
   - Defaults to a **4 MiB tail** and **1,000 matching events**; supplied numeric limits are floored and bounded to 1 through those maxima. It also reads one preceding boundary byte when necessary.
   - Discards a partial first line when the tail starts inside a line. It discards any final line without a terminating newline—even valid JSON—and marks that scan limited.
   - Parses complete lines newest first, retaining the newest matching events, then reverses them into file order. Blank lines are ignored; malformed JSON increments `malformed`; well-formed but irrelevant/session-mismatched rows are ignored.
   - Sets `limited` for tail truncation, short reads, unfinished trailing content, or encountering an additional matching event beyond the event cap.
   - Returns `partial` after a readable scan, including empty/no-match scans. Even `limited: false` does not establish complete session, provider or subagent coverage.

2. **Normalized records**

   All three emit `kind: "session"`, `name: "model_usage"`, `model_usage_version: "1"` and `usage_coverage: "partial"`. Counter values below are **strings in `attrs`**, not top-level `tokens` or `saved`.

   | Input | Session / `usage_id` | Host / `usage_source` | Time and other metadata | All emitted token counters |
   |---|---|---|---|---|
   | OpenCode | `oc-s` / `part-7` | `opencode` / `opencode.step_finish.v1` | `usage_time_basis: "observed"`; no `ts`, source time, model or provider | `uncached_input_tokens: "11"`; `cache_read_input_tokens: "17"`; `cache_write_input_tokens: "3"`; `input_tokens: "31"`; `reported_output_tokens: "13"`; `reasoning_output_tokens: "5"` |
   | Claude | `cc-s` / `cc-r` | `claude-code` / `claude.assistant_usage.v1` | `usage_time_basis: "source"`; both `ts` and `usage_source_time` are `2026-09-08T12:00:00Z`; `model: "m"`; no provider | `uncached_input_tokens: "10"`; `cache_read_input_tokens: "20"`; `cache_write_input_tokens: "0"`; `input_tokens: "30"` |
   | Codex | `cx-s` / `cx-r` | `codex` / `codex.token_usage_record.v1` | Same source timestamp/basis as Claude; no model or provider | `input_tokens: "12"`; `output_tokens: "8"`; `reasoning_output_tokens: "3"`; `total_tokens: "20"`; `cache_read_input_tokens: "5"`; `cache_write_input_tokens: "0"`; `uncached_input_tokens: "7"` |

   These follow [`event`](source/src/core/model-usage.ts:92), [`claudeUsageEvent`](source/src/core/model-usage.ts:121), [`codexUsageEvent`](source/src/core/model-usage.ts:153), and `openCodeUsageEvent`.

   Deliberate omissions:

   - OpenCode omits canonical `output_tokens` and `total_tokens`, including supplied `total: 49`. Raw output and reasoning remain separate; **13 + 5 is not reported as output**.
   - Claude omits supplied `output_tokens: 1`, because assistant output may be a placeholder. It emits no reasoning, reported-output or total counter.
   - Codex ignores cumulative `thread_token_usage.input_tokens: 999`. It emits no `reported_output_tokens`; reasoning is not added again to output or total.

   [`counter` and `summedInput`](source/src/core/model-usage.ts:30) preserve explicit zero as `"0"`. An absent counter is omitted without invalidating the record. A supplied negative, noninteger, unsafe integer, or other invalid value in a mapped counter is omitted and sets `usage_invalid: "1"`. Derived input totals require all three input components; unsafe sums or invalid Codex subtraction also set that flag. Deliberately ignored fields are not validated.

   [`sourceTime`](source/src/core/model-usage.ts:54) validates calendar/time/zone syntax before parsing. An invalid supplied timestamp produces observed basis, no `ts` or `usage_source_time`, and `usage_invalid: "1"`; an absent timestamp produces observed basis without that flag. Go’s [`parseUsage`](source/aide/pkg/store/token_model_usage.go:70) excludes a flagged record from counters entirely. Missing counters remain distinct from reported zero; per-counter observation counts preserve that distinction.

3. **OpenCode retry scenario**

   [`createOpenCodeUsageRecorder`](source/src/core/model-usage.ts:350) caches `JSON.stringify([cwd, normalizedEvent])` only after a successful acknowledgment.

   | Delivery at `/work/a` | Writer called? | Result |
   |---|---|---|
   | Original; CLI error | Yes | Not cached; nothing persisted by assumption |
   | Original; matching acknowledgment | Yes | Original variant persisted and cached |
   | Original unchanged | No | Cached fingerprint skips writer |
   | Input changed to 12; matching acknowledgment | Yes | Revised variant persisted and cached |

   Thus: **3 writer attempts, 1 skipped writer call, 2 distinct persisted variants**. The revision changes `uncached_input_tokens` from `"11"` to `"12"` and derived `input_tokens` from `"31"` to `"32"`.

   Delivering the revision at `/work/b` **does attempt another write**, because cwd participates in the in-memory key. This does not prove which durable store `/work/b` resolves to.

   The cache retains at most **1,024 acknowledged fingerprints**. Adding entry 1,025 evicts the oldest insertion; cache hits do not refresh order. Failed writes consume no entry. An evicted fingerprint can be written again, leaving durable deduplication to Go.

4. **CLI, durability and accounting**

   [`cmdObserveDispatcher`](source/aide/cmd/aide/cmd_observe.go:18) routes `record`; [`cmdObserveRecord`](source/aide/cmd/aide/cmd_observe.go:292) routes `--stdin` to [`cmdObserveRecordBatch`](source/aide/cmd/aide/cmd_observe.go:237). The batch opens one backend, scans JSONL with a 1 MiB scanner maximum, ignores blanks, counts malformed/missing-kind/missing-name/zero-timestamp records as skipped, constructs `observe.Event`, and calls `backend.Store().AddObserveEvent`. Store errors increment skipped; nil errors increment recorded. Scanner failure returns an error rather than the final acknowledgment.

   **Boundary:** the supplied files show that interface call and separately show `BoltStore.AddObserveEvent`; they do not supply `NewBackend`/`Store()` wiring or establish any RPC route between them.

   In the supplied Bolt implementation:

   - [`usageIdentity`](source/aide/pkg/store/token_model_usage.go:36) is **`[session, host, usage_id]`**, here `["oc-s","opencode","part-7"]`. It excludes cwd, source, message ID and time.
   [`usageFingerprint`](source/aide/pkg/store/token_model_usage.go:46) describes the variant: version, source, time basis, model/provider presence and values, invalid flag, and each recognized counter’s presence/value. Source-time validity participates, but the actual timestamp does not; neither does coverage.
   - [`usageOrigin`](source/aide/pkg/store/token_model_usage.go:59) combines identity and fingerprint. [`AddObserveEvent`](source/aide/pkg/store/observe_events.go:140) hashes that origin for durable deduplication, supplies missing ID/time, and honors valid source time. Equal retries retain one event with the earliest retained timestamp. Different fingerprints survive as separate events.
   - A deduplicated retry still returns nil, so the CLI’s **acknowledged count measures accepted lines, not necessarily new insertions**. It also does not certify accounting validity.

   [`TokenStats`](source/aide/pkg/store/token_events.go:153) passes all retained events to the model-usage accumulator before applying query selection. [`modelUsage.observe`](source/aide/pkg/store/token_model_usage.go:135) marks differing fingerprints under one identity as conflicting. [`resultWithSessions`](source/aide/pkg/store/token_model_usage.go:172) excludes that identity’s counters and counts one conflict when any retained variant is selected.

   For the first four deliveries, with both successful variants retained:

   ```json
   {
     "version": 1,
     "observations": 0,
     "conflicts": 1,
     "invalid": 0,
     "by_source": []
   }
   ```

   **Selecting t1 but excluding t2 does not remove the conflict.** Both variants established the conflict before filtering; selecting t1 is enough to report it. No source group or token counters survive.

   These session events do not contribute legacy tool-token event counts, text estimates, or savings: [`observeToTokenEvent`](source/aide/pkg/store/observe_events.go:82) returns nil for session events, bypassing the legacy tally. Accepted, nonconflicted model usage is reported separately under accounting. The [`model-usage contract`](source/aide/pkg/memory/token_model_usage.go:20) explicitly limits it to captured partial host counters: it cannot establish billing, complete session usage, savings or task quality.

Verification was source-level, including the supplied normalization, acknowledgment and hook-entry tests. The fixture is not a complete build; those tests were read, not executed.
