# Outcome pilot v3

This package defines 18 fresh-context trials: three tasks × three treatments ×
two repetitions, in the unchanged v2 order. Navigation follows multiple source
files and now explicitly requests the normalization warning already required by
its rubric. The debug task and small edit control retain their v2 fixtures and
requirements. `source_commit` describes those copied fixtures; guidance and tool
description provenance are recorded separately.

This is a new descriptive experiment. Broader retrieval access, clarified task
wording and an isolated bridge prevent treating v2 versus v3 as a causal before
and after comparison. No command here launches participants or providers.

## Freeze and prepare

Before running participants, the controller fills guidance provenance, verifies
the temporary binary digest in `retrieval_bridge`, validates seed/reference
grading, and freezes a SHA256 manifest covering this package. The live binary,
daemon and configuration remain unchanged.

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v3/run.py prepare .aide/state/evaluation/outcomes-v3 --binary /tmp/aide-retrieval-v3-next
```

Preparation verifies the manifest and binary and rejects existing destinations.
It copies only task templates, composes prompts and writes `ledger.json`; it
does not start a process or seed indexes. `{{ROOT}}`, `{{BRIDGE}}`, `{{BINARY}}`
and `{{EVIDENCE}}` are replaced across all prompt sections with absolute,
shell-quoted paths. Plain words such as ROOT are not substitutions. Each trial
gets its own root and external `bridge-evidence` directory path.

The controller then seeds **all 18 roots**, including ordinary conditions,
through the frozen bridge:

```sh
rtk proxy python3 /absolute/package/bridge.py prepare --root /absolute/trial/root --binary /tmp/aide-retrieval-v3-next --evidence-dir /absolute/trial/bridge-evidence
```

Preserve setup evidence separately from participant usage. Do not launch trials
until every isolated binding is verified. Never overwrite prior roots, captures
or evidence; retain failures and deviations without replacement runs.

## Participant boundaries

Ordinary uses the same shell/file tools without aide retrieval. Available and
guided can optionally use the bridge's five capabilities: `code_search`,
`code_references`, `code_symbols`, `code_outline` and `code_read_symbol`. Their
identical argument examples and invocation syntax are supplied in the prompt.
Guided additionally receives the production `buildWelcomeContext` Code Retrieval
prose and conditional structure/batch advice. Zero optional uptake is valid.

All native/global aide MCP tools and direct aide CLI calls are prohibited. The
external bridge is an execution-only exception to the root boundary; participants
must not read its implementation, binary, evidence directory, package, references,
graders or other roots. They must not inspect or alter controller-created `.aide`
state. Source searches exclude it with `--glob '!.aide/**'`. No internet,
subagents, commits, reindexing or daemon/config changes are allowed.

Indexes describe the initial fixture; search/reference candidates can be stale
after edits. Current-source retrieval verifies candidates. This limitation and
all bridge initialization/process costs belong in the interpretation.

## Grade and report

Navigation uses all five frozen rubric checks and source references, graded
independently and blinded to treatment and usage. Debug/edit run frozen visible
and hidden suites in disposable copies. Their integrity check excludes only the
root `.aide` controller directory and records that exclusion; it never copies
that directory into grading sandboxes. This exclusion does not authorize any
participant alteration: a separate trace audit must reject forbidden reads,
writes and tool calls. Other immutable files, added configuration and symlinks
remain guarded.

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v3/grade.py edit /absolute/trial/root --output /absolute/new-grade.json
rtk proxy python3 scripts/retrieval-quality/outcomes-v3/run.py report --ledger /absolute/reviewed-ledger.json --output /absolute/new-report.json
rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v3 -p 'test_*.py'
```

Complete a separate reviewed ledger, preserving `protocol_sha256`. Every row
keeps its planned identity and actual runtime actor (`runtime_session_id`), raw
capture path, status, scope/trace review, explicit protocol violations, independently
verified quality and optional-tool uptake. The report retains missing, failed and
invalid rows. `quality_verified` means grading occurred, including a failed grade.

Runtime input/cache/output/reasoning/total counters, response count, elapsed time,
outer tool calls, aide operations, source result bytes and hook observations are
separate measures. `bridge_duration_ms` records attributable bridge overhead;
`bridge_setup` records controller setup evidence independently. `bridge_metrics` preserves setup index, guard, startup, identity, request, exit and total timings, call outcomes and emitted text bytes. `aide_duration_ms` is handler duration only when measured; bridge wall time is not CPU or handler duration. Preserve precise
underlying receipts, including initialization, process and MCP timings where
available. Unmeasured fields remain null; no zero-cost inference follows from
zero tool uptake. Source bytes exclude test/edit output and protocol wrappers.

Reports verify the manifest, protocol digest and parent usage collector digest.
Comparisons require both trials to pass independently, comply with protocol,
complete, have known usage and match runtime model/provider/effort/CLI. Provider
cache state is uncontrolled. There is no aggregate efficiency score, billing
estimate, causal savings claim or cross-host performance conclusion.
