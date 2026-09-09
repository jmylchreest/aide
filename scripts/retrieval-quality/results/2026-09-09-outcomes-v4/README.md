# Cross-file retrieval outcome pilot (v4)

12/12 outcomes passed the frozen quality checks; 6/6 assisted trials chose aide.

12/12 trials have recorded protocol deviations; the frozen reporter permits 0 paired comparisons. Individual counters remain visible. No token-saving percentage is established.

[Open the compact report](index.html) · [Machine-readable evidence](report.json) · [Frozen protocol](../../outcomes-v4/README.md)

## Outcomes

| Task | Ordinary quality passes | Assisted quality passes | Assisted uptake |
| --- | ---: | ---: | ---: |
| Trace | 2/2 | 2/2 | 2/2 |
| Impact | 2/2 | 2/2 | 2/2 |
| Implement | 2/2 | 2/2 | 2/2 |

Each implementation was assessed with the same 4 visible and 20 hidden runtime checks, source integrity and five independent source-review criteria. Original Vitest suites and full TypeScript type checking were not executed. Optional participant scratch tests are retained separately and do not increase the frozen mandatory check count.

Trace and impact grading checked factual routes, normalization, conflict handling, affected consumers and bounded claims. After answers were collected, the reviewer corrected an inconsistent interpretation of the existing prompt-scope rule while still blinded. [Initial grades and adjudication](quality-adjudication.md) are preserved; the frozen rubric was not changed.

## What the measurements establish

The 12/12 trials with complete runtime counters consumed **3,542,763 input tokens** and **62,246 output tokens** in total. Cached input (3,091,584) is a subset of input. Reported reasoning (3,440) is a subset of this host's output. These are runtime counters, not billed amounts or estimated file tokens.

These totals include participant instruction reads, retrieval, editing, tests, failures and retries. They exclude controller preparation, execution management, grading and reporting, which are recorded in the separate [overhead snapshot](overhead.json). Root context replay is evaluation overhead, not normal aide operating cost. Its timestamp cutoff excludes later responses and final delivery.

Recorded aide operations: `code_search` 0, `code_references` 0, `code_symbols` 14, `code_outline` 2, `code_read_symbol` 14.

Bridge evidence records scoped identity, returned text and local timings. Handler elapsed sits inside bridge elapsed and is not CPU time; index setup and controller exports are separate. Captured source-output bytes are a distinct boundary: truncated/unattributable results leave complete totals unknown and retain conservative lower bounds. No byte-to-token estimator replaces provider counters.

The frozen reporter accepts only integer bridge_duration_ms values, so its top-level field is null for measured fractional durations. The preserved bridge_metrics.timings_ms.total.total is the measured bridge elapsed time in milliseconds; the HTML uses that nested field. No rounding or frozen-runner change was applied.

## Protocol limitation and next step

The launcher pointed each participant to an instruction file. Initial bootstrap reads used raw shell commands before the participant had read the `rtk` requirement. This is a launcher design flaw, and the trace reviews retain the exact deviations. Authorized instruction-file reads remained within scope, but the frozen protocol does not exempt them from the prefix rule. No trial was coached, replaced or selectively rerun.

The reporter therefore keeps the observed outcomes and counters but suppresses ineligible comparisons. Do not treat matching quality grades as general quality equivalence, fewer returned bytes as measured token savings, or greater uptake as an efficiency gain.

For a future clean run, pass the complete prepared prompt directly at launch so the constraints are available before any tool call. Freeze that launch change before another balanced run, retain this dataset, and keep the same bounded outcome grading. The recorded prompt-scope interpretation should be made explicit in that future protocol. This report does not silently waive the current rules or replace failed protocol checks.

The source fixture includes Claude Code and OpenCode adapters, but participants ran only in Codex. Access used an isolated stdio bridge, so this does not establish live cross-host performance or native MCP presentation effects. Provider cache was uncontrolled, even with fresh actors. Two repetitions per condition and one bounded source snapshot cannot establish a general savings rate.

## Evidence

- `ledger.json` preserves controller execution state; `reviewed-ledger.json` combines separate reviews.
- `tNN-capture.json`, answers, tool results, provenance and patches preserve each actor's evidence.
- `changes/tNN/` preserves changed and added files; final source inventories are in provenance records.
- `tNN-trace-review.json` records scope, protocol deviations, nested calls, test execution and byte boundaries.
- `quality-*.json` contains blinded criterion grades; `blinding.json` reveals the final mapping.
- `blind-inputs/` retains the reviewed answers and check evidence; `blind-inputs-manifest.json` hashes the full reviewed corpus, including reconstructable source copies.
- `bridge/tNN/` and server-work exports preserve setup, identity, receipts and timing evidence.
- [Independent final audit](final-review.md), [browser checks](visual-check.json) and [artifact hashes](evidence-manifest.json) record the final verification boundaries.
- The frozen package remains unchanged at commit `032b074`; production aide code was not modified by the trials.
