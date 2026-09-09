---
title: Retrieval experiments
description: Evaluate retrieval correctness, model usage and aide work with reproducible evidence.
---

The retrieval experiment checks whether prescribed aide tool sequences return the
expected source evidence and how much text they produce. It uses temporary projects
and does not add synthetic observations to your project's token history.

From the repository's `aide/` directory, run:

```sh
go test ./cmd/aide -run '^TestRetrievalExperiment' -count=1 -v
```

To retain a JSON report, set `AIDE_RETRIEVAL_REPORT` to a new file path:

```sh
AIDE_RETRIEVAL_REPORT=/tmp/aide-retrieval-experiment.json go test ./cmd/aide -run '^TestRetrievalExperiment' -count=1
```

The report refuses to overwrite an existing file. Use your shell's environment
variable syntax on Windows. `-count=1` disables Go's test-result cache. Go, the
repository's normal native build prerequisites and available grammar packs are
required, as with the other retrieval tests. Tests use the standard temporary
directory; set `TMPDIR` to a clean writable directory if a parent project marker
affects local root resolution.

## What the report measures

Each scenario runs three times with a fresh project and index. The report retains
every returned text block, failed attempt, fallback, source checksum and individual
operation duration. A reference is one raw full-file read, counted once per trial.
The signed byte delta is reference bytes minus all result bytes. Positive means
less returned text; negative means overhead. Token deltas use aide's central
UTF-8 bytes/3 estimate, applied separately to the reference and each result.

Evidence checks cover a large-file outline followed by a symbol read, a small
file, ambiguous definitions followed by explicit selection, current source with
a stale index, a partially failed batch followed by a full-file read, and
unsupported content followed by a full-file read. The report also verifies
successful retrieval receipts against the exact source snapshot.

These checks do **not** measure model answer correctness, edit quality, provider
tokens, billing or cache effects. `model_quality` and `provider_usage` remain
`null`. A prescribed fallback is not a measured model fallback rate. Three
repetitions expose local variation; they do not establish statistical confidence.

Durations cover the in-process retrieval handler or local filesystem fallback.
They exclude startup, indexing, MCP transport, hooks and model time. Process,
grammar and filesystem caches are uncontrolled. Do not turn these timings into
an end-to-end latency improvement claim.

## Initial fixture results

The September 8, 2026 run produced identical text counts across all three
repetitions of each scenario. All 18 source-evidence checks passed.

| Prescribed sequence | Full-file reference | Total result bytes | Byte delta |
| --- | ---: | ---: | ---: |
| Large file: outline, symbol | 17,409 | 2,745 | +14,664 |
| Small file: outline, symbol | 52 | 246 | −194 |
| Ambiguous symbol, explicit selection | 134 | 314 | −180 |
| Stale index, current symbol | 51 | 142 | −91 |
| Partial batch, full-file fallback | 55 | 314 | −259 |
| Unsupported content, full-file fallback | 32 | 120 | −88 |

These deliberately chosen fixtures demonstrate reduction and overhead. They are
not a representative workload, and their results should not be averaged into
an aide savings headline. Rerun after retrieval or grammar changes rather than
treating these counts as permanent performance guarantees.

## Live collection verification

On September 8, 2026, the installed daemon identified itself as
`0.1.18-dev.7+f0efcc7`. Live Codex calls to `code_outline` and `code_read_symbol`
produced matching server and host records with the same receipt IDs, source
hashes and text sizes. The host records included session, actor and context epoch.
A simple shell file read also produced a verified full-file record.

This verifies those collection paths in that session, not complete host coverage
or final provider input. Its context-window comparison remained unavailable
because the session included unclassified shell activity and incomplete evidence.
That limitation must remain visible in the CLI and web Details view.

## Independent task comparison protocol

The first frozen pilot is in `scripts/retrieval-quality/` in the repository.
It defines three read-only code-understanding tasks, two retrieval treatments,
six fresh sessions, a 12-call retrieval limit per session, pinned source hashes
and 23 exact answer checks per treatment. Grading material is kept out of trial
prompts. This small pilot does not cover edits or representative workload quality;
no model trials were run when its task package was first committed. The first six
trials have since completed; results are described below.

The next stage requires fresh model contexts, not another pass by a model that
already knows these answers. Freeze repository snapshots, task prompts and hidden
answer/test checks before running trials. Include navigation, debugging and edits,
with ambiguous symbols, stale indexes, shell searches, small and large files, and
unsupported content. Include a task where the outline alone is insufficient.

Compare ordinary retrieval with aide-assisted retrieval under the same model,
host, initial instructions and token budget. Counterbalance run order and keep
each trial's context isolated. Record the model/version, host/version, aide commit,
settings, trial IDs and all failures or abandoned runs. Optional stronger steering
is a separate experiment once it exists; it is not part of the current fixture.

Grade answers and edits against the frozen checks without revealing the treatment
to the grader. Report per-task correctness, missed evidence, retries, full-file
fallbacks, wall-clock completion time and actual provider usage when available.
Keep missing provider usage unknown, and keep cached input separate from uncached
input. A context reset or cache-clear event does not justify compounding an earlier
text reduction across later turns.

Publish raw trial results, sample counts and uncertainty before drawing a
recommendation. Do not promote steering based on text reduction alone if answers
or edits worsen. The deterministic test harness does not run model-quality trials.

## First model pilot results

The September 8 pilot passed all 46 answer-and-citation checks across six fresh
agents. Aide-assisted retrieval returned less source text in two of three pairs,
but used more model responses and more total runtime input tokens in all three.
Cache-hit counts differed, so these observations do not establish a billing saving
or increase. Narrow comprehension results also do not establish general quality.

