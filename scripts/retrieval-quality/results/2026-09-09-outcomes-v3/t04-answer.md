1. **Trigger and route:** The plugin registers `createEventHandler` as its event hook (`src/opencode/hooks.ts:194`). On `message.part.updated`, if a binary is available, it passes `event.properties.part` to the usage recorder (`src/opencode/hooks.ts:429`, `src/opencode/hooks.ts:458`). Only a `step-finish` part with valid session, part and message IDs and a tokens object produces usage (`src/core/model-usage.ts:203`). The recorder calls `recordModelUsage`, which writes newline-delimited JSON through `observe record --stdin` and checks the acknowledgement (`src/core/model-usage.ts:319`, `src/core/model-usage.ts:350`). **`message.updated` itself contributes no usage:** it has no switch case and falls through to “Ignore other events” (`src/opencode/hooks.ts:463`).

2. **Persisted observation:** `kind="session"`, `name="model_usage"`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, version `"1"`, usage ID `step-7`, coverage `partial`. `message-2` is validated but is neither persisted nor part of the usage identity. The logical identity is `["session-1","opencode","step-7"]`; the storage event ID is separately generated when absent (`src/core/model-usage.ts:92`, `src/core/model-usage.ts:203`; `aide/pkg/store/token_model_usage.go:36`; `aide/pkg/store/observe_events.go:160`).

   | Counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   Counters are serialized as string attributes. Normalized `output_tokens`, `total_tokens`, model and provider remain absent/unknown, not zero. Time basis is `observed`: the adapter supplies no source timestamp; the store fills a missing timestamp with `time.Now()` (`src/core/model-usage.ts:33`, `src/core/model-usage.ts:41`, `src/core/model-usage.ts:217`; `aide/pkg/store/observe_events.go:163`).

3. **Three writer attempts, two stored variants.** Broadcast 1 fails and is not cached; broadcast 2 retries successfully and is cached; broadcast 3 is suppressed; broadcast 4 changes the fingerprint and writes successfully, with normalized input now **32**. The in-memory cache fingerprints `[cwd, normalizedEvent]`, retains only acknowledged writes, and evicts the oldest entry above 1,024 entries (`src/core/model-usage.ts:349`). Durable deduplication combines logical identity with a metadata/counter fingerprint, excluding timestamp differences: identical retries reuse the retained observation, while changed counters survive as a separate variant (`aide/pkg/store/token_model_usage.go:44`, `aide/pkg/store/token_model_usage.go:59`; `aide/pkg/store/observe_events.go:166`).

4. **The identity contributes one conflict and no accepted observations or counters.** If these are the only usage records, the usage result has `conflicts=1`, `observations=0`, `invalid=0`, and `by_source=[]`. It does not sum or select either variant. Conflict detection scans stored events before applying report filters; a date filter selecting only the earlier variant still reports the conflict and cannot restore its counters (`aide/pkg/store/token_events.go:153`, `aide/pkg/store/token_events.go:191`; `aide/pkg/store/token_model_usage.go:150`, `aide/pkg/store/token_model_usage.go:183`).

5. **Usage is separate from tool counts and byte estimates.** Session events do not translate into token/tool events and therefore do not enter their tally; model usage is attached separately (`aide/pkg/store/observe_events.go:82`; `aide/pkg/store/token_events.go:93`, `aide/pkg/store/token_events.go:172`, `aide/pkg/store/token_events.go:196`). These counters describe captured, host-reported usage with partial coverage; they establish neither complete session usage, billed cost, savings nor output quality (`aide/pkg/memory/token_model_usage.go:3`, `aide/pkg/memory/token_model_usage.go:20`). **Do not add cache counters again to `input_tokens`: they are already included. Do not sum `reported_output_tokens` with `reasoning_output_tokens` when overlap is unknown** (`src/core/model-usage.ts:41`, `src/core/model-usage.ts:224`).

Verified by source inspection. No files edited; no tests run.
