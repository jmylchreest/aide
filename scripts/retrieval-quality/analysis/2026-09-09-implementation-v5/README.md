# Where the v5 implementation resources went

The two assisted implementations used more responses and more total input, with no demonstrated quality advantage. Both fetched unchanged function bodies through aide and direct reads. This supports a small guidance adjustment to reuse sufficient current source evidence; it does **not** establish how many tokens that change will save.

This is a follow-up analysis of four existing trials, not a new experiment. The [frozen v5 results](../../results/2026-09-09-outcomes-v5/README.md), trial prompts and grades remain unchanged. These Codex trials do not establish Claude Code or OpenCode outcomes.

## Observed accounting

| Measurement | Ordinary t05 | Assisted t06 | Ordinary t08 | Assisted t07 |
|---|---:|---:|---:|---:|
| Model responses | 6 | 9 | 6 | 8 |
| Input tokens | 166,234 | 271,096 | 176,572 | 238,321 |
| Cached input (included above) | 146,816 | 245,504 | 154,624 | 213,760 |
| Uncached input (input minus cached) | 19,418 | 25,592 | 21,948 | 24,561 |
| Output tokens | 4,753 | 5,329 | 4,818 | 5,038 |
| Source output bytes | 37,167 | 51,105 | 45,865 | 43,635 |
| Elapsed seconds | 91.832 | 106.285 | 91.175 | 103.002 |

Assisted input increased **63.1%** and **35.0%**. Cached input accounts for **94.1%** and **95.8%** of those increases; uncached input increased by 6,174 and 2,613 tokens. These are runtime counter differences, not billed prices or provider savings. In pair 2, source output was 2,230 bytes lower while total input was higher: returned source size alone does not measure whole-task efficiency.

The final response output counters were lower for assisted runs (99 versus 169; 72 versus 310). The higher total output occurred in earlier responses. These counters include reasoning; they are not token counts of visible final-answer text alone.

## Response activity

The table partitions input by the activity accompanying each response. Every response still carries prior context: this is **not** a causal allocation of input cost to the current tool, retrieval or editing. The first edit boundaries were manually reviewed against captured commands, and raw logs verify response/call order.

| Phase: input tokens | t05 | t06 | Pair 1 increase | t08 | t07 | Pair 2 increase |
|---|---:|---:|---:|---:|---:|---:|
| Before first edit | 38,868 | 90,992 | 52,124 | 38,870 | 66,796 | 27,926 |
| Editing and verification | 91,333 | 138,495 | 47,162 | 99,139 | 132,268 | 33,129 |
| Final response | 36,033 | 41,609 | 5,576 | 38,563 | 39,257 | 694 |

The assisted runs spent four and three responses before the first edit; ordinary runs spent two each. About half of the additional input occurs before editing (49.7% and 45.2%); the rest occurs during later responses with accumulated context.

- **t06:** outline, then batched reads of three known functions, then a direct read of unchanged writer/recorder bodies already returned by aide. That last response also reads the visible harness, so removing overlap does not necessarily remove a whole response.
- **t07:** symbol listing, then batched reads of the same three functions alongside direct reads of overlapping unchanged bodies. The overlap occurs within the same response; eliminating the duplicate command is not evidence of an avoided model response.
- **Both conditions:** read code again after editing and add scratch contract tests. Those activities can contribute verification value and are not classified as redundant merely because they consume resources.

Evidence: [t05](../../results/2026-09-09-outcomes-v5/t05-trace-review.json), [t06](../../results/2026-09-09-outcomes-v5/t06-trace-review.json), [t07](../../results/2026-09-09-outcomes-v5/t07-trace-review.json), [t08](../../results/2026-09-09-outcomes-v5/t08-trace-review.json). Their capture files preserve submitted tool inputs; the [derived timeline](accounting.json) ties every response to an outgoing call and its counters.

## Did the additional work improve quality?

An independent [review of implementation and verification value](quality-review.md) found all four meet the same mandatory bar: 5/5 source criteria, 4 visible checks and 20 hidden checks. All four add useful optional contract checks, but assisted runs do not demonstrate broader coverage or additional product behavior. Six versus five scratch tests in pair 1 is largely a grouping difference; ordinary t08 has stronger assertions on several cases than assisted t07.

Some differences have plausible value: t06 restores spies after each test; t07 reruns after adding spy cleanup. No prevented defect is observed. Conversely, t06 does not rerun participant tests after its final formatting edits; controller checks pass on the final candidate. None of these facts justifies cutting verification to improve token totals.

## Guidance adjustment and next decision

The shared welcome guidance now directs agents with known files and symbols straight to bodies, using batched symbol reads or bounded direct reads. Structure discovery is for unknown structure. Agents should reuse sufficient current evidence and fetch again for a specific missing context or post-edit verification need. A current symbol-body response can itself verify a discovery candidate; a duplicate direct read is not required. Tool use remains optional; correctness and sufficient evidence remain the aim.

The revised guidance adds 237 UTF-8 bytes to the welcome injection (prose grows from 563 to 800 bytes). That is added instruction overhead, not measured tokens. Any future net-benefit comparison must include it; this analysis does not claim that avoiding duplicate reads outweighs it.

The implementation lives in `src/core/session-init.ts`, used by OpenCode directly and by the session-start hook used by Claude Code and Codex. This is shared guidance coverage, not evidence of behavior or savings on all three hosts. No enforcement, estimator, accounting, UI or test-budget policy changes are made here.

A future comparison should retain the quality gates and record unchanged-body overlap, responses before editing, cached/uncached input, output, elapsed time and aide execution overhead separately. Acceptance should require maintained quality and observed resource benefit, not increased aide uptake alone. No new participants were launched for this analysis. The previous evaluation-overhead snapshot excludes this follow-up work, so it must not be presented as this analysis's total cost.

## Reproduce

From the repository root:

```sh
rtk proxy python3 scripts/retrieval-quality/analysis/2026-09-09-implementation-v5/analyze.py --check
```

This verifies every frozen result artifact against its existing manifest, recomputes counters and checks `accounting.json`. Add `--verify-raw` on the original machine to also verify locally retained provider-log hashes and response-to-call associations. Raw verification was performed when producing this report; those private local logs are not needed for ordinary reproduction. Captured commands are inspected as data and never executed.

Two pairs cannot establish causality or general performance. No byte-to-token conversion, hypothetical avoided-call saving, cached-token price, or future guidance saving is included in these numbers. There is no assumption that context or cache persists across resets.
