# Retrieval outcomes v2 — September 9, 2026

**18/18 trials completed. All 12 code changes pass. All six navigation answers pass 4/5 frozen criteria. Aide retrieval was used in 2/12 optional/guided trials. This pilot establishes no validated general token-saving claim.**

Open [the compact visual report](index.html) for top-level numbers and per-task input/cache bars. [report.json](report.json) contains the machine-readable rows; [reviewed-ledger.json](reviewed-ledger.json) links their evidence. Details remain outside the product Overview.

| Task | Ordinary | Aide available | Selective guidance |
| --- | --- | --- | --- |
| Large-file navigation | Both 4/5 criteria; no aide calls | Both 4/5; no aide calls | Both 4/5; 9 and 4 aide calls |
| Four-file debugging | Both pass 12/12 tests | Both pass 12/12 | Both pass 12/12 |
| Small edit | Both pass 9/9 tests | Both pass 9/9 | Both pass 9/9 |

The navigation answers supplied correct counters and supported citations but omitted an explicit warning against adding cache counters to normalized input again. The frozen rubric required that warning; the prompt did not expressly request it. Independent blind grading retains the failures. This prompt/rubric mismatch limits their interpretation: the result does not show that retrieval harmed answer quality. Navigation comparison deltas are suppressed, and no replacement trials or retrospective criterion changes were made.

The two guided navigation trials used outlines and batched symbol reads. Their runtime input counts were lower than the ordinary runs, but they did not pass every frozen quality criterion. Optional navigation also varied substantially without using aide. All quality-qualified code-task comparisons have **zero aide retrieval uptake**, so their differences cannot estimate a retrieval-tool benefit. These results do not justify stronger default outline guidance. A future, separately frozen experiment should first clarify the navigation criterion and use more varied tasks.

## Measured resources

Across the 18 participants, the runtime reported **2,851,533 input tokens**, of which **2,602,880 were cached input**, and **43,335 output tokens**. Cache-write input was reported as zero; reasoning output was 6,017 and is retained separately, not added again to output. These are runtime reports, not invoices or inferred byte-estimator savings. The summed participant task durations were 926.878 seconds; that is not total experiment elapsed time.

Aide returned 13 retrieval operations across the two guided navigation runs. All work receipts joined uniquely to the correct host actor and matching source/text hashes. Their summed measured operation elapsed times were **35 ms and 22 ms**. This excludes model time, transport, hooks, daemon startup and background indexing; it is neither CPU time nor a whole-task latency saving. [Receipt reviews](t07-receipt-review.json) retain [both runs](t10-receipt-review.json).

[overhead.json](overhead.json) separately captures model usage for implementation, preparation, coordination, trace review, blind grading and reporting within its explicit response-timestamp window. It includes the reused parent context and failed preparatory attempt. The snapshot excludes responses finishing after its end, including final delivery. It must not be presented as complete billing or hidden inside participant totals. The quoted input counts include cached subsets; do not add them again.

## Evidence boundaries

- All 18 actors are distinct, fresh `fork_turns=none` runs with matching reported model/provider/effort/CLI metadata. Participants ran sequentially in the frozen order; offline review and background machine activity were not isolated, so timings are descriptive.
- Provider cache state was uncontrolled. All conditions retained installed aide hooks. This tests retrieval conditions, not installing versus removing aide; it establishes no Claude Code or OpenCode performance result.
- Runtime records protect assignment message text. Prepared prompts matched frozen composition and were submitted directly; exact delivery bytes cannot be independently compared from protected runtime logs. [Spawn provenance](prompt-provenance.json) records that limit rather than treating protected text as a mismatch.
- Source-result bytes, host payload bytes and final runtime input tokens have different boundaries. Trials t10 and t16 have outer truncation. Their visible returned text counts include the truncation marker; larger host payloads are not claimed to have reached the model unchanged. An observed reread is retained, not converted into a causal fallback/savings rate.
- Six expected pre-fix reproducer failures are retained separately from transport failures. No transport failures, retry-after-tool-error calls, source/edit scope violations or call-target overruns were found in the reviewed traces. Ordinary temporary test data is permitted consistently with the supplied reproducer; scope review does not claim syscall-level filesystem confinement.
- Known zero remains separate from unknown. Hook injection totals and hook CPU time remain unknown. A missing grep after an outline is not counted as a proven avoided operation.

## Reproduction and provenance

The package was prepared September 8 and frozen before trial one in commit `928ee21`, using source snapshot `efe758c`. The result directory retains the preparation date; execution occurred September 9 UTC after an interrupted preparatory review was resumed. [Environment](environment.json), [preflight review](preflight-review.md), frozen SHA256 inventory and pinned collector identify the inputs. No rebuild, live-daemon replacement or collection-recovery change was required.

Each trial retains its final answer, patch, runtime capture, tool-result evidence, integrity record, host observations and independent trace review. Functional grades use the frozen visible/hidden suites in disposable copies. Navigation retains both masked grade batches and their label mapping. The raw runtime logs stay local; captures omit model reasoning content.

The dated `capture_trial.py`, `capture_overhead.py`, `check_prompt_provenance.py`, `assemble_report.py` and `render_report.py` record evidence processing. They use local logs/roots and refuse to overwrite their output artifacts. The frozen general preparation/reporting instructions are in [outcomes-v2/README.md](../../outcomes-v2/README.md). Raw local logs are needed to regenerate usage reports; checked-in captures preserve the selected reported evidence without redistributing complete sessions.

Validation: 19 offline runner/grader tests pass; fixture ESLint passes; production Vitest discovery excludes the isolated Bun fixtures. The frozen seed suites fail and their reference fixes pass. These checks validate this evaluation machinery and its declared task contracts, not general aide effectiveness.
