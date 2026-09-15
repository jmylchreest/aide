# Independent retrieval quality pilot

This is a frozen, read-only pilot for six fresh agent sessions: three code-understanding
tasks, each attempted with ordinary retrieval and with aide retrieval available.
It does not run automatically or make a provider API call.

`tasks.json` contains the exact common instructions, treatment instructions, tasks,
allowed files and run order. `grading.json` is grader-only material. `sources.json`
pins the source file hashes. Do not expose the grading file or this directory to
trial agents; compose each prompt from its task and applicable instructions only.

Before running, verify every pinned hash against the checkout. If a source changed,
freeze a new pilot revision and recheck the oracle before any trial. Do not update
the oracle in response to an agent's answer. Use the same model, reasoning settings,
host instructions and 12-call retrieval limit for both treatments. Create each
agent with no conversation history. Never reuse a session between treatments.

The ordinary treatment excludes aide code tools, but it does not disable aide
hooks or host-level instructions. This tests retrieval choices within one host,
not aide installed versus uninstalled. The tasks deliberately exercise conditions
inside function bodies, where an outline alone is insufficient.

Run sequentially in the specified order, reversing treatment order for the middle
task. Six trials provide no statistical confidence or full order balance. Record
model/host/runtime identity, source hashes, task/treatment, agent ID, start/end time,
call trace and unedited answer for every trial, including invalid or failed runs.
Confirm the trace obeyed the retrieval policy; do not rely on an agent's claim that
it did. A run whose trace cannot be checked must be labelled unverified.

Blind grading to treatment. Match the answer values and check each source citation.
Report per-field correctness and whole-task pass/fail; retain infrastructure
failures and policy violations separately. The parent grader already knows aide's
implementation, so the frozen exact-value oracle limits discretion; it does not
make this an independent human evaluation.

Use attributed host receipts for returned-text quantities only when the recorded
host/session/actor/invocation evidence establishes the trial boundary and coverage.
Otherwise leave returned text unknown. Agent self-reported byte totals are not
measurements. Provider input/output/cache usage remains unknown unless the runtime
provides it directly. Elapsed time is dispatch-to-completion wall time, including
tool waits; report it without treating one run as a stable speedup estimate.

These tasks test narrow code comprehension. They do not test edit validity,
implementation quality, representative debugging performance, other hosts, or
production cost savings. Broader tasks and repeated runs remain necessary before
changing retrieval steering. No model trials have been run when this pilot is
first committed.

## Offline automation

Python 3.10+ is required. These commands read local files and never launch a model
or call a provider. Run them from the repository root:

```sh
python3 -B -m unittest discover -s scripts/retrieval-quality -p 'test_*.py'
python3 -B scripts/retrieval-quality/report.py verify-sources --root . --output /tmp/aide-pilot-sources.json
python3 -B scripts/retrieval-quality/collect_codex.py --log /path/to/one-trial.jsonl --output /tmp/aide-pilot-capture.json
python3 -B scripts/retrieval-quality/report.py grade --input scripts/retrieval-quality/results/2026-09-08/trials.json --output /tmp/aide-pilot-grade.json
```

Use new output paths; both scripts refuse to overwrite files. The collector accepts
one explicitly selected Codex runtime log. It extracts final answers, runtime
identity, call inputs and usage counters; it omits reasoning, encrypted content
and base instructions. Call inputs and answers may still contain project data, so
review a capture before sharing it. This is a diagnostic adapter for the observed
runtime schema, not a cross-host usage collection path in the plugin.

The collector sums per-response usage once per response ID and checks the final
cumulative counters. Missing, conflicting or incomplete evidence leaves usage
unknown. Runtime-reported input, cache and output counters are separate from
returned-text measurements and from billing. Cached input is not added to input.

The grader accepts an array of records with `id`, `task`, `treatment`, `answers`,
`evidence`, `source_validity`, `trace_validity` and a per-field `evidence_review`.
It checks exact JSON values and types against the frozen oracle. Source and trace
validity and citation support require a separate audit; the script does not infer
them from answers. Optional `observed_text_bytes` and `provider_usage` require
explicit verification flags. Here `provider_usage_verified` means the supplied
runtime counters were checked, **not** that an invoice was verified. Preserve a
`usage_verification_basis` with each record. Failed, invalid and unverified trials
remain visible; unknown quantities remain null.

## Completed pilot

The [September 8 results](results/2026-09-08/README.md) retain all six answers,
audited host observations, runtime captures, blinded citation reviews and the
generated report. All 46 answer fields passed, but aide-assisted runs used more
total model input in all three pairs. Smaller source results alone did not predict
lower whole-task input. This is evidence to investigate round trips and retrieval
quality, not a general savings or cost claim.

The [post-outline repetition](results/2026-09-08-post-outline/README.md) uses the
same frozen tasks and retrieval instructions with the repaired outline renderer.
Its report can be compared with the first run using the offline helper:

```sh
python3 -B scripts/retrieval-quality/compare.py --before scripts/retrieval-quality/results/2026-09-08/report.json --after scripts/retrieval-quality/results/2026-09-08-post-outline/report.json --output /tmp/aide-pilot-comparison.json
```

`compare.py` matches trial identities and task/treatment pairs, retains both rows
and emits signed after-minus-before deltas only for eligible measurements. Failed
or unverified trials remain visible and suppress deltas. Runtime-related deltas
also require matching nonempty model, effort, provider and CLI metadata; missing
or conflicting conditions have explicit reasons. Matching metadata does not verify
cache equivalence, source identity or complete host conditions. Those need the
separate run manifest and trace audit. No percent saving or causal effect is inferred.

After these trials, the outline tool description was changed to conditional
guidance. Consequently the current checkout no longer matches the old frozen
`cmd_mcp_code.go` hash. The source verifier must fail on that mismatch. Reproduce
the old source check in a checkout of `7da2150`; freeze a new package before a new
model trial. Offline grading and comparison of the retained reports still work.
