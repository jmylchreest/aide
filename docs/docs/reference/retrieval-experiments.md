---
title: Retrieval experiments
description: Reproduce deterministic retrieval checks without claiming provider token savings.
---

The retrieval experiment checks whether prescribed aide tool sequences return the
expected source evidence and how much text they produce. It uses temporary projects
and does not add synthetic observations to your project's token history.

From the repository's `aide/` directory, run:

```sh
go test ./cmd/aide -run '^TestRetrievalExperiment' -count=1 -v
```

To retain a JSON report, set `AIDE_RETRIEVAL_REPORT` to a new file path:

```sh
AIDE_RETRIEVAL_REPORT=/tmp/aide-retrieval-experiment.json go test ./cmd/aide -run '^TestRetrievalExperiment' -count=1
```

The report refuses to overwrite an existing file. Use your shell's environment
variable syntax on Windows. `-count=1` disables Go's test-result cache. Go, the
repository's normal native build prerequisites and available grammar packs are
required, as with the other retrieval tests. Tests use the standard temporary
directory; set `TMPDIR` to a clean writable directory if a parent project marker
affects local root resolution.

## What the report measures

Each scenario runs three times with a fresh project and index. The report retains
every returned text block, failed attempt, fallback, source checksum and individual
operation duration. A reference is one raw full-file read, counted once per trial.
The signed byte delta is reference bytes minus all result bytes. Positive means
less returned text; negative means overhead. Token deltas use aide's central
UTF-8 bytes/3 estimate, applied separately to the reference and each result.

Evidence checks cover a large-file outline followed by a symbol read, a small
file, ambiguous definitions followed by explicit selection, current source with
a stale index, a partially failed batch followed by a full-file read, and
unsupported content followed by a full-file read. The report also verifies
successful retrieval receipts against the exact source snapshot.

These checks do **not** measure model answer correctness, edit quality, provider
tokens, billing or cache effects. `model_quality` and `provider_usage` remain
`null`. A prescribed fallback is not a measured model fallback rate. Three
repetitions expose local variation; they do not establish statistical confidence.

Durations cover the in-process retrieval handler or local filesystem fallback.
They exclude startup, indexing, MCP transport, hooks and model time. Process,
grammar and filesystem caches are uncontrolled. Do not turn these timings into
an end-to-end latency improvement claim.

## Initial fixture results

The September 8, 2026 run produced identical text counts across all three
repetitions of each scenario. All 18 source-evidence checks passed.

| Prescribed sequence | Full-file reference | Total result bytes | Byte delta |
| --- | ---: | ---: | ---: |
| Large file: outline, symbol | 17,409 | 2,745 | +14,664 |
| Small file: outline, symbol | 52 | 246 | −194 |
| Ambiguous symbol, explicit selection | 134 | 314 | −180 |
| Stale index, current symbol | 51 | 142 | −91 |
| Partial batch, full-file fallback | 55 | 314 | −259 |
| Unsupported content, full-file fallback | 32 | 120 | −88 |

These deliberately chosen fixtures demonstrate reduction and overhead. They are
not a representative workload, and their results should not be averaged into
an aide savings headline. Rerun after retrieval or grammar changes rather than
treating these counts as permanent performance guarantees.

## Live collection verification

On September 8, 2026, the installed daemon identified itself as
`0.1.18-dev.7+f0efcc7`. Live Codex calls to `code_outline` and `code_read_symbol`
produced matching server and host records with the same receipt IDs, source
hashes and text sizes. The host records included session, actor and context epoch.
A simple shell file read also produced a verified full-file record.

This verifies those collection paths in that session, not complete host coverage
or final provider input. Its context-window comparison remained unavailable
because the session included unclassified shell activity and incomplete evidence.
That limitation must remain visible in the CLI and web Details view.

## Independent task comparison protocol

The next stage requires fresh model contexts, not another pass by a model that
already knows these answers. Freeze repository snapshots, task prompts and hidden
answer/test checks before running trials. Include navigation, debugging and edits,
with ambiguous symbols, stale indexes, shell searches, small and large files, and
unsupported content. Include a task where the outline alone is insufficient.

Compare ordinary retrieval with aide-assisted retrieval under the same model,
host, initial instructions and token budget. Counterbalance run order and keep
each trial's context isolated. Record the model/version, host/version, aide commit,
settings, trial IDs and all failures or abandoned runs. Optional stronger steering
is a separate experiment once it exists; it is not part of the current fixture.

Grade answers and edits against the frozen checks without revealing the treatment
to the grader. Report per-task correctness, missed evidence, retries, full-file
fallbacks, wall-clock completion time and actual provider usage when available.
Keep missing provider usage unknown, and keep cached input separate from uncached
input. A context reset or cache-clear event does not justify compounding an earlier
text reduction across later turns.

Publish raw trial results, sample counts and uncertainty before drawing a
recommendation. Do not promote steering based on text reduction alone if answers
or edits worsen. No independent model-quality comparison has been run by this
test harness.
