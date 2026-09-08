# September 8, 2026 retrieval pilot

All six trials passed all 46 exact-answer and citation checks. Aide-assisted
retrieval returned less source text in two pairs, but used more model responses,
more total runtime input and more elapsed time in every pair. This pilot does not
establish a billing saving, a billing increase or a general quality difference.

| Task | Treatment | Correct fields | Source result bytes | Source calls | Model responses | Input tokens | Cached input | Output tokens | Seconds |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Exit status | Ordinary | 8/8 | 28,027 | 2 | 3 | 65,314 | 43,904 | 1,141 | 46.335 |
| Exit status | Aide available | 8/8 | 22,831 | 5 | 5 | 111,630 | 100,224 | 1,076 | 61.786 |
| Window cost | Ordinary | 10/10 | 17,217 | 2 | 3 | 68,157 | 53,504 | 1,052 | 42.670 |
| Window cost | Aide available | 10/10 | 19,150 | 3 | 4 | 84,939 | 72,704 | 1,101 | 51.877 |
| Current source | Ordinary | 5/5 | 33,421 | 2 | 2 | 48,339 | 35,328 | 683 | 32.460 |
| Current source | Aide available | 5/5 | 22,658 | 6 | 5 | 110,041 | 98,688 | 898 | 52.442 |

## Boundaries and provenance

The frozen run order was exit ordinary/assisted, window assisted/ordinary, current
ordinary/assisted. Each trial used a fresh agent with no parent conversation,
`gpt-6-astra`, high reasoning effort and Codex CLI `0.153.4`. The live aide daemon
reported `0.1.18-dev.9+0840d4b`; the source checkout was `5db36e7`, whose six pinned
files match revision `0840d4b9a80023e3fd931ccd0f40102f55c922e6`. Their hashes were
verified before and after the trials. The outline correction made after this
pilot is not reflected in these results.

Source-result bytes sum attributed aide `host_result.payload_bytes` for each
audited source call. They include numbering and tool-result text, not only raw
file bytes. Source calls count individual nested calls, including a symbol batch
as one call. Tool discovery is excluded from that source-only count. Model
responses count distinct runtime response IDs; a response can contain several
tool calls. Neither column should be substituted for the other.

Runtime input/output/cache counters come directly from each completed trial's
`token_usage_record` rows: deduplicated per-response usage sums match the final
cumulative counters. These are runtime-reported measurements, not tokenizer
estimates or invoice verification. Cached input is reported alongside input and
is not added to it. Cache residency and warmup were uncontrolled. These totals
include instructions, tool discovery, repeated context and all model responses;
source-result bytes cover only retrieved source text. No avoided-call estimates,
cache compounding, dollar conversion or aggregate savings percentage is reported.

The ordinary exit-status trial additionally read 482 bytes of mandatory host
instructions. That bootstrap is recorded separately from source bytes and remains
inside its runtime usage and elapsed time. Its local instruction path is replaced
with `<mandatory-RTK-instructions>` in the published capture. No answer text or
counter was changed. All source calls were within the allowed files and 12-call
limit; the bootstrap is a disclosed host setup difference.

The product recorded these child contexts as unknown. This report uses explicit
trial runtime boundaries and matching actor/session host observations; it does
not assert that aide's context-window comparison was available. The report's
verification flags record a separate trace/citation audit, not an automatic
guarantee by the grader.

## Evidence and reproduction

- [trials.json](trials.json): grader inputs, answers, citations, measurements and
  explicit verification basis.
- [report.json](report.json): generated exact-value checks and reviewed outcomes.
- [captures.json](captures.json): selected runtime metadata, unedited final-answer
  text, call inputs, per-response usage counters and matched host events. Response
  IDs permit checking deduplication and summing the retained counters independently.
  It excludes raw runtime
  logs, reasoning, encrypted content and base instructions.
- [blind-review.json](blind-review.json): citation review against source and frozen
  oracle, performed using shuffled anonymous answers without treatment or trace.
- [sources-after.json](sources-after.json): successful post-trial pinned-file check.
- The six `*-answer.txt` files retain the exact final answers independently.

Reproduce grading from the repository root with a new output filename:

```sh
python3 -B scripts/retrieval-quality/report.py grade --input scripts/retrieval-quality/results/2026-09-08/trials.json --output /tmp/aide-pilot-regraded.json
```

This recomputes answer checks, retaining the recorded citation/trace attestations.
It does not rerun agents or independently establish citation support. Recheck
source pins before any new trial; future source changes require a new frozen
package, not edits to this dated result.

## What this changes

The assisted window-cost run fetched both full files after two outlines. Inspection
then found a reproducible Go outline defect: collapsing a body also removed the
function or method declaration on that line. Regression tests and a fix now
preserve the AST-derived signature. This is a concrete navigation repair; whether
it reduces model fallbacks or whole-task usage requires another fresh trial.

More selective retrieval can reduce returned text while adding model rounds. The
next experiment should test useful outlines and fewer discovery/read rounds before
introducing stronger retrieval steering. Keep source bytes, model usage and answer
quality separate in reporting; this pilot does not justify a savings headline in
the CLI or web Overview.

There is one trial per treatment on three narrow read-only comprehension tasks,
with no edit or debugging evaluation and no statistical confidence. All agents
shared an aide-equipped host: this compares retrieval options, not installation
on/off. Shared host instructions remained present. Tool discovery, bootstrap and
cache effects differed. Infrastructure agents ran alongside sequential participant
trials, so durations are not an uncontended speed benchmark. The citation grader
was a separate model agent, not an independent human evaluator. These limitations
prevent generalizing the observed correctness, timing or usage differences.
