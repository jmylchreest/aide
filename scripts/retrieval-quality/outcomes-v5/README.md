# Cross-file retrieval rerun (v5)

This balanced 12-trial rerun asks whether optional aide retrieval supports correct cross-file work at acceptable total resource cost. It reuses the [unchanged v4 corpus, tasks, treatments and grading](../outcomes-v4/README.md), pinned by its manifest. It does not pool with or replace the v4 results.

The launch repair supplies the **complete prepared prompt directly in `spawn_agent.message`**, including the shell-prefix requirement before any tool use. No instruction-file pointer is used. The exact UTF-8 prompt and hash are archived before launch. The shared instructions explicitly settle pipeline-prefix interpretation. [Grading clarifications](grading-clarifications.md) are fixed before participant exposure.

All twelve cells retain their v4 counterbalanced order. Use fresh actors, `fork_turns="none"`, no model/effort override, sequential execution, no coaching, replacements or selective reruns. A deviation or failed answer remains in the report. Before each launch, verify the package, full prompt, pristine source inventory, zero prior bridge calls, isolated seed binding, prior completion and absence of any previous launch for that cell. Record prelaunch evidence for t01 too.

Freeze and commit this package before preparing the actual roots. Preparation and prelaunch helpers do not launch agents. The controller passes the entire recorded `launcher_message` unchanged to the collaboration tool, then records the actor. Encrypted host task records cannot independently prove plaintext message equality; retain controller evidence without overstating that boundary.

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v5/run.py verify
rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v5 -p 'test_*.py'
rtk proxy python3 scripts/retrieval-quality/outcomes-v5/run.py prepare /tmp/aide-outcomes-v5-trials --binary /tmp/aide-retrieval-v4
```

Seed every root once through the inherited scoped bridge, including ordinary conditions. No reindexing after edits. Use the unchanged v4 disposable grader and independent blinded review; provide reviewers the clarification document above. Audit tool traces independently.

Lead the report with quality, uptake and per-task observations. Only completed, protocol-valid, quality-passing pairs with matching actual runtime dimensions receive descriptive deltas. Keep input/cache/output/reasoning, elapsed time, source bytes, local handler/bridge work and evaluator overhead separate. Cached input and host-overlapping reasoning are subsets. No pooled savings percentage, causal billing claim or live Claude Code/OpenCode claim follows from this Codex bridge pilot.

The inherited reporter's integer-only top-level `bridge_duration_ms` can remain null for fractional measurements. Preserve actual values in `bridge_metrics.timings_ms.total.total` and identify that measurement boundary in the report. Do not silently round or change frozen helpers.
