# Outcome pilot v2

This is an offline, frozen evaluation package for 18 fresh-context trials: three
tasks × three treatments × two repetitions. `protocol.json` supplies the exact
balanced execution order, participant instructions, treatment wording and grading
rules. Participants receive only their composed prompt and isolated task root.
No command here starts models, calls providers or discovers runtime logs.

Prepare all roots and prompts once, after freezing `manifest.json`:

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v2/run.py prepare .aide/state/evaluation/outcomes-v2
```

Preparation verifies every SHA256 entry, requires coverage of the protocol, task
prompts and every template file, and rejects an existing destination. Each trial
gets `ID/root`, `ID/participant-prompt.md` and a row in `ledger.json`. Instructions
substitute the absolute candidate path for `ROOT`. Never expose this package,
reference changes, hidden graders or another trial to a participant. Launch one
fresh-context participant per planned row in the frozen order; retain failures
and deviations without replacement runs.

Grade navigation using its frozen rubric with the grader blinded to treatment
and usage. Run the task's frozen functional grader for debug/edit, then separately
audit source scope and visible-test integrity. Tool uptake is a reviewed outcome:
zero optional aide usage is valid. The quality field records whether checks pass;
`quality_verified` means the independent grading was performed, even for a fail.

Complete a separate reviewed ledger without overwriting the prepared ledger:

```json
{
  "trials": [{
    "id": "t01", "task": "navigation", "treatment": "ordinary", "repetition": 1,
    "agent_id": "/root/participant01", "runtime_session_id": "actual-runtime-actor-id",
    "log_path": "/absolute/path/to/explicitly-selected-completed-log.jsonl",
    "status": "completed", "scope_verified": true, "trace_reviewed": true,
    "protocol_violations": [],
    "quality_verified": true, "quality": {"passed": true, "checks": []},
    "aide_operations": 0, "aide_uptake": false,
    "aide_duration_ms": null, "retrieval_output_bytes": null, "hook_metrics": null
  }]
}
```

`runtime_session_id` must match the collector's `runtime.actor_id` (the raw log's
`session_meta.payload.id`). If omitted, `agent_id` must match instead. Canonical
agent paths are not session identities. Status is `planned`, `completed`, `failed`
or `cancelled`; omitted planned rows remain `missing`. Review quantities require
explicit trace evidence; leave unknown fields null. `aide_operations` counts
actual nested aide operations, not outer tool wrappers. `aide_duration_ms` is
separate from whole-trial elapsed time. Hook metrics and source retrieval output
bytes are separate boundaries and must not be substituted for runtime counters.
Record treatment or other deviations in `protocol_violations`; a nonempty list
prevents comparisons. Reusing one runtime actor for two trials also prevents
comparisons because it cannot establish fresh contexts.

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v2/run.py report --ledger REVIEWED_LEDGER.json --output NEW_REPORT.json
rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v2 -p test_run.py
```

Reporting imports the existing `../collect_codex.py` for explicit raw captures,
runtime metadata, completion and counters. It retains every planned row and the
raw capture reference. Invalid captures or mismatched session identities leave
attributed usage unknown. Captured outer tool-call counts, reviewed aide operation
counts, quality, input/output/cache counters and elapsed time remain distinct.
Failed grades retain measured resources. Pair deltas are emitted only within a
task/repetition when both trials have verified scope and traces, completed with
passing independently verified grades, have known usage and match model,
provider, effort and CLI version. Missing evidence suppresses comparisons.

Results are descriptive observations from a small fixed pilot. There is no
aggregate efficiency score, billing estimate or causal/provider-savings claim.

The report CLI verifies the package manifest, the reviewed ledger’s protocol digest,
and the parent usage collector digest pinned in the protocol. It records all three
digests in the report. Preserve `protocol_sha256` when completing the prepared ledger.
