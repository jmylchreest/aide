# Offline v4 runner/protocol follow-up review

**Verdict: approve the scoped runner/protocol changes for freeze. No remaining blocker found in this scope.**

Reviewed `scripts/retrieval-quality/outcomes-v4/run.py`, `grade.py`, their two test files, and protocol grading/measurement declarations on 2026-09-09. This follows [the initial review](/tmp/aide-v4-protocol-review.md), which records the original findings and the 27 live project decisions checked individually. The fixes add no production/runtime changes or new decision-conformance issue. Source fixtures, task answers, rubrics' substantive correctness, README, and bridge preflight remain outside this follow-up's scope.

## Four original findings resolved

1. **Manifest/copy consistency:** `run.py:95-103` applies the same bytecode/cache exclusions to common and participant-overlay copies. The existing regression covers common template exclusions; an additional synthetic check confirmed newly added unmanifested `.pyc` and `__pycache__` contents in an overlay do not enter the copy.
2. **Independent implementation review:** `grade.py` now retains successful tests as `functional_quality` and leaves overall quality null/unverified. `run.py:161-193` reconstructs overall quality from complete boolean functional fields and an independent named review; it does not trust an externally supplied bare overall pass. `verified_report` applies this gate by the planned implementation IDs before the inherited comparison calculation (`run.py:196-205`).
3. **Read-only scratch additions:** `grade.py` now converts scratch additions on trace/impact into integrity failures. The new test exercises this without launching candidate code.
4. **Scratch-test count inflation:** the visible suite command is built from frozen baseline test files, rather than the participant's entire tests directory. The regression checks that a newly added scratch test is excluded from that command.

## Quality and comparison gate verification

I executed **21 focused synthetic scenarios**, using the real v4 `verified_report` and pinned inherited report with a mocked collector providing explicit synthetic runtime evidence. The check script is `/tmp/aide-v4-review-checks.py`; all scenarios passed. This supplements the root agent's reported 17 passing unit tests; I did not rerun that suite.

- Valid complete functional booleans plus a named reviewer different from the participant/session, the exact frozen criterion keys, true boolean checks, and nonempty evidence produced one eligible implementation pair.
- Removing review evidence, giving the participant's own ID or a blank reviewer, using nonboolean checks, adding or omitting criterion keys, or omitting/blanking evidence left overall quality null and unverified and removed the pair.
- Missing/nonboolean functional fields, inconsistent aggregate functional status, or a nonboolean verification flag likewise remained pending despite a supplied overall pass.
- Known functional, integrity, and complete independent-review failures remained verified failures and produced no comparison.
- Missing any model/provider/effort/CLI comparison dimension suppressed the pair.
- Reports retained all 12 planned rows, the functional results, and the complete independent-review object. The caller's ledger was not mutated.
- Participant-overlay bytecode/cache content remained excluded from copying.

The existing collector/report checks continue to govern actual actor identity, completion, fresh actors, scope/trace review, and runtime usage. The new implementation gate does not bypass them. Review names and cited evidence are controller-supplied records; the controller must ensure the reviewer really is independent and the citations substantiate the checks. The runner validates record completeness, not the truth of a human review.

## Remaining scope limits

This approval does not certify the task/reference answers, fixture-source provenance, test completeness, final package manifest, or bridge preflight. Those remain the root controller's freeze checks. The protocol still clearly discloses the pinned bridge transport and lack of per-agent native MCP binding, and disclaims causal savings, billing, and native-tool-presentation claims.

No participants were launched, no experiment usage was measured, and no production code was changed by this review.
