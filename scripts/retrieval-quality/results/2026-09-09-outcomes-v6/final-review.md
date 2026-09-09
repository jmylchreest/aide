# Independent final evidence review

Reviewer: `/root/implementation_value_review`. Reviewed completed v6 captures, frozen protocol/launch chain, raw provenance, root-authored trace annotations and derived output boundaries, independent source-grade integration, paired arithmetic and evaluator-overhead accounting. This audit launched no participants, executed no captured command or candidate code, and changed only this review file. Source grades are the separate blinded reviewer's judgments, not a new grade assigned here.

**Disposition: no substantive unresolved error found in the completed four-trial evidence and report accounting.** All four source grades are now present, all mandatory functional/integrity checks pass, and both pairs meet the frozen comparison gates. Lower input in the assisted arms is an observed condition difference; neither assisted participant invoked aide, so these results demonstrate no tool-offloading savings.

## Protocol, identity and evidence integrity

- `outcomes-v6/run.py verify` passes, including its frozen v5/v4 corpus/helper chain. The v6 freeze commit is `dc5fc4b`, timestamped 2026-09-09 18:17:31 UTC, preceding participant launch. No reused helper was edited by this audit.
- For each of t01–t04, independently recomputing the raw-log SHA-256 matches provenance, deterministic collector recollection equals the saved capture, and deriving response evidence equals the archived response associations. Actor identities are distinct, actual runtime dimensions match (Codex 0.153.4, openai, gpt-6-astra, high), and execution is sequential.
- Regenerated full launch prompts equal archived message text, prelaunch text, recorded launch text and prepared prompt files; all associated hashes agree. Initial inventory equals seeded inventory. Final candidate files and preserved changed-file copies match the provenance snapshot hashes.
- Submitted-message records appropriately keep independent logged plaintext equality unknown. Encrypted launch content and implicit host context are not established by comparing controller archives. `fork_turns=none` is not proof of cleared provider cache or absent ambient guidance.

Evidence: [frozen protocol](../../outcomes-v6/protocol.json), [execution completion](execution-complete.json), each `tNN-{capture,provenance,prelaunch,launch,submitted-message-check,response-evidence}.json`, and [ledger](ledger.json).

## Trace scope, output boundaries and overlap

All outer calls are execution-tool calls. Reviewed shell inputs use the RTK prefix and remain inside the authorized fixture apart from the mandatory RTK bootstrap. Source edits affect the six permitted TypeScript files and participant-authored scratch tests. Inline Python uses local Path reads/writes/string transformations; t01 also removes its own scratch file. No bridge/native aide invocation, reindex, external source, grader/reference read, internet, commit or subagent action appears in the participant traces. Bridge evidence contains setup only for every trial; uptake is 0/2 assisted.

Archived tool-result objects equal the raw tool-output events. Independent decoding confirms every recorded output boundary, UTF-8 byte length and exit code; no truncation marker or unattributable source output was found. Source totals exclude execution wrappers, bootstrap, discovery, edits and verification output:

| Trial | Source-output UTF-8 bytes | Responses | First edit response |
| --- | ---: | ---: | ---: |
| t01 ordinary | 50,108 | 8 | 3 |
| t02 assisted | 40,061 | 6 | 3 |
| t03 assisted | 56,091 | 7 | 4 |
| t04 ordinary | 52,295 | 8 | 4 |

First-edit boundaries are supported by captured edit commands and raw response associations. Zero unchanged implementation-body overlap is supported under the frozen definition and its post-edit-verification exclusion. Initial caller-match excerpts are followed by fuller source reads; later body reads follow edits or supply new context. This is not a claim that every returned line is novel, that all navigation was necessary, or that an alternative workflow would avoid a response.

Evidence: [t01 trace](t01-trace-review.json), [t02 trace](t02-trace-review.json), [t03 trace](t03-trace-review.json), [t04 trace](t04-trace-review.json), corresponding annotations/captures/tool-results, [decoder](../2026-09-09-outcomes-v5/trace_bodies.py), [audit derivation](audit_trace.py), [response association](response_evidence.py).

## Quality and verification boundaries

All four independent reviews satisfy the five unchanged criteria, in addition to each final candidate's 4 visible and 20 hidden controller checks and integrity pass. More scratch tests do not raise the mandatory grade. t01 executed seven temporary checks, removed their file, then reran the four visible tests; their source remains in capture input rather than the final snapshot. The blind reviewer correctly leaves their detailed assertions unknown in the supplied blind package. t02 and t03 retain four and six scratch checks and have final post-edit runtime runs. t04 executed seven scratch checks before final formatting, then parsed six migrated files with `Bun.Transpiler`; that is a syntax check, not a post-format runtime rerun, Vitest execution or semantic TypeScript checking. Final controller runtime grading is separate and passes. No participant claims otherwise.

Evidence: [blinding map](blinding.json), [L4](quality-L4.json), [B9](quality-B9.json), [S2](quality-S2.json), [G7](quality-G7.json), [blinded comparison](quality-comparison.json), each `tNN-grade.json`, answers and trace execution verification.

## Arithmetic and interpretation

Pair 1 assisted-minus-ordinary input is −79,699 (−31.5901%), with −75,776 cached and −3,923 uncached input; output is −271 and elapsed −7,882 ms. Pair 2 input is −42,153 (−16.8251%), with −42,880 cached and **+727 uncached** input; output is −625 and elapsed −20,389 ms. These values reconcile to complete per-response counters and [report.json](report.json)/[findings.json](findings.json). Total participant input is 883,804, of which 787,840 is cached; output is 19,892. Cached input and reasoning remain subsets, not additional totals.

The comparison is optional assistance plus revised launch guidance under common current host context. Both assisted participants chose direct reads. Neither observed input reduction nor zero operational overlap can be assigned to aide execution, proven guidance efficacy, cache pricing or counterfactual saved calls. Historical v5 is a separate descriptive reference; the same known task and two repetitions do not establish general or cross-host performance.

## Evaluator overhead and delivery limits

The inspected [overhead snapshot](overhead.json) begins with the current user request at 2026-09-09 18:14:29.148 UTC and ends at 18:26:02.652240 UTC. Its actor selection includes root, the two reused evaluators, v6 execution controller and v6 quality reviewer; it excludes the four participant actors. Selected response records and actor/phase totals independently reconcile to raw logs through that cutoff. The recorded request ordinal is zero-based. Input legitimately includes replay of earlier context; this is evaluation cost, not ordinary aide overhead.

Refresh the snapshot for final packaging if including later audit/reporting work, retain the explicit cutoff, and continue excluding responses completed afterward (including final delivery). A refreshed snapshot can have larger totals without contradicting this audit's checked cutoff. Index setup/export measurements remain separate from participant elapsed and zero retrieval-handler activity. The subsequently supplied README was reviewed: its headline and interpretation explicitly rule out tool-offloading savings, disclose ambient guidance and historical-comparison limits, preserve the second pair's +727 uncached-input increase, and accurately distinguish participant verification. No additional product-code change appears in the working tree. This audit does not claim browser screenshot inspection; the separate UI check must preserve its DOM-only evidence boundary and failed screenshot capture.

Reviewed at 2026-09-09T18:30:45.547842+00:00.
