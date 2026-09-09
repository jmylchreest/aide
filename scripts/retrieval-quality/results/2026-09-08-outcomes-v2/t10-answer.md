1. **Trigger and path.** `message.part.updated` passes `event.properties.part` to the usage recorder when `state.binary` exists. Only a `step-finish` part with valid session, part and message IDs and a tokens object is accepted. The recorder defaults to `recordModelUsage`, which sends newline-delimited JSON to `aide observe record --stdin`. `message.updated` falls through the handler’s default branch and contributes no usage. Evidence: `src/opencode/hooks.ts:194`, `:432`, `:458–465`; `src/core/model-usage.ts:203–222`, `:319–339`, `:350–358`.

2. **Persisted observation.** It has `kind="session"`, `name="model_usage"`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, version `"1"`, and `usage_id="step-7"`. The logical identity is `["session-1","opencode","step-7"]`; `message-2` is validated but is not persisted as part of that identity. The storage event ID is generated separately if absent. Evidence: `src/core/model-usage.ts:92–118`, `:209–222`; `aide/pkg/store/token_model_usage.go:36–41`; `aide/pkg/store/observe_events.go:160–165`.

   | Counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   Counter attributes are decimal strings. `output_tokens` and `total_tokens` remain absent, as do model and provider metadata. Raw output is kept separate because output/reasoning overlap changed across OpenCode versions. Time basis is `"observed"` with partial coverage: this adapter supplies no source timestamp, and storage supplies `time.Now()` if the event timestamp is zero. Evidence: `src/core/model-usage.ts:30–50`, `:99–116`, `:217–231`; `aide/pkg/store/observe_events.go:163–165`.

3. **Three writer attempts; two stored variants.** Broadcast 1 attempts and fails, so nothing enters the acknowledgement cache. Broadcast 2 attempts successfully and is cached. Broadcast 3 is skipped. Broadcast 4 changes the normalized input counters to 12 uncached / 32 total input, producing a different fingerprint and another successful attempt. The cache keys `JSON.stringify([cwd, next])`, adds only acknowledged writes, and evicts the oldest entry above 1,024 entries. Evidence: `src/core/model-usage.ts:350–361`.

   Durable deduplication uses logical identity **plus the usage fingerprint**, preserving changed variants while deduplicating equal retries independently of the in-memory cache. Timestamp differences do not create usage variants; equivalent records retain the earliest timestamp. Evidence: `aide/pkg/store/token_model_usage.go:44–68`; `aide/pkg/store/observe_events.go:166–216`.

4. **The changed identity becomes one conflict, contributing no counters.** For this identity alone, the report has `conflicts=1`, `observations=0`, `invalid=0`, and no `by_source` entry. It neither sums the variants nor chooses the latest. A date filter selecting only the earlier variant cannot restore its counters: the report scans all stored observations to establish conflicts before applying date selection; selecting either conflicting variant counts the conflict. Evidence: `aide/pkg/store/token_events.go:153–162`, `:191–196`; `aide/pkg/store/token_model_usage.go:150–163`, `:181–200`.

5. **These observations do not increase tool-event counts or byte-based token estimates.** Session events are excluded from conversion to token events; model usage is aggregated separately into `Accounting.ModelUsage`. Evidence: `aide/pkg/store/observe_events.go:82–109`; `aide/pkg/store/token_events.go:93–97`, `:172–174`, `:191–196`.

   The counters establish only captured host-reported usage, with absent counters distinguished from reported zero. Coverage is partial; they do not establish complete session totals, billed cost, savings, or output quality. Evidence: `aide/pkg/memory/token_model_usage.go:3–4`, `:20–22`.

No files changed. Verified by tracing the supplied adapter, writer, persistence, and reporting code; no execution was required for this read-only snapshot (`README.md:1`).
