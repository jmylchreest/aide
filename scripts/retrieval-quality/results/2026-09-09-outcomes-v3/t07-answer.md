1. **Trigger and writer path.** `createHooks` installs `createEventHandler`; its `message.part.updated` branch passes `event.properties.part` to the usage recorder when a binary exists. Only a `step-finish` part with valid session, part, and message IDs and a tokens object becomes usage. `message.updated` has no handler and contributes no usage. The recorder calls `recordModelUsage`, which writes newline-delimited JSON through `observe record --stdin` and requires an acknowledgement. Evidence: `src/opencode/hooks.ts:194`, `:432`, `:458`; `src/core/model-usage.ts:203`, `:319`, `:350`.

2. **Persisted observation.** Kind=`session`, name=`model_usage`, session=`session-1`; attributes include version=`1`, host=`opencode`, source=`opencode.step_finish.v1`, usage ID=`step-7`, coverage=`partial`. The reporting identity is `["session-1","opencode","step-7"]`. `message-2` is required for validation but is not persisted or included in that identity. The storage event ID is separate and generated if absent. Evidence: `src/core/model-usage.ts:92`, `:209`, `:217`; `aide/pkg/store/token_model_usage.go:36`; `aide/pkg/store/observe_events.go:160`.

   | Counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   Attributes store these values as strings. Normalized `output_tokens`, `total_tokens`, model, and provider remain absent/unknown, not zero. Time basis is `observed`: this adapter supplies no source timestamp; storage supplies the current time if the timestamp remains empty. Evidence: `src/core/model-usage.ts:33`, `:41`, `:99`, `:217`; `aide/pkg/store/observe_events.go:163`.

3. **Three writer attempts; two stored variants.** Broadcast 1 attempts writing but fails and is not cached. Broadcast 2 retries successfully and is cached. Broadcast 3 is suppressed. Broadcast 4 changes normalized input to 32 and uncached input to 12, producing a new fingerprint and successful write. The bounded, 1,024-entry in-memory cache remembers only acknowledged fingerprints of `[cwd, event]`. Durable deduplication uses usage identity plus fingerprint, survives recorder restarts, preserves changed variants, and retains the earliest timestamp for equivalent records. Evidence: `src/core/model-usage.ts:349`; `aide/pkg/store/token_model_usage.go:46`, `:59`; `aide/pkg/store/observe_events.go:166`, `:188`.

4. **Reporting excludes the conflicted identity’s counters.** It contributes one conflict, zero accepted observations, and no counters or source group. If these are the only records, the model-usage report has `observations=0`, `conflicts=1`, `invalid=0`, `by_source=[]`. All stored usage is examined before date filtering; different fingerprints mark the identity conflicted. A filter selecting only the earlier variant still reports the conflict and cannot restore its counters. Evidence: `aide/pkg/store/token_events.go:157`, `:191`; `aide/pkg/store/token_model_usage.go:150`, `:183`.

5. **These observations do not increase tool-event counts or byte-based token estimates.** Session events do not convert into token events; model usage is reported separately. The counters describe captured, partial host-reported usage and cannot establish complete session consumption, billed cost, savings, or output quality. Sources must remain separate. **Do not add cache counters again to `input_tokens`: they are already included. Do not sum `reported_output_tokens` with `reasoning_output_tokens` when overlap is unknown.** Evidence: `aide/pkg/store/observe_events.go:82`; `aide/pkg/store/token_events.go:93`, `:172`, `:196`; `aide/pkg/memory/token_model_usage.go:3`, `:20`; `src/core/model-usage.ts:41`, `:230`.

No files changed. Verified by tracing the supplied source; indexed hook results were checked against current file contents.
