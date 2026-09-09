# Outcome pilot v2 pre-freeze review

Verdict: approve for the declared offline pilot. No blocking correctness or decision-conformance issues remain in the inspected files. This review precedes the final package manifest and participant execution; it does not certify trial results.

Scope: scripts/retrieval-quality/outcomes-v2/**, scripts/retrieval-quality/.gitignore, and the vitest.config.ts exclusion change. Live decisions were loaded using aide decision_list before this conformance pass; full returned text is retained at /tmp/outcomes-v2-decisions.md. No package files were modified by this review.

## Evidence

- E1: `rtk proxy git status --short` and `rg --files scripts/retrieval-quality/outcomes-v2` restricted changes to the evaluation package, Python cache ignore, and Vitest exclusion. No production source, grammar packs, release snapshots, plugin manifests, binary packaging, or installer changes.
- E2: Direct line-numbered inspection of run.py:31–120,127–246 and grade.py:15–40,47–76,79–124,127–145; aide code_outline called for both executable modules. Runner prepares template-only copies, rejects overwritten destinations, verifies participant-input hashes, retains planned rows, separates quality from usage, and requires comparable completed passing evidence for deltas. CLI verified_report verifies the package manifest and protocol/collector pins and records their hashes.
- E3: Protocol inspected in full: exactly 18 rows and 18 unique task/treatment/repetition cells; same declared model/effort, no replacements, fresh participants, optional aide uptake valid, shared hooks, uncontrolled provider cache disclosed, independent grading, no causal/billing/savings claims.
- E4: Navigation source/rubric traced across OpenCode hooks, TS normalization/writer, Go observation persistence, usage conflict aggregation, and legacy projection. Debug source/reproducer/hidden cases and reference inspected and validated. Edit source/reproducer/hidden cases and reference inspected, including true end-of-name and ASCII Unicode-folding boundaries. Task prompts now match scratch-file and debug baseline semantics.
- E5: `rtk proxy python3 -m unittest discover -s scripts/retrieval-quality/outcomes-v2 -p 'test_*.py'`: 19 tests passed after review fixes. Tests cover identity mismatch, unknown captures, missing/failed rows, no overwrite, frozen inputs, runtime comparability, zero uptake, reused actors, provenance tampering, immutable files, forbidden config, symlinks, timeouts, and output location.
- E6: `rtk proxy node ./node_modules/vitest/vitest.mjs list --filesOnly`: exit 0, 44 intended files, zero outcomes fixture paths. Direct Bun invocation failed in esbuild service initialization; the normal Node CLI succeeded. Vitest exclusion is outcomes-v*/**; .gitignore contains only __pycache__/ and *.pyc.
- E7: Pattern search across package Python/JSON/Markdown for subprocess/Popen/os.system/exec/eval/shell=True/network URLs/--store/scope:/random/git/chmod/flock/readlink/installed_plugins/TODO/FIXME. Execution is the offline Bun functional grader, not provider launch or shell-string evaluation; explicit argv and 120-second timeout are used. Trial instructions prohibit external roots, memories, global index lookup, internet and subagents.
- E8: aide findings_list(analyzer=deadcode,file=scripts/retrieval-quality/outcomes-v2,include_accepted=true,limit=100) returned no findings. This is supplementary, not proof of indexing coverage. Real CLI entry points and unittest/grader call paths were inspected directly. Read-only navigation snapshots intentionally serve the navigation task.
- E9: Collector digest in protocol matches ../collect_codex.py. Every debug source-hashes.json entry matches current files. Existing validation records show debug seed visible 0/1, hidden 6/11; reference visible 1/1, hidden 11/11. Root supplied updated edit validation; reference boundary behavior was independently checked during the earlier review.

## Decision conformance

Decisions loaded: 27. Each decision appears exactly once below.

