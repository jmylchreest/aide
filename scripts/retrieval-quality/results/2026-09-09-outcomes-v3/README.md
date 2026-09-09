# Broader retrieval outcomes — September 9, 2026

All 18 planned trials completed and **18/18 pass the frozen quality checks**.
All 12 code-change trials pass their tests; all six navigation answers pass all
five independently blinded criteria. These checks are not exhaustive correctness
proof. All 18 runtime usage captures are known and agree with their cumulative
counters.

Explicit retrieval uptake is **2 of 12 eligible trials**, both guided navigation
runs. The five calls comprise one file-symbol listing, two outlines and two batched
implementation reads. Neither definition search nor reference search was selected.
These are explicit retrieval counts: every condition retained installed aide hooks.

The guided navigation runs reported 252,251 and 229,563 input tokens, versus
171,947 and 243,987 for the corresponding ordinary runs. They took 54.4 and 62.0
seconds, versus 41.9 and 55.0. Input differences change direction across repetitions;
elapsed time was higher in both guided runs. This small sample does not establish
an efficiency gain or a quality benefit. Zero-uptake code-task differences cannot
be attributed to aide retrieval.

Task totals are **3,139,132 input tokens** (2,861,824 cached, already included) and
**43,773 output tokens** (6,170 reasoning, already included). These are runtime
counters across repeated requests, not unique context sizes or billed costs.
Preparation and review usage is recorded separately with an explicit cutoff.
That snapshot covers seven non-participant actors from `2026-09-09T00:44:28.065Z`
through `2026-09-09T01:26:16.960100Z`: **57,502,050 input tokens** (56,452,352 cached,
already included) and **217,246 output tokens**. It includes implementation,
experiment preparation, orchestration, grading and this long root session's
context replays. It excludes responses after the cutoff and is not a complete
billing total. The large overhead is part of the cost of this development/evaluation
process; it is not the overhead of ordinary aide use.

The concrete gains from this work are more discoverable retrieval choices, real
OpenCode hint delivery, working MCP batches and stronger measurement evidence.
This retest does not justify requiring more retrieval calls. A future experiment
could use repository-wide definition/impact tasks where those tools are relevant;
this pilot's small fixtures do not exercise that workload.

## What changed

Production commit `4d15db8` adds a short shared welcome guide and broader conditional
suggestions for definitions, callers/impact, file structure and batched source
reads. Direct reads remain appropriate for small files; text search remains
appropriate for literals/imports. OpenCode now delivers eligible suggestions with
tool results and records the added text. Failed reference lookups are unavailable,
not zero; indexed counts do not establish complete semantic references.

Preflight found that MCP schema validation rejected advertised symbols-only
batches. Commit `c59f59a` corrects the two optional singular selector tags; actual
MCP regression tests cover singular/batched requests and invalid empty requests.
The temporary binary used here includes that fix.

## Frozen method

The v3 package was committed in `44f25ec`; `51d42ec` completes the file inventory
with the nested grading manifest, before any trial. Its 45 file hashes cover the
protocol, templates, prompts, hidden checks, references, bridge, graders and runner.
The fixture source snapshot remains the v2 snapshot; production guidance and binary
have separate provenance. `environment.json` pins those identities and digests.

Eighteen fresh participants run sequentially in the declared balanced order: three
tasks, three retrieval conditions and two repetitions. Ordinary uses shell/file
retrieval. Available and guided can use five actual aide handlers through an
isolated stdio bridge; guided additionally receives the exact new welcome prose
and conditional structure/batch advice. Guided adds 934 UTF-8 prompt bytes over
available; that is a text measurement, not a token estimate. All conditions retain
installed aide hooks.
The run does not certify new hook text reaching a live OpenCode/Claude provider.

Every root is seeded and identity-checked before participants, including ordinary
conditions. Search/references use that seed after edits; current-file tools can
inspect new source. The bridge prevents explicit outside-file requests and rejects
unapproved tools; it is not a filesystem sandbox. Separate trace audits check source
scope, forbidden native/global aide access and controller-state access. Running
provided tests necessarily permits their own temporary test artifacts, without
allowing outside source or another trial's data.

Prepared prompts match the frozen protocol. All 18 actual actors, fresh forks,
unchanged model settings and sequential launches are verified. Delivered spawn
messages are encrypted in the raw logs, so independent plaintext equality remains
unknown; the executor records an exact-copy attestation separately.

The navigation prompt now expressly asks for the warning already required by its
rubric. Other task source and quality checks are unchanged. All rows remain visible,
including failed, invalid, incomplete and zero-uptake rows. No replacement trials
or post-launch coaching are allowed. Clarified wording, broader capabilities and
the bridge mean this is not a causal before/after comparison with v2.

## Measurement boundaries

- Runtime counters come from explicit actor logs, checked against response IDs and
  cumulative counters. Cached input is a subset of input; reasoning is a subset of
  the runtime output counter. These are not billing or savings measurements.
- Captured source-result text excludes test/edit/bootstrap output where attributable.
  Outer truncation or mixed source/diagnostic output can make complete totals
  unknown (t01, t04, t13 and t16). Full bridge-emitted text and
  host-captured text are different boundaries.
- Source receipt byte counts are independently checked against the frozen initial
  files only when their SHA256 matches the reported version. Unmatched edited
  versions stay unknown. These are source bytes, separate from returned text.
- Bridge records preserve every bound attempt, full MCP result and separate guard,
  startup, identity, request and exit timings. Invalid binding attempts can exist
  only in host traces; evidence completeness needs that separate audit.
- Handler elapsed time is accepted only through an exact receipt join to exported
  server observations. It sits inside request/bridge time and must not be added
  again. It is elapsed time, not measured CPU or a latency improvement. Integer
  millisecond timing can report zero for sub-millisecond work.
- Controller indexing, identity checks and observation exports are setup/report
  work. `setup-verification.json` and per-trial export records retain their times.
- `overhead.json` is a separate model-usage snapshot for preparation,
  orchestration, grading and reporting, with an explicit cutoff. It includes root
  context replays and excludes responses completing after that cutoff.

Quality, model input/output, cached input, elapsed time and local work remain
separate. Comparisons require passing verified quality, protocol validity and
matching actual runtime metadata. Two repetitions and uncontrolled cache state do
not establish general performance or causal savings. No estimate from this pilot
is added to the product Overview.

## Evidence

Use `index.html` for the compact report and collapsed work details; `report.json`
and `reviewed-ledger.json` retain machine-readable results. Per-trial captures,
answers, patches, functional grades, trace reviews, bridge records and exported
observations sit alongside them. `production-review.md` and `preflight-review.md`
record implementation checks and the defects resolved before freezing.
