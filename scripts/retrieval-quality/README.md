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
