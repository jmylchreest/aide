# Realistic retrieval outcomes: 8 September 2026

All six trials passed their frozen checks, but **none of the three agents with
aide retrieval available used it**. This run provides no estimate of savings
from outlines or symbol reads. It measures default tool choices and task outcomes
under optional availability. The differences below must not be attributed to aide.

We froze three tasks and their checks in commit `c665740` before launching six
fresh-context agents: trace a real OpenCode/Claude observation path, fix a seeded
no-match search regression, and extend a small pruning helper. Each pair started
from identical isolated source copies. No failed trial was replaced or discarded.
The change tasks are evaluation fixtures, not changes shipped into production.
See the [frozen protocol](../../outcomes-v1/protocol.json) and
[reproduction instructions](../../outcomes-v1/README.md).

| Task | Retrieval available | Hook retrieval bytes | Runtime input tokens | Cached input subset | Model responses | Seconds |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Navigation | Ordinary | 59,013 | 161,829 | 140,160 | 6 | 83.8 |
| Navigation | Aide available | 61,958 | 186,032 | 162,688 | 7 | 93.3 |
| Debug | Ordinary | 13,014 | 157,317 | 147,072 | 7 | 115.0 |
| Debug | Aide available | 12,621 | 156,482 | 147,456 | 7 | 101.1 |
| Edit | Ordinary | 3,864 | 100,761 | 93,312 | 5 | 82.4 |
| Edit | Aide available | 2,374 | 118,807 | 111,616 | 6 | 93.0 |

Hook retrieval bytes include source reads, searches, listings, fixture documentation
and visible test reads. They exclude mandatory RTK instructions (482 bytes per
trial), test execution output, edits and cleanup. Full task runtime input includes
all of those, instructions and repeated context. Cached input is already included
in input; it must not be added again. Runtime counters are deduplicated per response
and reconciled against final cumulative counters. They are not billing records,
and no bytes/3 token estimate is used in this table.

All participants reported the same runtime: `gpt-6-astra`, high effort,
`openai`, CLI `0.153.4`. The live aide daemon was `0.1.18-dev.15+4d91e72`.
Parent preparation, preflight, grading and review are outside participant task
usage. Exact counters, output categories and durations are in
[trials.json](trials.json); [captures.json](captures.json) retains the allowlisted
runtime evidence and attributed host events. No reasoning messages, credentials,
or unrelated session content were exported. The collector's JSON-answer parse
field is unused: this protocol deliberately requested free-text answers.

## Outcome and evidence quality

- Navigation: both answers passed all five frozen criteria and citation checks
  under [independent blinded review](blind-review.json).
- Debugging: both fixes passed all 12 frozen functional tests; editing: both
  changes passed all nine. The provided tests and other fixture files stayed intact.
- [Supplemental source review](debug-supplemental-review.json) found that `t04`
  additionally makes mixed error/invalid metadata unverified where the existing
  implementation reports failed. The prompt ambiguously required both error failure
  and invalid-metadata nonverification. The frozen grade remains unchanged; this
  limits any claim of complete behavioural equivalence. `t03` preserves the original
  precedence. Supplemental examples are static analysis, not additional executed tests.
- The aide-available navigation trial (`t02`) had an orchestration output truncated
  after the hook recorded it. Its 61,958 hook retrieval bytes are not a claim that
  all 61,958 reached the model. Logged tool-result text bytes are recorded separately,
  including wrappers and non-retrieval output; source-only delivered bytes are unknown
  for the truncated batch. Native edit responses in `t05`/`t06` also have different
  host and logged representations. Incomplete decoded-body coverage is retained.
- All child observations have unknown context-window identity. This run establishes
  no context-reset, KV-cache, repeated-context or compounded savings figure.

- The [final independent audit](final-audit.json) verifies all six protocols, 62
  nested tool calls, runtime counters and 31 frozen package hashes. It also finds
  unreliable file attribution: `t03`'s range receipt names/hashes the main checkout,
  despite the command running in its modified trial copy. `t06` has the same path
  problem hidden by identical source content. These receipts cannot establish a
  trustworthy file baseline. Byte/call/runtime measurements remain valid; no
  savings inference uses these receipts.

## What this changes in the plan

Keep outcome quality and whole-task runtime usage as the acceptance criteria.
Optional availability produced no uptake here; the traces contain no aide retrieval
or discovery calls. They do not establish why the agents chose direct reads. A small
file can reasonably favour a direct read, so this is not evidence to mandate outlines.

The production follow-up now refuses relative shell source baselines when Codex
hooks do not preserve the command working directory. Existing explicit workdir
handling resolves correctly when that field reaches aide. Exact text bytes remain
available when file attribution is unknown. The change was made after all trials;
it does not rewrite their recorded evidence or grades.

[Live verification](cwd-guard-verification.json) confirms an ambiguous relative
read has no source baseline, while an absolute read identifies the actual isolated
copy. Both record 1,678 bytes. The full plugin suite passes 461 tests, and the fix
passed independent review. The configured hooks load this checkout on each event,
so no rebuild or restart was needed.

The next retrieval investigation should examine how existing host guidance presents
selective retrieval at a useful decision point. Then test one explicit, targeted
retrieval strategy on suitable larger-file work, preserving ordinary reads for small
or broadly needed files. Record whether the strategy was actually used, its extra
calls, task quality and total runtime usage. Keep that separate from these frozen
optional-availability results; do not replace them with a more favourable run.

Do not add a savings percentage to CLI/web from this evidence. Keep top-level usage
and measured transformations concise; method, coverage, unknowns and evaluation
results belong in details. Estimator pluggability remains deferred until it helps
this outcome work. No rebuild or harness restart is required for these artifacts.
