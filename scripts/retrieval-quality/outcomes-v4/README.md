# Cross-file retrieval outcome pilot (v4)

**Question:** does optional aide retrieval help an agent produce correct cross-file answers and changes, at an acceptable total resource cost?

This is a 12-trial diagnostic pilot: three tasks × ordinary/assisted retrieval × two repetitions. It is not a benchmark-wide savings estimate. No participant runs are included in this frozen preparation package.

| Task | Required outcome | Quality evidence |
| --- | --- | --- |
| [Trace](trace/prompt.md) | Follow host usage through normalization, writing, deduplication and reporting; calculate explicit examples | Seven frozen rubric criteria and source citations |
| [Impact](impact/prompt.md) | Identify affected direct and indirect consumers of a specified API change, and justify exclusions | Six frozen rubric criteria; missed callers and unnecessary changes separately recorded |
| [Implement](implement/prompt.md) | Apply the request/result API change and migrate both adapters and original test consumers | Four visible and 20 hidden runtime checks, protected-file integrity and five independent source-review criteria |

The corpus is 49 byte-identical files from `58816d9`: 46 production files plus three original Vitest tests. [Provenance](common/provenance.json) records every source hash. It includes the selected TypeScript import closure and downstream Go accounting excerpts, not the whole repository or every backend implementation. Authored harness files are separately overlaid only for implementation trials.

The implementation change exists only in the exercise and its reference solution. It does not change production collection or implement the deferred usage-recovery proposal.

## Treatment and execution

Both conditions have ordinary shell/file tools and unchanged installed hooks. Assisted trials additionally receive the current retrieval guidance, an outline/batch suggestion and five optional aide capabilities. Tool uptake is an outcome; zero use is valid. This tests that bundle, not a separately identified effect of each suggestion or tool.

The exposed collaboration API cannot bind an isolated native MCP server to each participant. The hash-pinned [v3 bridge](../outcomes-v3/bridge.py) starts fixture-scoped stdio MCP, checks root/database/socket identity and records requests, responses and timings. Native/global aide access is prohibited. This transport adds discovery and startup costs and does **not** measure native MCP presentation or cross-host UX. Participants run in the current Codex harness; Claude Code and OpenCode source behavior is covered by the fixture, not by live participant hosts.

[Protocol](protocol.json) fixes all 12 cells and their counterbalanced order before execution. Use fresh participants with `fork_turns="none"`, no model/effort override, sequential execution, no coaching, replacement or selective reruns. Verify actual runtime model/provider/effort/CLI dimensions. Fresh actors do not imply a cleared provider cache. The 40-call exploration target is advisory; correctness takes priority and all calls remain measured.

## Controller procedure

Run from the repository root; these commands do not launch agents:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v4/run.py verify
rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v4 -p 'test_*.py'
rtk proxy python3 scripts/retrieval-quality/outcomes-v4/common/validate_fixture.py
rtk proxy python3 scripts/retrieval-quality/outcomes-v4/run.py prepare /tmp/aide-outcomes-v4-trials --binary /tmp/aide-retrieval-v4
```

Preparation refuses existing destinations, changed package/helper/binary hashes, symlinks and source-overwriting overlays. It makes 12 independent roots and prompts, without indexes or participants. Do not rewrite the manifest after participant exposure. The pinned binary can live elsewhere but must match the protocol SHA-256.

After the freeze commit, seed **every** root (including ordinary trials) once with the bridge's controller-only `prepare` operation. Retain setup evidence; never reindex after edits. For each ledger row substitute its literal root/evidence paths:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v3/bridge.py prepare --root /tmp/aide-outcomes-v4-trials/t01/root --binary /tmp/aide-retrieval-v4 --evidence-dir /tmp/aide-outcomes-v4-trials/t01/bridge-evidence
```

Before each launch, verify the frozen package, prepared prompt hash and seed binding. Give the participant only its prepared prompt and root. Record actor/runtime session IDs, completion and raw capture paths even for failed or cancelled trials. Inspect captured tool traces for scope and transport compliance. Never replace a failed trial. Participants may not inspect controller evidence, grading, references, other trials or runtime captures.

Grade a candidate without mutating its source:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v4/grade.py implement /tmp/aide-outcomes-v4-trials/t05/root --output /tmp/t05-functional-grade.json
```

Use `trace` or `impact` for their integrity audit. Blind independent reviewers receive the task, frozen rubric, final answer/source patch and relevant source evidence with treatment identity removed. Review tool traces separately for protocol compliance. Trace/impact graders record every rubric check and cited justification; keep correctness separate from protocol validity.

For implementation, copy the grader's `functional_quality_verified`, `functional_quality` and `integrity` into the ledger. Passing runtime checks alone leave `quality` unknown. Supply `implementation_review` with a distinct `reviewer` identity, a `checks` object mapping **every** frozen criterion ID to a boolean, and an `evidence` object mapping those same IDs to nonempty cited justifications. The reporter derives overall correctness from both stages and ignores a bare externally supplied overall pass. Failed grades remain known failures. Incomplete review suppresses resource comparisons.

The offline Bun harness executes actual shared functions and adapters with boundary I/O mocks. It does not run the three original Vitest suites or type-check TypeScript. Migration of those original tests and public type declarations therefore require source review. Additional scratch tests under `tests/` are allowed, but mandatory grading executes only the frozen visible test paths plus hidden checks. Source review and tests provide bounded evidence, not exhaustive correctness.

Generate the report after populating the ledger:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v4/run.py report --ledger /tmp/aide-outcomes-v4-trials/ledger.json --output /tmp/aide-outcomes-v4-report.json
```

## Reporting contract

Lead with task quality, completed/planned count, uptake and compact per-task resource comparisons. Keep criterion-level grades, transcripts, receipt provenance and overhead details behind links or expandable detail. Failed, unused, cancelled and unknown rows remain visible. Compare only completed, protocol-valid, quality-passing pairs with equal actual runtime dimensions. With two repetitions, report individual values and descriptive differences, not significance or generalized savings.

Keep these quantities separate:

- Runtime-reported input, cached input, output and reasoning; cached input is a subset and host-specific reasoning overlap must not be double-counted. Missing counters stay null.
- Returned UTF-8 bytes and retrieval calls; bytes are not observed tokens, and an avoided call is inferred avoidance rather than measured provider savings.
- Participant elapsed time and local aide/bridge work. Handler elapsed is inside bridge elapsed; neither is CPU or additive independent time.
- Controller index setup and model-based preparation/execution/review/reporting overhead, with explicit response-timestamp windows and coverage limits. Large root context replays belong to this evaluation overhead, not normal aide runtime cost.

No byte estimator substitutes for runtime counters. No billing reduction follows directly from token differences. Do not pool tasks into a flattering aggregate or treat this larger fixture as a causal before/after comparison with v3.

[Preflight evidence](preflight.json) records calibration and isolation checks. Reference answers, patches and their reviews are controller-only; they are excluded from prepared roots.
