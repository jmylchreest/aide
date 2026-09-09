This trace applies only to the supplied snapshot pinned to `58816d991ef7172304f3eef594844d79e995a5e1`. No code was changed.

1. **Host entry points and writer**

   OpenCode’s `createHooks` initializes state and exposes `event: createEventHandler(state)` ([hooks.ts:154](source/src/opencode/hooks.ts:154)). Initialization resolves `state.binary`; `skipInit` returns before that resolution ([hooks.ts:351](source/src/opencode/hooks.ts:351)).

   `createEventHandler` creates one `createOpenCodeUsageRecorder` closure. On **`message.part.updated`**, it passes `event.properties.part` to that recorder if `state.binary` exists, then separately invokes `handleMessagePartUpdated`. **`message.updated` has no case and is ignored.** The text-part handler returns unless the part is nonempty text; it performs skill/prompt processing, not model-usage collection ([hooks.ts:432](source/src/opencode/hooks.ts:432), [hooks.ts:738](source/src/opencode/hooks.ts:738)).

   `openCodeUsageEvent` requires an object with `type:"step-finish"`, valid `sessionID`, `id`, and `messageID`, and an object-valued `tokens`. Its ID validator requires a nonblank string, not exactly `"unknown"`, with length ≤1,024. `messageID` is a guard, not the emitted usage identity. The recorder normalizes the part and writes unless that normalized event plus cwd was previously acknowledged ([model-usage.ts:17](source/src/core/model-usage.ts:17), lines 203–231 and 349–362).

   `recordModelUsage` synchronously spawns the selected binary with **`observe record --stdin`**, using the supplied cwd, newline-delimited JSON plus a final newline, a 10-second timeout, and piped stdin/stdout/stderr. Empty batches return `true` without spawning. Nonempty batches succeed only when stdout matches the acknowledgment format, its recorded count equals the batch length, and its skipped count is absent or zero. Execution errors return `false`; there is no older-CLI fallback ([model-usage.ts:319](source/src/core/model-usage.ts:319)).

   For Claude Code/Codex, `session-summary.main` reads stdin and directly parses the JSON. It uses `cwd || process.cwd()` and `session_id || "unknown"`. Usage scanning requires `hook_event_name === "Stop"`, a truthy explicit `transcript_path`, and a found binary. `detectPlatform()` selects Codex only when `AIDE_PLATFORM === "codex"`; otherwise it selects Claude Code. The Stop implementation does not call the separately defined input-normalization helper ([session-summary.ts:108](source/src/hooks/session-summary.ts:108), [hook-utils.ts:145](source/src/lib/hook-utils.ts:145)).

   **`stop_hook_active` does not suppress usage scanning.** It guards only subsequent summary capture. Stop passes collected events to the shared writer but **ignores its boolean** and continues. The scanner has no cursor or acknowledgment cache, so a later Stop can retry failed writes or discover delayed transcript flushes, provided those records remain within its bounded tail ([session-summary.ts:125](source/src/hooks/session-summary.ts:125), [model-usage.ts:241](source/src/core/model-usage.ts:241)).

   `collectTranscriptUsage` reads only the supplied absolute regular-file path, checked before and after opening. Invalid session/path, missing/unreadable files, or unsupported file types yield `status:"unavailable"`. A successful scan yields **`"partial"` even if every available line was read**, because file completeness cannot establish provider/subagent coverage.

   Its defaults and caps are **4 MiB of tail data and 1,000 matching events**; ordinary numeric overrides are floored and clamped to at least one. It drops a cut initial line unless the tail begins at a newline boundary. It ignores an unfinished final line—even valid JSON without its terminating newline—and marks that limitation. It scans complete lines newest-first, skips blanks, counts JSON parse failures as malformed, and silently ignores well-formed nonmatching rows. It returns retained events in original order. Byte truncation, short reads, unfinished trailing content, or encountering an additional matching event beyond the cap set `limited:true`. Malformed counts cover only processed lines ([model-usage.ts:245](source/src/core/model-usage.ts:245)).

