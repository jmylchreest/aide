# Independent v5 preflight review

Reviewer: `/root/v4_final_audit`. Reviewed before participant exposure: `run.py`, `launch.py`, `protocol.json`, `grading-clarifications.md`, `README.md`, and the 14 offline tests in `test_launch.py`. No participants or candidate code were run by this reviewer; no implementation files were changed by this review.

**Disposition: no unresolved blocker to freezing this bounded rerun.** Regenerate the v5 manifest to include this review, verify it, and commit the freeze before preparing actual trial roots. This is a review of the launch repair and evidence safeguards, not a claim that future participants will follow the protocol.

## Scope and frozen choices

V5 pins and reuses the unchanged v4 corpus manifest, source fixture, binary digest, inherited helpers, twelve task/condition/repetition cells, counterbalanced order, treatment guidance, check IDs and mandatory implementation checks. The task-text adjustment only changes the visible-test command spelling to `rtk proxy bun test tests`. Results must remain separate from v4; no pooled or causal comparison is justified.

The complete prepared prompt is delivered directly in `spawn_agent.message`, beginning with the RTK requirement and an authorized prefixed bootstrap example. Prompt generation and preflight compare the saved UTF-8 prompt with the exact regenerated message and its ledger hash. A pointer-only or modified message cannot pass that comparison. The controller archives the full message before launch; it must actually submit those exact bytes to the collaboration tool.

The chosen prefix rule explicitly applies to each submitted shell-command string; downstream pipeline stages need not repeat the prefix. That declared boundary is accepted for this run and must be applied consistently by trace reviewers. A stricter per-segment rule is not part of v5.

The grading document fixes the previously adjudicated prompt-scope interpretation before answers exist. Trace logging/final-newline omissions and impact text-skill-handler enumeration have the stated bounded treatment; explicitly incorrect claims still count. Impact's requested wire/logging requirements and all other checks remain. Independent reviewers must receive the clarification document while remaining blinded to condition and resources.

## Launch guards

Preflight verifies the frozen package and exact ledger plan; rejects out-of-order, duplicate, already-started and later-active cells; rejects an orphan launch-message artifact before creating prelaunch evidence; and requires previous distinct actors to have complete captured evidence. Prior ledger, provenance and capture identities/paths must agree, the raw-log SHA256 must match, and recollecting the raw log must reproduce the saved capture. This closes the initially identified reliance on saved completion assertions alone.

Each launch, including t01, requires a matching isolated binding and frozen binary, an evidence directory containing only setup, and an exact source inventory matching both the original template/task harness and the seeded inventory. New launch records use exclusive creation and reject a reused actor path. Actual runtime actor IDs, dimensions and sequential timing still require capture review; an actor-path label alone does not prove runtime identity.

## Verification and limits

The controller reported 14 passing offline tests; their code was independently inspected. They cover full-message archival, missing/modified/pointer prompts, source/seed/binary differences, prior bridge calls, order/duplicate/later-cell/orphan guards, missing completion evidence, normalized test spelling and retained nested fractional durations. These tests mock package verification and bound bridge configuration; they do not replace real per-root preflight checks. The controller additionally confirmed that a selected completed v4 t12 capture equals deterministic recollection from its provenance-selected raw log. This reviewer separately ran `run.py verify` successfully against the provisional v5 manifest and unchanged v4 package/helpers.

The inherited integer-only top-level `bridge_duration_ms` limitation is explicitly retained. Fractional values remain in `bridge_metrics.timings_ms.total.total`; reporting must identify and use that boundary without rounding or converting missing values to zero. Handler time remains inside bridge time, while setup, source bytes, runtime counters and evaluation overhead remain separate.

Saved launch records are controller evidence of asserted spawn arguments; encrypted participant task messages do not independently establish plaintext equality. Preserve the actual parent spawn-call input during execution review. Deviations, failures, missing evidence and all planned cells must remain visible; no coaching, replacements or selective reruns are authorized by this review.

## Reviewed file hashes

These hashes identify the reviewed implementation and instructions; the regenerated manifest will also cover this review.

- `run.py`: `ae9e266693755401b717fb8af7ad670c9278e159f7d4ef956fc11e9c099e4f69`
- `launch.py`: `4807c22b821f17011da2a030ef6bc6f390ac45f66f1c8614b8c2d4d4792e7292`
- `protocol.json`: `b1f965bd2d3ed1f8090a8b40122c60c1261136bd5005fc41b2c8f436b31e3aad`
- `grading-clarifications.md`: `74e174cbd2f19add801c9040b09d48fc2a7a71c44e0c4197550ac942a8c0f4ad`
- `README.md`: `1ecd77ed6d58f0c9abc7f8d376db6ba6a2532666ad171c4a41dd8bf3e681fbb0`
- `test_launch.py`: `edbba4e84f431d398ad367f2a153b83b7a4ae34595f12337e139fcef45600a02`
