# Focused implementation comparison: v6

**4/4 implementations pass the quality checks. Neither assisted trial used aide retrieval.** Input was lower in both assisted-designated trials, but this run does not demonstrate token savings from aide calls or work offloaded to its retrieval tools.

[Compact report](index.html) · [Counters and evidence](report.json) · [Frozen protocol](../../outcomes-v6/README.md) · [Independent final audit](final-review.md)

## Results

| Trial | Condition | Responses | Input | Cached input, included | Output | Seconds |
|---|---|---:|---:|---:|---:|---:|
| t01 | Ordinary, repeat 1 | 8 | 252,291 | 227,840 | 5,123 | 100.522 |
| t02 | Assisted, repeat 1 | 6 | 172,592 | 152,064 | 4,852 | 92.640 |
| t03 | Assisted, repeat 2 | 7 | 208,384 | 182,528 | 4,646 | 87.899 |
| t04 | Ordinary, repeat 2 | 8 | 250,537 | 225,408 | 5,271 | 108.288 |

Both pairs pass protocol and quality gates and match actual runtime dimensions: gpt-6-astra / openai / high / CLI 0.153.4. Every trial uses a distinct actor; no replacements or coaching occurred.

| Assisted minus ordinary | Repeat 1 | Repeat 2 |
|---|---:|---:|
| Input tokens | −79,699 (−31.6%) | −42,153 (−16.8%) |
| Cached input tokens | −75,776 | −42,880 |
| Uncached input tokens | −3,923 | **+727** |
| Output tokens | −271 | −625 |
| Elapsed seconds | −7.882 | −20.389 |

These are descriptive runtime differences, not prices or causal savings. The second pair illustrates why total input alone is insufficient: cached input fell while uncached input rose. No byte estimator or assumed cache discount is substituted for measured counters.

## Retrieval and work quality

All four used ordinary searches and direct reads. All four have **zero observed unchanged implementation-body overlap commands** under the protocol's definition. Rereads after edits, caller checks and new surrounding context are preserved as verification or additional evidence, not labeled wasted work. The old symbol-body/direct-read duplication was absent; with zero aide calls, this is not a demonstration of better combined use of those methods.

Source output bytes were 50,108 / 40,061 / 56,091 / 52,295 for t01–t04. The second assisted trial returned **more** source bytes than its paired ordinary trial while using less total input. It had seven model responses versus eight; source bytes alone do not explain whole-task use. First edits occurred in responses 3 / 3 / 4 / 4. None of these observations identifies counterfactual avoided responses or tokens.

Every candidate passes 4 visible and 20 hidden runtime checks, protected-file integrity, and all five independently reviewed source criteria. The [blinded quality comparison](quality-comparison.json) found no substantive production-correctness difference. Optional tests differ in grouping and assertions, so test counts are not a quality ranking.

- t01 ran seven temporary contract checks, removed its own scratch file, then reran the four visible checks. The original test definitions remain in captured tool input; the blind reviewer did not receive their removed source and records that limitation.
- t02 and t03 retained four and six contract checks and ran runtime tests after their final edits.
- t04 retained seven contract checks, then formatted source and parsed all six migrated TypeScript files. That syntax check adds verification of files outside the runtime suite, but is not semantic type checking. It did not rerun participant runtime checks after formatting; controller checks pass the final candidate.
- None ran the original Vitest suites or semantic TypeScript checking. All final reports state those limits accurately.

## What this supports

Keep retrieval optional and evidence-driven. This fixture shows correct completion with direct reads under the revised guidance. It provides no reason to require more aide calls or to publish a tool-savings percentage. It also does not establish that the guidance caused fewer responses: execution choices, added instructions, common host context and caching remain possible influences.

The next useful evaluation would use different realistic tasks with substantial caller discovery or selective reading in larger files, keeping quality and total resources as the criteria. This task was used to motivate the change and is not held out. Further prompt tuning on this one fixture would offer weak evidence of general value. No additional product code changes were made in this run.

## Measurement and provenance

Known participant totals: **883,804 input**, including **787,840 cached input**, and **19,892 output**, including **2,165 reasoning output** tokens. The 903,696 total is input plus output; cached and reasoning subsets are not added again. All participant instructions, exploration, edits, checks and final responses count.

The pinned aide binary indexed all four pristine roots before execution, around 192–194 ms per index operation. No participant invoked a bridge or retrieval handler, so there is no participant retrieval-handler work measurement. Setup/export work and the [timestamped evaluation overhead](overhead.json) remain separate. That overhead includes root context replays and preparation, controller, source-review and audit work; it is not normal aide runtime cost. Its snapshot excludes responses completing after its explicit cutoff, including capture/delivery work, and must not be called a complete end-to-end cost.

Frozen v4 source/calibration is reused through verified v5 manifests. The v6 protocol and capture helpers were committed as `dc5fc4b` before participants. The same bridge capability wrapper is available to both assisted trials; ordinary trials explicitly prohibit it. Exact launch text is archived and checked against regenerated prompts. Raw logged message fields can be encrypted, so archive equality is not independently decrypted plaintext delivery verification.

The current host may supply common guidance outside those messages. The assisted package also removes v5's supplementary navigation paragraph. This is not a clean old/new wording comparison, installed/uninstalled aide comparison, or native MCP presentation test. Historical v5 results are not pooled. Fresh actors do not establish cleared provider KV caches. These Codex observations do not establish Claude Code or OpenCode performance.

## Reproduce and inspect

```sh
rtk proxy python3 scripts/retrieval-quality/outcomes-v6/run.py verify
rtk proxy python3 scripts/retrieval-quality/results/2026-09-09-outcomes-v6/verify_results.py
rtk proxy python3 scripts/retrieval-quality/results/2026-09-09-outcomes-v6/assemble_report.py
rtk proxy python3 scripts/retrieval-quality/results/2026-09-09-outcomes-v6/render_report.py
```

Assembly recollects the locally retained participant logs and reads preserved final fixture roots; those local inputs are required for full reproduction. Archived captures, changes, hashes, response associations and trace reviews remain independently inspectable without executing captured commands. [Reviewed inputs](blind-inputs-manifest.json), [artifact hashes](evidence-manifest.json), and [browser checks](visual-check.json) document the retained evidence and UI verification limits.

Unchanged reviewed source files are retained through hash-verified references to the frozen v4 corpus, alongside each candidate's preserved changes. `package_evidence.py` verifies every reconstructed file against the exact blind-review input before packaging. Saved `.patch` files retain their blank context lines; Git's whitespace checker reports those patch-format spaces, so they are preserved as evidence.
