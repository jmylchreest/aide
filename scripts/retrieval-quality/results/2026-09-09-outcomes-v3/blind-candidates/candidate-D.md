1. **Trigger and route.** The plugin registers `createEventHandler`; `message.part.updated` passes `event.properties.part` to its usage recorder when `state.binary` exists. Only a `step-finish` part with valid session, part and message IDs and a tokens object qualifies. The recorder normalizes it, then `recordModelUsage` sends newline-delimited JSON to the binary’s `observe record --stdin` writer and checks acknowledgment. `message.updated` itself contributes no usage: it falls through the ignored default. Evidence: `src/opencode/hooks.ts:194`, `src/opencode/hooks.ts:432`, `src/opencode/hooks.ts:458`, `src/core/model-usage.ts:203`, `src/core/model-usage.ts:319`.

2. **Persisted observation.** It has `kind="session"`, `name="model_usage"`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, usage ID `step-7`, version `1`, and coverage `partial`. Its logical identity is `["session-1","opencode","step-7"]`; `message-2` is validated but not persisted or used in that identity. The storage event ID is separately generated.
   
   | Counter | Value |
   |---|---:|
   | `input_tokens` | 31 |
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   `output_tokens`, `total_tokens`, model and provider remain absent/unknown. Time basis is `observed`: the adapter supplies no source timestamp; persistence supplies the current time when missing. Evidence: `src/core/model-usage.ts:41`, `src/core/model-usage.ts:92`, `src/core/model-usage.ts:217`, `aide/pkg/store/token_model_usage.go:36`, `aide/pkg/store/observe_events.go:160`.

3. **Three writer attempts; two stored variants.** Broadcast 1 fails and is not cached; 2 retries successfully; 3 is suppressed; 4 changes the normalized input to 32 and uncached input to 12, producing another successful attempt. The in-memory set caches acknowledged fingerprints of `[cwd, normalized event]`, bounded to 1,024 entries. Durable deduplication uses logical identity **plus usage fingerprint**, suppressing identical retries while retaining contradictory variants; repeated timestamps do not create new usage variants. Evidence: `src/core/model-usage.ts:349`, `aide/pkg/store/token_model_usage.go:44`, `aide/pkg/store/token_model_usage.go:59`, `aide/pkg/store/observe_events.go:166`.

4. **The report counts one conflict and excludes both variants’ counters.** This identity contributes no accepted observation or source-counter totals. If these are the only usage records, the usage report has `observations=0`, `conflicts=1`, `invalid=0`, and `by_source=[]`. A date filter selecting only the earlier variant cannot restore its counters: reporting discovers conflicts across stored records before filtering, then counts the conflict if any variant is selected. Evidence: `aide/pkg/store/token_events.go:153`, `aide/pkg/store/token_events.go:191`, `aide/pkg/store/token_model_usage.go:150`, `aide/pkg/store/token_model_usage.go:181`.

5. **These observations do not increase tool-event counts or byte-based token estimates.** Session observations have no legacy token-event mapping; usage is aggregated separately. Counters describe captured host-reported usage with partial coverage, and establish neither complete session usage nor billed cost, savings, or output quality. **Do not add cache counters again to `input_tokens`**: they are already included in its sum. **Do not sum `reported_output_tokens` with `reasoning_output_tokens` when their overlap is unknown**; the adapter deliberately keeps raw output separate. Evidence: `aide/pkg/store/observe_events.go:82`, `aide/pkg/store/token_events.go:93`, `aide/pkg/store/token_events.go:172`, `aide/pkg/memory/token_model_usage.go:3`, `aide/pkg/memory/token_model_usage.go:20`, `src/core/model-usage.ts:41`, `src/core/model-usage.ts:224`.

Verified by source inspection; no files changed or tests run.