- ➖ `pack-index-override` — not applicable: E1 contains no embedded/disk index or pack-loader change.
- ✅ `project-identity-guard` — conforms within scope: E1/E2/E7 show explicit supplied task roots and no new anchor resolver or memory-injection path; development preparation does not redefine project identity.
- ➖ `store-routing` — not applicable: E2/E7 show filesystem evaluation artifacts, not memory/decision store routing; no --store or audience-based placement mechanism.
- ➖ `survey-git-approach` — not applicable: E1/E7 show no new survey or runtime Git operations; reviewer Git commands are inspection tooling.
- ✅ `cross-platform-compatibility` — conforms within scope: E1/E4/E7 show no installed runtime changes; executable fixture TS retains Node/Bun filesystem/path APIs, while Python/rtk orchestration is declared development-only.
- ✅ `memory-isolation` — conforms within scope: E2/E3/E7 prohibit participant memory access and copy only task templates; no memory/state export or cross-project injection is added.
- ✅ `plugin-runtime` — conforms within scope: E1/E2/E7 keep fixture TypeScript on Bun; Python is offline development/grading tooling, explicitly outside this decision's runtime requirement.
- ➖ `skills-architecture` — not applicable: E1/E7 show no skill-manager, provider registry, fetching, or matching changes.
- ➖ `survey-entry-scope` — not applicable: E1/E4 introduce navigation evidence with source lines, not persisted survey entries.
- ➖ `survey-mcp-naming` — not applicable: E1/E3 expose no new MCP tools and do not rename survey tools.
- ✅ `terminology-axes` — conforms within scope: E7 shows no new scope: audience tags or --scope routing flag; scope_verified denotes ordinary review scope, which the decision explicitly permits.
- ➖ `max-depth-convention` — not applicable: E1/E2 show no max_depth API or project walker depth semantics.
- ➖ `pack-index-architecture` — not applicable: E1 shows no marker index, entrypoint pack, or grammar architecture change.
- ➖ `stored-id-determinism` — not applicable to index groupings: E1/E2/E3 show fixed protocol trial IDs and ordered rows, no stored module/community assignment. Temporary test-directory identities are not persisted code-index groupings.
- ➖ `survey-code-graph` — not applicable: E1/E3 show no stored/query-time graph change; global indexed retrieval is prohibited for participants.
- ➖ `versioned-docs-generated` — not applicable: E1 confirms no docs/versioned_docs files changed.
- ➖ `multi-arch-binary-distribution` — not applicable: E1 shows no binary package, resolver, downloader, or npm dependency changes.
- ✅ `no-aspirational-code` — conforms: E2/E4/E5/E8 establish real preparation/report/grading entry points and executed tests; fixture snapshots are consumed by explicit evaluation tasks, not dormant proposed production features.
- ➖ `non-language-detection` — not applicable: E1 shows no grammar directories or marker labels changed.
- ➖ `plugin-version-pinning` — not applicable: E1 shows no opencode.json installer or MCP bunx version changes.
- ➖ `survey-analyzers` — not applicable: E1/E7 show no analyzer constants, schema, or survey execution changes.
- ➖ `survey-run-identity` — not applicable: E1/E2 show evaluation provenance hashes rather than survey entry metadata or freshness storage.
- ➖ `survey-store-location` — not applicable: E1/E2 show no BoltDB/Bleve survey store change; evaluation roots are ordinary artifact directories.
- ➖ `abandoned-approaches` — not applicable: E1/E7 show no memory categories, templates, or automated abandonment detection.
- ➖ `grammar-detection-dual-pass` — not applicable: E1 shows no project-marker or file-scan grammar detection changes.
- ➖ `hud-wrapper-root-resolution` — not applicable: E1/E7 show no HUD wrapper, pointer file, marketplace resolver, or installed_plugins parsing changes.
- ➖ `memory-scoring` — not applicable: E1/E7 show no memory scoring, stored score, decay, weighting, or environment-switch change.

## Findings resolved before freeze

1. Unfrozen external collector/report provenance: fixed by verified_report checking and recording protocol, collector and package-manifest digests; provenance tamper regression passes (E2/E5/E9).
2. Grader executed forbidden configs after integrity failure: fixed by returning explicit unexecuted failing checks before copying or running code. Tampered test, extra bunfig, missing source and symlink regressions pass (E2/E5).
3. Prompt authorized broader scratch files than grader accepted: both prompts now explicitly permit only new non-src *.test.ts/*.spec.ts files (E4).
4. Edit reference accepted non-ASCII Kelvin sign after lowercasing and had an end-anchor issue: original-string ASCII matching and strict end validation now align with prompt; corresponding hidden negatives added (E4).
5. Debug first-baseline wording conflicted with a later recovery-veto full result: prompt now selects the most recent matching full-output call and preserves earlier full targets only through successfully shortened repeats (E4).
6. Navigation failed-write scenario lacked persistence specificity: prompt now states the failure occurs before persistence (E4).

## Security and validity limits

No provider or participant launches occurred in this review. Runner uses explicit log paths and strict parsing; it does not discover runtime logs. Grading runs candidate code in a disposable copy, rejects known integrity violations before execution, and preserves raw roots. A disposable directory is not an adversarial OS sandbox: allowed source code still executes with local process privileges, so this approval assumes the declared cooperative offline pilot and retained trace review.

Quality_verified, scope_verified, trace_reviewed, and reviewed aide/retrieval/hook measurements remain reviewer assertions supported by external artifacts; the reporter does not independently prove them. Unknown values stay null. Successful functional checks do not themselves establish task quality beyond their declared contracts. No unsupported assurance of billing, total provider coverage, or causal savings is made.

Post-freeze verification: the final package manifest now contains 43 entries; all 43 file SHA256 digests match. The review contains exactly 27 individually evidenced decision entries. No participant trials have started. Participant/grade/trace evidence remains subsequent execution work. Approval is for committing and running this frozen protocol, not for any future result claim.
