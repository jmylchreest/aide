1. **Trigger and writer path.** `message.part.updated` passes `event.properties.part` to the usage recorder when a binary is available; `message.updated` falls through the ignored-event default (`src/opencode/hooks.ts:429–466`). The converter requires `type="step-finish"`, valid session/part/message IDs, and a tokens object (`src/core/model-usage.ts:203–216`). The recorder normalizes the part and calls `recordModelUsage`, which sends newline-delimited JSON to `observe record --stdin` and checks its acknowledgement (`src/core/model-usage.ts:319–362`).

2. **Persisted observation.** It has `kind="session"`, `name="model_usage"`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, version `"1"`, and usage ID `step-7`. Its logical usage identity is `["session-1","opencode","step-7"]`; `message-2` is validated but does not enter that identity. A storage event ID is generated if absent (`src/core/model-usage.ts:92–118,203–229`; `aide/pkg/store/token_model_usage.go:36–41`; `aide/pkg/store/observe_events.go:160–164`).

   | Counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   Input is the sum of the three input components (`src/core/model-usage.ts:41–51,224–229`). Normalized `output_tokens`, `total_tokens`, model, and provider remain absent/unknown. Output/reasoning overlap is deliberately unresolved (`src/core/model-usage.ts:217–231`). Coverage is `partial`; time basis is `observed`, with no source timestamp supplied. Persistence fills a missing timestamp using `time.Now()` (`src/core/model-usage.ts:99–116,217–222`; `aide/pkg/store/observe_events.go:155–165`).

3. **Three writer attempts; two stored variants.** Broadcast 1 attempts and fails, so it is not cached. Broadcast 2 attempts, succeeds, and is cached. Broadcast 3 is suppressed. Broadcast 4 changes normalized uncached/input counters to 12/32, producing a new fingerprint and successful attempt. The in-memory cache contains only acknowledged fingerprints, includes cwd plus the normalized event, and evicts its oldest entry above 1,024 entries (`src/core/model-usage.ts:349–362`). Durable deduplication combines usage identity with a counter/metadata fingerprint, preserving changed variants while deduplicating identical retries and retaining the earliest timestamp (`aide/pkg/store/token_model_usage.go:44–68`; `aide/pkg/store/observe_events.go:168–216`).

4. **The identity becomes one conflict, contributing no counters or accepted observations.** If these are the only usage records, the usage result is version 1, `observations=0`, `conflicts=1`, `invalid=0`, `by_source=[]`. Reporting reads all stored usage before applying report filters; differing fingerprints mark the identity conflicted. A date filter selecting only the earlier variant still reports the conflict and cannot restore its counters (`aide/pkg/store/token_events.go:153–163,191–196`; `aide/pkg/store/token_model_usage.go:150–163,172–200`).

5. **These observations do not add tool-event counts or byte-based token estimates.** Session observations are excluded from conversion to token events; model usage is attached separately to accounting (`aide/pkg/store/observe_events.go:82–109`; `aide/pkg/store/token_events.go:93–106,172–174,191–198`). The counters describe captured host-reported usage with partial coverage; they cannot establish complete session usage, billed cost, savings, or output quality (`aide/pkg/memory/token_model_usage.go:3–4,20–22`). **Do not add cache counters again to `input_tokens`**, which already includes them. **Do not sum `reported_output_tokens` and `reasoning_output_tokens` when overlap is unknown** (`src/core/model-usage.ts:41–51,224–230`).

No files changed. Verification was a read-only source trace; this snapshot explicitly requires no installation or execution (`README.md:1`).