2. **Normalized independent records**

   All three emit `kind:"session"`, `name:"model_usage"`, `model_usage_version:"1"` and `usage_coverage:"partial"`. Counters below are **string-valued attributes**. None emits top-level `tokens` or `saved` ([model-usage.ts:92](source/src/core/model-usage.ts:92)).

   | Record | Session / `usage_id` | Host / source | Time |
   |---|---|---|---|
   | OpenCode | `oc-s` / `part-7` | `opencode` / `opencode.step_finish.v1` | `usage_time_basis:"observed"`; no `ts` or `usage_source_time`; store assigns observation time |
   | Claude | `cc-s` / `cc-r` | `claude-code` / `claude.assistant_usage.v1` | `"source"`; both `ts` and `usage_source_time` equal `2026-09-08T12:00:00Z` |
   | Codex | `cx-s` / `cx-r` | `codex` / `codex.token_usage_record.v1` | `"source"`; both timestamp fields equal `2026-09-08T12:00:00Z` |

   | Record | All emitted counters | Omitted counters |
   |---|---|---|
   | OpenCode | `uncached_input_tokens:"11"`, `cache_read_input_tokens:"17"`, `cache_write_input_tokens:"3"`, `input_tokens:"31"`, `reported_output_tokens:"13"`, `reasoning_output_tokens:"5"` | `output_tokens`, `total_tokens`; supplied `total:49` is ignored |
   | Claude | `uncached_input_tokens:"10"`, `cache_read_input_tokens:"20"`, `cache_write_input_tokens:"0"`, `input_tokens:"30"` | Supplied `output_tokens:1` is deliberately ignored because it can be a message-start placeholder; no output, reasoning, total, or reported-output counter |
   | Codex | `input_tokens:"12"`, `output_tokens:"8"`, `reasoning_output_tokens:"3"`, `total_tokens:"20"`, `cache_read_input_tokens:"5"`, `cache_write_input_tokens:"0"`, `uncached_input_tokens:"7"` | Cumulative `thread_token_usage.input_tokens:999` is ignored; no `reported_output_tokens` |

   Claude additionally emits `model:"m"`; no supplied record yields a provider, and the other two yield no model. OpenCode keeps raw output separate because output/reasoning overlap varies: **13 and 5 must not be added**. Codex preserves the supplied total of 20; it does not add reasoning again. These mappings are implemented in `claudeUsageEvent`, `codexUsageEvent`, and `openCodeUsageEvent` ([model-usage.ts:121](source/src/core/model-usage.ts:121)).

   For inspected counters, absence means omission without an invalid marker. Explicit zero is retained as `"0"` and counts as an observation. Negative, noninteger, unsafe, or otherwise nonnumeric supplied values are omitted and set `usage_invalid:"1"`. Input sums require all three components; an unsafe sum is omitted and marked invalid. Codex derives uncached input only when input and both cache fields exist, and marks an invalid subtraction. Deliberately ignored fields are not validated by these counter calls ([model-usage.ts:30](source/src/core/model-usage.ts:30)).

   Invalid supplied timestamps produce no `ts` or `usage_source_time`, use `"observed"`, and set `usage_invalid:"1"`; an absent timestamp uses observed time without that marker. Calendar-valid zoned timestamps preserve their original precision and offset. Durable accounting rejects marked-invalid records rather than treating their surviving counters as valid partial observations ([model-usage.ts:53](source/src/core/model-usage.ts:53), [token_model_usage.go:70](source/aide/pkg/store/token_model_usage.go:70)).

