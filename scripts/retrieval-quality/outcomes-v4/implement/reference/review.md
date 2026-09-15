# Reference calibration review

Reviewed the complete six-file reference patch against all five implementation
rubric criteria. This is a second source-review pass by the reference author,
not a blinded participant-quality review or a participant-model execution.
All five criteria pass on the inspected reference.

| Criterion | Result and source evidence |
| --- | --- |
| `public_types` | Pass. `files/src/core/model-usage.ts:318` exports the exact request fields; `:324` exports the required discriminated result. `:329` explicitly types the single request and return value. At `:360`, the default injected writer inherits that signature, and the returned recorder explicitly takes `{binary,cwd,part}`. No boolean compatibility union or untyped public API was introduced. |
| `production_consumers` | Pass. The recorder forwards `{binary,cwd,events}` and checks `result.status` at `files/src/core/model-usage.ts:369`. Both actual adapter calls migrate: `files/src/opencode/hooks.ts:463` and `files/src/hooks/session-summary.ts:135`. Stop's existing debug line appends the required status at `:138`; summary processing stays after the usage write. |
| `original_test_consumers` | Pass by source review. `files/src/test/model-usage.test.ts:46` uses structured injected results and its four recorder calls use object requests. `model-usage-recording.test.ts:16` and `:19` assert the new request/result contract. `model-usage-hooks.test.ts:7`, `:79` and `:136` migrate the injected mock and both host-call expectations. Existing test cases, retry/conflict sequence, acknowledgment rejection cases and ignored-event checks remain present; none were deleted or skipped. |
| `bounded_behavior_change` | Pass. The complete patch changes only the writer/recorder API, two adapter calls/logging and three affected original test consumers. Normalizers and transcript scanning are byte-unchanged. JSONL transport, acknowledgment regex/count requirements, timeout, stdio, invalid-part filter, cwd fingerprint, cache capacity/eviction and exception boundary remain intact. No Go or harness files occur in the reference patch. |
| `validation_report` | Pass for calibration notes. The execution results and their limits are recorded below; there is no claim that Vitest or TypeScript type checking executed. |

Calibration checks: all 49 frozen source/test hashes verify. Seed passes 4/4
visible tests and fails 13/20 hidden tests (7 pass). The reference passes 4/4
visible and 20/20 hidden tests with zero skips or todos. Four deliberate mutants
are rejected, including leaving either adapter unmigrated and treating result
objects as booleans; details are in `mutation-checks.json`.

The original three Vitest test files were syntax-checked using Bun's TypeScript
transpiler, **not executed with Vitest or type-checked with TypeScript**. Public
declarations and test-consumer compatibility were inspected in source. The
offline Bun harness executes the real usage implementation and adapter routes
with the documented external-boundary mocks; it does not validate a full aide
application build or a live Go binary.

Re-ran the hardened validator with Python optimization enabled (`python3 -O`):
the same seed/reference counts pass. Its Bun subprocess calls use `rtk proxy`
and a 30-second timeout. Anchored-summary parsing rejects missing or duplicate
required summaries and skipped-test entries; counts are compared numerically.
Applying `reference.patch` to a fresh source snapshot exactly reproduces all
six files under `reference/files`.
