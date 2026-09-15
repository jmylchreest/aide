# Focused implementation comparison (v6)

Four fresh implementation trials compare ordinary retrieval with optional aide retrieval and the revised welcome guidance. Order: ordinary/assisted, then assisted/ordinary. This is a small descriptive follow-up on the task that motivated the change, not held-out validation or an isolated test of wording.

The restarted host may supply revised guidance to every child. `fork_turns=none` does not remove common host/developer instructions. Consequently, an old-versus-new installed-guidance comparison is not supported here. The ordinary arm explicitly prohibits aide access; the assisted arm permits only the frozen fixture-scoped bridge. Both run under the same current host configuration.

## Frozen inputs

- Source, implementation prompt, grader and public-type/consumer review rubric: unchanged `outcomes-v4`, transitively verified through the `outcomes-v5` manifest.
- Binary: the pinned `/tmp/aide-retrieval-v4`, SHA-256 `38ad788faab22d2f0376b5b53488c69fdbc8eb5991dda054772da2369fd7a5f0`. The live rebuilt binary is not the experimental bridge binary.
- Assisted guidance: exact current two-paragraph welcome text from `src/core/session-init.ts` at `28edfdc`, archived in `guidance.txt`. The additional v5 experimental navigation paragraph is omitted. This changes the supplied guidance package, so historical differences cannot isolate the welcome edit.
- Launch checks: reuse v5 full-message equality, pristine inventory, binary/index binding, ordering and no-replacement checks through the new runner.
- Calibration: reuse verified v4 seed/reference results (4 visible/7 hidden pass with 13 hidden failures for seed; 4 visible/20 hidden pass for reference), mutation checks and independent fixture review. No claim of newly rerun calibration. Bun revision `1.4.0+34cbb9a40` matches that evidence.

## Execute and judge

Commit the manifest and protocol before launching. Prepare only the four planned roots; seed all four with the same pinned bridge. Use a fresh actor with the exact archived complete prompt per trial, sequentially, with no coaching, replacement, internet, native/global aide or direct aide CLI. Retain failures and incomplete rows. Record actual model, provider, effort and CLI; suppress pair deltas if those differ or quality/protocol checks fail.

Every candidate needs the existing runtime checks and independent source review. Quality covers contract types, writer behavior, recorder cache/retry behavior, production and original-test consumers, and truthful validation reporting. Optional test count is not additional mandatory correctness. Keep blind source review inputs separate from treatment and resource counters; intrinsic answer wording can limit blinding.

Review repeated source-body retrieval using the operational definition in `protocol.json`. Preserve command/output evidence and distinguish partial overlap, missing context, truncation, stale data and post-edit verification. An overlapping call in a batch is not a measured avoided model response.

Report input, cached/uncached input, output/reasoning, responses, elapsed time, source bytes, local bridge/handler work and evaluation overhead separately. Cached input is included in input, reasoning in output on this host, and handler elapsed in bridge elapsed. All participant instructions count. No byte estimator, billing conversion or counterfactual saved-call total replaces runtime counters.

Do not pool with v5 or claim causality, general savings, native MCP performance, cleared KV caches or cross-host performance. A favorable result would justify more evaluation; maintained quality with increased aide uptake alone is not evidence of efficiency.

## Commands

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v6/run.py verify
rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v6 -p 'test_*.py'
rtk proxy python3 scripts/retrieval-quality/outcomes-v6/run.py prepare /tmp/aide-outcomes-v6-trials --binary /tmp/aide-retrieval-v4
```

Preparation does not seed indexes or launch actors. Preflight and launch recording are separate runner commands; the collaboration controller passes the returned full prompt directly to a fresh actor.
