# Post-outline repetition: September 8, 2026

All 46 exact-answer and citation checks passed, but two ordinary trials violated
the frozen retrieval restrictions. Four trials are protocol-valid; only the
exit-status task has a valid ordinary/assisted pair in this repetition. The two
invalid trials remain in the table and report, with comparison deltas suppressed.
The valid pair returned less source text with aide while using more runtime input.
This does not establish a billing saving or a causal effect of the outline fix.

| Task | Treatment | Correct fields | Source result bytes | Source calls | Model responses | Input tokens | Cached input | Output tokens | Seconds |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Exit status | Ordinary | 8/8 | 28,027 | 2 | 3 | 65,025 | 54,272 | 930 | 40.841 |
| Exit status | Aide available | 8/8 | 23,315 | 4 | 4 | 85,748 | 75,904 | 943 | 52.304 |
| Window cost | Ordinary (invalid protocol) | 10/10 | 19,004 | 4 | 4 | 82,460 | 72,960 | 1,078 | 50.872 |
| Window cost | Aide available | 10/10 | 15,530 | 4 | 4 | 83,383 | 72,576 | 1,051 | 56.620 |
| Current source | Ordinary (invalid protocol) | 5/5 | 21,785 | 2 | 3 | 64,869 | 56,704 | 808 | 36.037 |
| Current source | Aide available | 5/5 | 14,134 | 4 | 4 | 83,764 | 74,880 | 842 | 49.196 |

## What changed relative to the first run

The [first run](../2026-09-08/README.md) used the previous outline renderer. This
repetition used the same frozen prompts, treatment instructions, source files and
run order, with six fresh agents at `gpt-6-astra`, high reasoning effort, Codex CLI
`0.153.4`. The live daemon was `0.1.18-dev.12+7da2150`. Preflight confirmed that Go
method declarations survived body collapsing. The source checkout was `7da2150`;
all six task files still matched their hashes pinned at `0840d4b`.

The assisted window task used two outlines and two symbol batches, with no shell
full-file fallback. Previously it used two outlines and one full-file read of both
files. Its source text decreased from 19,150 to 15,530 bytes, but model responses
stayed at four and runtime input changed from 84,939 to 83,383 tokens. This shows a
different navigation path; it does not isolate why the model chose it.

The other two assisted trials used four model responses each, versus five in the
first run. Their runtime input also decreased. Ordinary retrieval changed too:
the window trial added a file line count and text search before full reads, and
the current-source trial used a targeted search instead of reading the whole second
file. Both ordinary trials violated the frozen literal-search restriction, so
their measured increases in input are excluded from eligible comparisons. These
deviations, uncontrolled caches and bootstrap differences prevent attributing
the changes solely to the outline repair.

## Evidence boundaries

As in the first run, source-result bytes sum audited `host_result.payload_bytes`
for the trial actor's source calls, including source-related line counts, numbered
text and search output. They are not raw-file byte totals or model input tokens.
Source calls count nested source operations individually; a symbol batch counts
once. Model responses count deduplicated runtime response IDs. Tool discovery is
outside the source-call count but inside runtime usage and elapsed time.

Four trials read mandatory RTK host instructions: both exit-status treatments,
ordinary window cost and assisted current source. Each returned 482 bootstrap
bytes, recorded separately from source results and retained in runtime totals and
elapsed time. The other two trials did not make this extra read. The instruction
path alone is redacted in published captures; final answers and counters are intact.

The ordinary window trial's `wc -l` (94 bytes) and regex search (2,689 bytes), and
the ordinary current-source trial's regex search (14,771 bytes), are included as
source operations. They read only allowed files, but do not meet the frozen
"literal text searches" restriction and enumerated retrieval operations. Those
trials have `trace_validity=invalid`, recorded `protocol_deviations` and null
comparison deltas. Their byte measurements and answer checks are retained. No
post-hoc waiver, replacement run or removal hides the deviations.

All participants stayed within the allowed files and 12-call limit. No application
execution or edits occurred. Runtime counters were collected from each participant's
completed log, deduplicated by response ID and matched to final cumulative counters.
The capture retains each response's counters so the sums can be checked offline.
These are runtime-reported measurements, not invoice verification. Cached input
is shown separately and is not added to input tokens. No cache-clear savings or
future reuse multiplier is inferred.

Child context status remained unknown. Explicit runtime trial identities and
matched host observations support these source-result measurements; the report
does not assert a valid product context-window comparison. The citation grader
saw shuffled anonymous answers and pinned source, without treatment or traces.
It was a separate model agent, not an independent human evaluator.

There is one trial per treatment and task in this repetition, no edit/debugging
quality evaluation, no statistical confidence and no controlled cache state.
Participant trials ran sequentially while reporting infrastructure work ran
alongside them. Elapsed times are therefore not an uncontended speed benchmark.
The host retained aide hooks and shared instructions in both treatments: this
compares retrieval choices, not installation on/off.

## Reproduce the reports

This directory retains [the pre-trial manifest](run-manifest.json),
[before](sources-before.json) and [after](sources-after.json) source verification,
[audited trial inputs](trials.json), [selected captures](captures.json),
[blind citation reviews](blind-review.json), [graded results](report.json),
[before/after comparison](comparison.json) and all six exact answer text files.

From the repository root, using new output paths:

```sh
python3 -B scripts/retrieval-quality/report.py grade --input scripts/retrieval-quality/results/2026-09-08-post-outline/trials.json --output /tmp/aide-post-grade.json
python3 -B scripts/retrieval-quality/compare.py --before scripts/retrieval-quality/results/2026-09-08/report.json --after /tmp/aide-post-grade.json --output /tmp/aide-post-compare.json
```

These commands reproduce arithmetic and exact-value grading. Source/trace/citation
verification flags remain recorded audit attestations. The comparison preserves
both sides and suppresses deltas for failed, unverified or unknown measurements.
Runtime-related deltas additionally require matching model, effort, provider and
CLI metadata. Equal metadata does not establish equal cache or host conditions.

After collection and grading, the outline tool description was changed to remove
the unconditional outline-first instruction and guaranteed reduction claim. The
new guidance allows direct reads for small files or when most content is needed.
That guidance was **not** used in this repetition, and has no measured effect here.
Its edit changes a pinned source file; reproducing source hashes requires checkout
`7da2150`. Future model trials require a newly frozen source package.

Keep this detailed evidence with the experiment reports. The CLI and web Overview
should continue to separate measured text from conditional estimates and avoid a
whole-task savings headline. The next useful test is optional retrieval guidance
on broader navigation/debugging tasks, including cases where an outline adds work.
