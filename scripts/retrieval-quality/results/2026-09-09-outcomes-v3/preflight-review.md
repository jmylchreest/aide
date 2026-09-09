# Independent v3 experiment preflight

Verdict: **ready for manifest freeze** at the reviewed protocol digest below. No unresolved preflight defects. This is approval of the package before freeze, not a claim that the manifest or all 18 controller indexes already exist. No model trials were run.

Reviewed scope: `scripts/retrieval-quality/outcomes-v3/{protocol.json,README.md,run.py,grade.py,bridge.py,test_run.py,test_grade.py,test_bridge.py}`, navigation prompt/rubric/source excerpts, debug/edit prompts and grading specifications, and the real temporary binary. Production was read only for guidance, schema, description and provenance checks. The result assembly/render scripts are outside this bounded package review.

## Provenance verified

- Guidance/source commit: `c59f59afd24bc3cb965af3fc0ba920db92bfc9d0`.
- Temporary executable: `/tmp/aide-retrieval-v3-next`.
- Binary SHA256: `073252810ae38e96052504b742d9b05b7382437e1c3f12454f667aa3b3491d13`.
- Reviewed protocol SHA256: `545e57ee5fc4bf16e41beb98022e46c2de07a83ebfab8264414eb4671d686731`.
- Every `guidance_source_hashes` and `tool_description_source_hashes` entry matches the current file bytes. The parent collector digest matches. README uses the corrected binary path.
- Actual MCP `tools/list` was queried through a fresh isolated process: all five descriptions match the current Go source strings byte for byte, and all property sets match the bridge allowlist. `code_search` requires query; symbols/outline require file; references/read_symbol no longer unconditionally require symbol.

## Checks passed

| Area | Independent evidence |
| --- | --- |
| Exact 18-trial order | Parsed v2/v3 protocol arrays are equal; 18 rows and 18 unique task/treatment/repetition cells. Order remains navigation/debug/edit throughout, with the specified balanced ordinary/available/guided assignments. |
| Prompt/rubric match | Navigation's five questions align with the five rubric checks. The final question now explicitly asks both normalization cautions. Current fixture source confirms step-finish collection, input 31, raw output/reasoning separation, successful-write-only fingerprint caching, conflict exclusion before filtering, and no legacy session projection. Debug/edit requirements match the frozen allowed files and checks. |
| Unchanged v2 | `git diff --name-only HEAD -- scripts/retrieval-quality/outcomes-v2` is empty. Hash inventories show v3 debug/edit trees identical to v2; navigation differs only in prompt.md. The rubric and fixture source remain unchanged. |
| Guidance | Extracted the production `buildWelcomeContext` Code Retrieval section and compared its text to guided treatment: exact match. Available is an exact prefix of guided. Additional conditional structure/batch advice is explicitly described as part of guided treatment. No savings promise or mandatory tool uptake. |
| Index isolation | Read `configuration`, `prepare`, `bound_configuration`, `validate_request`, `child_environment`, and `call`. Canonical roots/binary/evidence are bound and hash checked; environment does not inherit HOME, credentials or aide live defaults. Requested handlers run only after checking actual root/cwd/DB/socket identity. Path traversal, metadata paths, unsupported tools, symlinks and altered binding/binary are rejected. Real integration verifies an external canary is absent and source edits leave indexed search stale while current-source tools see changes. |
| Participant boundaries | Shared text explicitly prohibits global/native aide MCP, direct aide CLI, other roots, package/grader/reference/evidence reads, internet, subagents and edits to controller .aide. Only the supplied bridge command is an execution exception in assisted conditions. Every condition inherits the `--glob '!.aide/**'` source-search exclusion; controller seeds all 18 indexes, including ordinary. |
| Real batch capability | Independently called the corrected binary through the isolated bridge with `symbols:["First","Second"]`, with no singular placeholder, for both code_read_symbol and code_references. Both return successful batch results. Fresh tools/list confirms the schema correction. |
| Functional grading | Independently graded temporary copies of each seed/reference with a controller-style `.aide/bin/aide` symlink. Results below. Candidate integrity and unchanged checks pass all four. |
| Controller exclusions | Read grade.py integrity, before/after snapshots and disposable copy. Root .aide is excluded in all three; similarly named or nested non-controller additions remain guarded. Tests cover the real-style symlink and absence from grading sandbox. The exclusion is explicitly not authorization to inspect/alter controller state; trace audit remains required. |
| Quality/report gates | Read run.py report/verified_report and tests. All planned rows retained; failed grades keep measured resources. Comparisons require independently verified passing quality, valid scope/trace/protocol, completion, known counters, distinct runtime actors, and equal nonempty model/provider/effort/CLI within task/repetition. Reviewed deviations, reused actors and unknown usage prevent comparisons. Manifest/protocol/collector hashes are checked; zero aide uptake remains valid. Setup, bridge timing, handler timing, source bytes and runtime usage remain separate fields. |

## Independent smoke results

| Fixture | Visible | Hidden | Integrity / unchanged |
| --- | --- | --- | --- |
| debug seed | 0 pass, 1 fail | 6 pass, 5 fail | pass / pass |
| debug reference | 1 pass, 0 fail | 11 pass, 0 fail | pass / pass |
| edit seed | 0 pass, 1 fail | 5 pass, 3 fail | pass / pass |
| edit reference | 1 pass, 0 fail | 8 pass, 0 fail | pass / pass |

All 38 Python tests passed independently with real-binary integration enabled during the package review. After the production schema correction and addition of actual symbols-only regression calls, the 14 bridge tests were rerun against `/tmp/aide-retrieval-v3-next` and passed. Separate direct schema/batch probes also passed against that corrected executable.

## Defects found and resolved before freeze

1. **Controller symlink broke functional grading.** Integrity/copy excluded .aide but before/after snapshots did not; the real bridge-created binary symlink caused inventory to raise. Package owner changed both snapshots to `exclude_controller=True` and added the symlink regression. Independent seeded/reference smoke confirms resolution.
2. **Advertised symbols-only batches were rejected by the real MCP schema.** Both tools listed singular symbol as required. Independent calls reproduced the validation error on the old executable. Production owner changed the two JSON tags to `symbol,omitempty`, added actual SDK tools/list/tools/call regressions, rebuilt the temporary binary, and updated protocol/source pins. Corrected actual batch calls and schema were independently verified. Protocol examples remain natural symbols-only batches.

## Required controller steps and interpretation limits

Freeze a complete SHA256 inventory covering all package inputs, code, tests, rubrics, references and graders, then verify it before preparation/reporting. `manifest.json` intentionally did not yet exist at the final pre-freeze pin check. Prepare fresh roots and seed/verify all 18 through the corrected binary before any participant starts; preserve those setup receipts separately. This review created only disposable smoke roots and does not stand in for all-18 setup verification.

The bridge is an index/path-binding guard, not an OS filesystem sandbox. Independent trace audit is necessary to enforce participant boundaries and controller-state integrity. Static indexes may be stale after edits; participant guidance correctly requires current-source verification. Actual rendered/provider context, inherited host hooks and provider cache state remain outside the bridge's measured source-text boundary. Bridge startup/MCP costs count in participant elapsed time; controller indexing is separate setup work. This is a new descriptive pilot, not causal evidence that guidance saved tokens compared with v2, and not a cross-host performance result.
