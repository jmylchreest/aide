1. **Trigger and writer path.** The plugin’s `event` hook uses `createEventHandler`; `message.part.updated` passes `event.properties.part` to the usage recorder when a binary exists. Only `step-finish` parts with valid session, part and message IDs and a tokens object normalize successfully. The recorder calls `recordModelUsage`, which sends newline-delimited JSON to `<binary> observe record --stdin` and requires an acknowledgement. `message.updated` has no handler and contributes no usage. Evidence: `src/opencode/hooks.ts:194`, `src/opencode/hooks.ts:432`, `src/opencode/hooks.ts:458–465`; `src/core/model-usage.ts:203–222`, `src/core/model-usage.ts:319–359`.

2. **Persisted observation.** It has `kind="session"`, `name="model_usage"`, session `session-1`, host `opencode`, source `opencode.step_finish.v1`, version `1`, usage ID `step-7`, and coverage `partial`. Its logical usage identity is `["session-1","opencode","step-7"]`; `message-2` is validated but is not included in that identity or emitted attributes. The database event ID is separate; the store generates a ULID if absent. Evidence: `src/core/model-usage.ts:99–117`, `src/core/model-usage.ts:210–229`; `aide/pkg/store/token_model_usage.go:36–41`; `aide/pkg/store/observe_events.go:160–165`.

   | Normalized counter | Value |
   |---|---:|
   | `uncached_input_tokens` | 11 |
   | `cache_read_input_tokens` | 17 |
   | `cache_write_input_tokens` | 3 |
   | `input_tokens` | 31 |
   | `reported_output_tokens` | 13 |
   | `reasoning_output_tokens` | 5 |

   Counter attributes are persisted as decimal strings. `output_tokens`, `total_tokens`, model and provider remain absent/unknown. Raw output is kept separate because output/reasoning overlap changed across OpenCode versions. No source timestamp is passed, so the basis is `observed`; the store fills a missing timestamp with `time.Now()`. Evidence: `src/core/model-usage.ts:33–50`, `src/core/model-usage.ts:104–115`, `src/core/model-usage.ts:217–231`; `aide/pkg/store/observe_events.go:155–165`.

3. **Three writer attempts, two stored variants.** Broadcast 1 attempts and fails, so it is not cached. Broadcast 2 attempts, succeeds and is cached. Broadcast 3 is skipped. Broadcast 4 produces different normalized counters—uncached input `12`, total input `32`—and succeeds. The in-memory cache fingerprints `[cwd, normalized event]`, records only acknowledged successes and evicts its oldest entry above 1,024 entries. Evidence: `src/core/model-usage.ts:349–361`.

   Durable deduplication combines logical usage identity with a fingerprint of metadata and counter presence/values. Identical retries reuse the stored observation; changed counters survive as another variant. Observed timestamps do not distinguish variants, and duplicate handling retains the earliest timestamp. Evidence: `aide/pkg/store/token_model_usage.go:44–68`; `aide/pkg/store/observe_events.go:176–216`.

4. **The report counts one conflict and excludes both variants’ counters.** For this identity alone, model usage reports `conflicts=1`, `observations=0`, `invalid=0`, and no source group. It neither sums variants nor selects the newest. All stored observations enter conflict detection before report date filtering. Selecting only the earlier variant’s date therefore still reports the conflict and cannot restore its counters. Evidence: `aide/pkg/store/token_events.go:153–162`, `aide/pkg/store/token_events.go:191–196`; `aide/pkg/store/token_model_usage.go:150–163`, `aide/pkg/store/token_model_usage.go:172–205`.

5. **Separate accounting, limited conclusions.** Session observations do not convert to legacy tool/token events, so they do not increment tool-event counts or enter byte-based token estimates. Model usage is attached separately to accounting. The counters describe captured host-reported usage with partial coverage; they cannot establish complete session usage, billed cost, savings or output quality. Missing counters remain distinct from reported zero. Evidence: `aide/pkg/store/observe_events.go:82–109`; `aide/pkg/store/token_events.go:93–106`, `aide/pkg/store/token_events.go:172–174`, `aide/pkg/store/token_events.go:191–196`; `aide/pkg/memory/token_model_usage.go:3–4`, `aide/pkg/memory/token_model_usage.go:20–22`.

No files changed. Verified by tracing the supplied source; no execution was needed for this read-only snapshot (`README.md:1`).