3. **Recorder delivery scenario**

   | Delivery at `/work/a` | Writer attempted? | Result |
   |---|---|---|
   | 1: original, CLI error | Yes | No persistence by assumption; no cache entry |
   | 2: original, matching acknowledgment | Yes | Original variant persisted and cached |
   | 3: unchanged | No | Skipped because cached |
   | 4: input changed to 12, matching acknowledgment | Yes | Revised variant persisted and cached; derived input becomes 32 |

   Thus: **3 writer attempts, 1 skipped delivery, 2 distinct persisted variants**. Changing input changes the normalized event fingerprint.

   Delivering the revision at `/work/b` **does attempt another write**, because the in-memory key is `JSON.stringify([cwd, next])`. That says nothing about which durable store `/work/b` selects. The closure retains at most **1,024 acknowledged fingerprints**; inserting the 1,025th evicts the oldest insertion. Cache hits do not refresh order, failures are not cached, and evicted variants may be attempted again ([model-usage.ts:349](source/src/core/model-usage.ts:349)).

4. **CLI, persistence, and accounting**

   The supplied CLI dispatcher routes `record` to `cmdObserveRecord`, whose `--stdin` branch invokes `cmdObserveRecordBatch`. It opens one backend, scans JSON Lines with a 1 MiB scanner limit, skips malformed/invalid rows and failed adds, preserves supplied nonzero timestamps, and increments `recorded` whenever `backend.Store().AddObserveEvent` returns nil. Scanner errors return an error instead of the final acknowledgment ([cmd_observe.go:18](source/aide/cmd/aide/cmd_observe.go:18), lines 218–295).

   Durable **identity** is the tuple **`[sessionID, host, usage_id]`**. It excludes cwd, OpenCode message ID, and timestamp. The **fingerprint** covers version, usage source, time basis, model/provider values and presence, invalid marker, and presence/value of all eight recognized counters. For source time it includes timestamp validity, not the timestamp’s actual value. Coverage is not fingerprinted ([token_model_usage.go:36](source/aide/pkg/store/token_model_usage.go:36)).

   `usageOrigin` combines identity and fingerprint. `BoltStore.AddObserveEvent` hashes that origin into `observe_origins`. Equal retries reuse the retained event and preserve its earliest time, updating it if an earlier observation arrives. Different fingerprints survive as separate events, allowing conflict detection. The method also supplies missing IDs/timestamps and prefers valid `usage_source_time` for source-timed usage ([observe_events.go:140](source/aide/pkg/store/observe_events.go:140)). Therefore **CLI acknowledgment counts successful add operations, not necessarily newly inserted events**: a deduplicated retry can return nil and count as recorded.

   `BoltStore.TokenStats` feeds **all retained events** to `modelUsage.observe` before applying the requested usage window through `resultWithSessions`. Equal fingerprints collapse to one observation at the earliest retained time; differing fingerprints mark the identity conflicted. Conflicts contribute neither valid observations nor counters ([token_events.go:153](source/aide/pkg/store/token_events.go:153), [token_model_usage.go:135](source/aide/pkg/store/token_model_usage.go:135)).

   For only the first four deliveries in the otherwise empty store, the final result is:

   ```json
   {
     "version": 1,
     "observations": 0,
     "conflicts": 1,
     "invalid": 0,
     "by_source": []
   }
   ```

   Both variants are individually valid, but their shared identity conflicts. **Selecting t1 while excluding t2 does not remove the conflict**: conflict detection already examined both retained variants, and a conflict is reported whenever any retained representative intersects the query ([token_model_usage.go:172](source/aide/pkg/store/token_model_usage.go:172)).

   Session events return nil from `observeToTokenEvent`, so these records do not increase legacy token-event counts, tool calls, text estimates, or savings. They are exposed separately through `Accounting.ModelUsage` ([observe_events.go:82](source/aide/pkg/store/observe_events.go:82), [token_events.go:191](source/aide/pkg/store/token_events.go:191)). Valid, nonconflicting records establish only the captured host-reported counters and each counter’s observation count; they do not establish complete session usage, billing, savings, or task quality ([memory/token_model_usage.go:3](source/aide/pkg/memory/token_model_usage.go:3)).

   **Boundary and verification:** the snapshot demonstrates the CLI’s backend-interface call and the concrete BoltStore implementation separately. It does not supply `NewBackend`, backend store selection, or an RPC connection establishing that runtime link. I verified the trace against current source and the supplied test assertions; I did not execute tests because this fixture is explicitly a navigation snapshot, not a complete build.
