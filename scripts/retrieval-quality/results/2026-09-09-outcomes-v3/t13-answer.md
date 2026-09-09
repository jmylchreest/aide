1. **Trigger and writer path.** The registered event handler receives `message.part.updated` and, when `state.binary` exists, passes `event.properties.part` to its usage recorder. Only a `step-finish` with valid session, part, and message IDs plus tokens qualifies. The recorder normalizes it and calls `recordModelUsage`, which sends JSONL to `observe record --stdin` and requires an acknowledgement. `message.updated` itself contributes nothing; it falls through the ignored default. Evidence: `src/opencode/hooks.ts:194`, `:432`, `:458`, `:463`; `src/core/model-usage.ts:203`, `:326`, `:332`, `:355`.

2. **Persisted observation.** It has `kind=session`, `name=model_usage`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, version `1`, and `usage_id=step-7`. Its logical identity is `["session-1","opencode","step-7"]`; `message-2` is validated but not persisted in that identity. The storage event ID is generated separately when absent. Normalized counters are:

   | Counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   `output_tokens`, `total_tokens`, model, and provider remain absent/unknown. Coverage is `partial`; time basis is `observed`, with no source timestamp supplied. Storage fills an absent timestamp with `time.Now()`. Evidence: `src/core/model-usage.ts:41`, `:92`, `:203`; `aide/pkg/store/token_model_usage.go:36`; `aide/pkg/store/observe_events.go:160`.

3. **Three writer attempts, two stored variants.** Broadcast 1 fails and is not cached; broadcast 2 succeeds and is cached; broadcast 3 matches the acknowledged fingerprint and skips writing. Broadcast 4 changes normalized input to 32, changing the fingerprint, so it writes successfully. The bounded, in-memory cache contains acknowledged `[cwd,event]` fingerprints only. Durable deduplication combines logical identity with a counter/metadata fingerprint, so identical retries collapse while contradictory variants survive. Evidence: `src/core/model-usage.ts:349`; `aide/pkg/store/token_model_usage.go:46`, `:59`; `aide/pkg/store/observe_events.go:166`, `:188`, `:208`.

4. **One conflict, no counter contribution from this identity.** The report detects differing fingerprints while scanning all stored events, before applying report date filters. It excludes the conflicted identity from observations and counter aggregation. Selecting only the earlier variant’s date still reports that conflict and cannot restore its counters. If these are the only usage records, the result has `conflicts=1`, `observations=0`, `invalid=0`, and empty `by_source`. Evidence: `aide/pkg/store/token_events.go:157`, `:191`; `aide/pkg/store/token_model_usage.go:151`, `:174`, `:183`.

5. **Separate accounting and limited conclusions.** Session usage events do not become legacy token events, so they add neither tool-event counts nor byte-based token estimates; model usage is attached separately. These are captured host counters with partial coverage, not proof of complete session usage, billed cost, savings, or output quality. **Do not add cache counters again to `input_tokens`: they are already included. Do not sum `reported_output_tokens` with `reasoning_output_tokens` when their overlap is unknown.** Evidence: `aide/pkg/store/observe_events.go:84`, `:107`; `aide/pkg/store/token_events.go:93`, `:172`, `:196`; `aide/pkg/memory/token_model_usage.go:3`, `:20`; `src/core/model-usage.ts:41`, `:224`, `:230`.

No files changed. Verified by source inspection; no tests executed.
