# Outcome evaluation v1

Six fresh-context trials: navigation, debugging and editing, each with ordinary
retrieval or optional aide outlines/symbol reads. `protocol.json` freezes task
order, permissions, treatment wording, grading and measurement boundaries before
participants start. This is a descriptive pilot, not a general savings estimate.

Create candidates in a new ignored repository directory:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v1/prepare.py .aide/state/evaluation/2026-09-08-outcomes
```

Give each participant only the protocol's shared instructions (substitute ROOT),
its treatment instructions and its task's `prompt.md`. Do not give participants
this package, hidden tests or reference fixes. The navigation snapshot is rebuilt
from its pinned Git commit and verified against all source hashes. Debug/edit
templates contain real source plus visible reproducers; reference fixes and
functional graders are separate. Fixture validation JSON records the expected
seed failures and reference passes.

Grade a completed change with:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v1/grade.py debug CANDIDATE_ROOT
rtk proxy python3 scripts/retrieval-quality/outcomes-v1/grade.py edit CANDIDATE_ROOT
```

Grade navigation independently against `navigation/rubric.json`, withholding
treatment and usage. Audit source scope and provided-test integrity separately.
Retain every trial, including failures and protocol deviations. Use the existing
`../collect_codex.py` to extract runtime counters from explicit participant logs.
Runtime counters are not billing records. Returned source text, host hook bytes
and runtime model input are different measurement boundaries.
