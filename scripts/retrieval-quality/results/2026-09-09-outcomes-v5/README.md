# Full-prompt retrieval rerun (v5)

12/12 outcomes passed the frozen quality checks (12/12 graded); 6/6 assisted trials chose aide.

12/12 trials met the protocol; 6/6 pairs qualify for descriptive comparisons. Two repetitions per task do not establish a general savings rate.

Assisted input was lower in 2/6 eligible pairs and higher in 4/6. Output was higher in 5/6; elapsed time was longer in 5/6. These counts describe this pilot and do not pool token differences into a savings percentage.

[Compact report](index.html) · [Machine-readable report](report.json) · [Frozen protocol](../../outcomes-v5/README.md)

## Outcomes

| Task | Ordinary quality passes | Assisted quality passes | Assisted uptake |
| --- | ---: | ---: | ---: |
| Trace | 2/2 | 2/2 | 2/2 |
| Impact | 2/2 | 2/2 | 2/2 |
| Implement | 2/2 | 2/2 | 2/2 |

Implementation quality combines 4 visible and 20 hidden mandatory runtime checks with five independent source-review criteria. The original Vitest suites and full TypeScript checking are outside the offline harness; participant validation claims are reviewed against recorded execution. Scratch checks are separate from mandatory checks.

Trace and impact use the unchanged v4 rubric IDs and the [grading interpretation declared before exposure](../../outcomes-v5/grading-clarifications.md). Independent reviewers receive source and answers with treatment and resource counters withheld. Intrinsic answer wording can limit blinding. All failed or incomplete criteria remain visible.

## Matched observations

Assisted minus ordinary; a negative input difference means fewer runtime-reported input tokens for that individual pair. This is descriptive, not a causal saving or billing reduction. Pair direction is normalized even where execution order is reversed.

| Task / repetition | Input difference | Output difference | Elapsed difference |
| --- | ---: | ---: | ---: |
| trace / 1 | +23,038 (+8.5%) | +711 | +21.0 s |
| trace / 2 | -46,163 (-13.0%) | -349 | -11.6 s |
| impact / 1 | -27,180 (-8.3%) | +491 | +13.8 s |
| impact / 2 | +47,166 (+19.0%) | +919 | +8.9 s |
| implement / 1 | +104,862 (+63.1%) | +576 | +14.5 s |
| implement / 2 | +61,749 (+35.0%) | +220 | +11.8 s |

## Measurement boundaries

Known participant totals (12/12 trials): **3,249,552 input** and **59,934 output** tokens. Cached input (2,783,232) is included in input. Reported reasoning (3,342) is included in this host's output. No byte-to-token estimator replaces runtime counters.

Task counters include all participant responses, direct instructions, retrieval, edits, tests, failures and retries. The separately timestamped [evaluator overhead snapshot](overhead.json) covers preparation, execution control, grading and reporting, including root context replays. It excludes responses after its explicit cutoff and final delivery; it is not normal aide runtime cost.

Aide operations: `code_search` 0, `code_references` 1, `code_symbols` 5, `code_outline` 4, `code_read_symbol` 9.

Handler elapsed is inside bridge elapsed, not CPU time or additional independent time. Accumulated parallel call durations are not wall time. Index setup and controller exports are separate. Returned UTF-8 bytes are a separate measurement boundary; truncation or ambiguous attribution leaves complete totals null with documented lower bounds.

The frozen reporter accepts only integer bridge_duration_ms values, so its top-level field is null for measured fractional durations. The preserved bridge_metrics.timings_ms.total.total is the measured bridge elapsed time in milliseconds; the HTML uses that nested field. No rounding or frozen-runner change was applied.

## Scope and limits

V5 supplies the complete prepared prompt directly at launch, including the RTK rule. Every trial has archived prompt bytes, hashes and prelaunch evidence. Raw host messages can be encrypted; controller archives establish intended delivery without claiming independently decrypted plaintext equality. The v4 invalid run remains unchanged and is not pooled with this rerun.

This tests a bundle of optional tools and guidance through an isolated stdio bridge in Codex. It does not measure native MCP presentation or live Claude Code/OpenCode sessions, even though both adapters are in the source fixture. Installed hooks are unchanged across conditions. Fresh actors do not establish cleared provider KV caches; cache and shared-machine effects remain uncontrolled.

Passing bounded checks is not general quality equivalence. Increased uptake, fewer calls or fewer returned bytes alone do not prove lower total model use. Individual paired measurements do not establish a broadly transferable savings percentage or a product change recommendation.

## Evidence

Controller ledger, exact launch messages, captures, answers, patches, source inventories, criterion reviews, trace audits and bridge receipts are retained alongside the report. Blinded input copies and their corpus hashes preserve what the quality reviewer saw. Production aide code and the frozen v4 corpus were not changed by these trials.

[Independent final audit](final-review.md) · [Browser checks](visual-check.json) · [Artifact hashes](evidence-manifest.json) · [Reviewed input hashes](blind-inputs-manifest.json)