The repository retains the complete dated result in
`scripts/retrieval-quality/results/2026-09-08/`: per-trial measurements, exact
answers, selected runtime captures, host observations and blinded citation reviews.
Source-result bytes and runtime-reported input/output/cache counters have different
boundaries and are reported separately. Unknown child context epochs prevent a
product context-window comparison; explicit runtime trial boundaries support this
separate pilot report.

Offline Python helpers in `scripts/retrieval-quality/` verify frozen source hashes,
extract selected data from an explicitly named Codex runtime log and grade recorded
answers. They do not call a provider. Missing or conflicting usage stays unknown;
failed and invalid trials stay visible. The directory README includes runnable
commands and the input schema. These diagnostic reports belong alongside the
experiment evidence, not in the web Overview as a savings claim.

## Repetition after the outline fix

A second six-agent run used the same tasks and instructions with daemon
`0.1.18-dev.12+7da2150`. All 46 answer/citation checks passed again, but two ordinary
trials violated the frozen literal-search restriction. They remain visible with
invalid protocol status and suppressed comparison deltas. The sole valid
ordinary/assisted pair returned less source text with aide while total runtime input
remained higher. The valid assisted window-accounting trial used symbol batches without
its previous full-file fallback: 15,530 source-result bytes versus 19,150 in the
first assisted run, with four model responses in both. Uncontrolled cache state,
bootstrap and ordinary-treatment variation prevent attributing that difference
solely to the fix.

The dated evidence and an offline before/after comparison are retained under
`scripts/retrieval-quality/results/2026-09-08-post-outline/`. `compare.py` preserves
both trial records and unknown values, and requires matching runtime metadata for
runtime-related deltas. These measurements remain separate from billing.

After the repetition, the outline tool's unconditional outline-first instruction
and dramatic reduction claim were replaced with conditional navigation guidance.
That prose change was not part of the measured run. Its pinned source now differs
from the experiment snapshot: use checkout `7da2150` to verify those old hashes,
and freeze new sources before another model trial.

## Navigation, debugging and editing outcomes

The next protocol, frozen in `c665740`, uses isolated real-code copies for one
navigation task, one seeded bug fix and one small behavioural change, each with
ordinary retrieval or optional aide outlines/symbol reads. Regex searches,
listings and size checks are explicitly permitted. All six trials remain in
`scripts/retrieval-quality/results/2026-09-08-outcomes/`; the reproducible task
package is `scripts/retrieval-quality/outcomes-v1/`.

Both navigation answers passed five independently graded criteria. Both bug fixes
passed 12 frozen regression tests, and both edits passed nine. Supplemental review
also records an ambiguous error-precedence requirement and an additional behaviour
change that the frozen tests did not distinguish. Passing these checks is not an
exhaustive correctness guarantee.

None of the three participants with aide retrieval available used it. Their
runtime differences therefore do not estimate savings from aide retrieval.
Whole-task counters, cached-input subsets, call counts and hook-boundary bytes are
reported separately. An outer truncation in one trial shows why hook bytes cannot
be equated with final model delivery. A working-directory attribution issue also
produced unreliable source receipts while leaving measured text bytes intact.
The result README and independent audit preserve these limits; no savings figure
from this run belongs in the CLI or web Overview.


## Repeated outcomes with optional guidance

The v2 package, frozen in `928ee21`, separates three conditions: ordinary
retrieval, optional aide retrieval, and optional aide retrieval with selective
navigation guidance. It plans two repetitions of each condition for a large-file
navigation task, a four-file debugging task and a small edit: 18 fresh contexts
in a fixed, balanced, sequential order. The small edit permits a direct read;
guidance does not require an outline when most of a file is needed.

Use `scripts/retrieval-quality/outcomes-v2/README.md` for the offline preparation,
grading and reporting commands. Preparation copies only participant templates
and verifies their frozen hashes. Hidden tests and navigation rubrics stay outside
participant roots. Grading preserves the roots and runs functional checks in
separate copies. Reports retain every planned row, including missing, failed,
invalid and zero-uptake trials. The report CLI checks the package manifest,
protocol digest and pinned usage collector before producing comparisons.

Runtime counters, quality, elapsed time, aide operations and returned text bytes
remain separate. Descriptive resource deltas require completed, valid trials,
passing independent grades and matching runtime model/provider/effort/CLI metadata.
Provider cache state is uncontrolled. All conditions retain the installed aide
hooks, so this is a retrieval strategy experiment, not an installation comparison.
Setup, orchestration and grading are additional work, outside participant totals.

This execution uses Codex. The task fixtures can be reused on Claude Code and
OpenCode, but these results establish no cross-host performance claim. Keep host
usage exports and their coverage distinct when repeating the protocol elsewhere.

The September 9 execution completed all 18 trials. All six debug fixes and six
small edits passed their frozen tests. Each navigation answer passed four of five
criteria: counters and citations were correct, but all omitted an explicit cache
double-counting warning required by the rubric and not expressly requested in the
prompt. The strict failures remain recorded; the prompt/rubric mismatch limits
their interpretation and navigation comparison deltas are suppressed.

Only the two guided navigation trials used aide retrieval, with nine and four
operations. Their verified server receipts total 35 ms and 22 ms of operation
elapsed time, separate from model and whole-task time. All quality-qualified
code-task comparisons have zero aide uptake and therefore do not estimate a
retrieval-tool benefit. This pilot does not justify stronger default guidance.

The compact visual report and complete evidence are in
`scripts/retrieval-quality/results/2026-09-08-outcomes-v2/` (`index.html` and
`README.md`); the directory retains its preparation date. It includes all runs,
actual input/output/cache counters, source-return and host boundaries, truncation
qualifications, blind grades, and a separate snapshot of experiment overhead.
No savings percentage from this pilot belongs in the CLI or web Overview.
