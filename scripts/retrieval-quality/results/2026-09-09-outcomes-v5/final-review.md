# Independent final evidence audit — v5

Reviewer: `/root/v5_final_audit`. Scope: accounting, protocol evidence, report gates and claims; no participant execution, coaching, answer regrading or production changes. The controller separately performed browser QA, recorded in `visual-check.json`.

**Verdict: accepted for this bounded descriptive pilot, with the measurement limitations below. No unresolved accounting or report-claim blocker was found.** All 12 planned outcomes remain visible; all 12 completed, passed the recorded independent quality assessment and met the observed protocol. All six same-task/repetition pairs qualify for descriptive comparisons. This verdict covers the report and snapshot hashes below; final evidence packaging and commit are subsequent controller work.

## Verified execution and quality evidence

The unchanged v5 package and inherited v4 corpus/helpers pass frozen-manifest verification. Participant raw metadata identifies freeze commit `6588483d373dd32fcb41bcda1be968fb6847f91f`. No production or frozen v4/v5 package differences were observed against that commit.

All twelve planned cells are unique and in the frozen counterbalanced order. Actual parent spawn records show exactly twelve launches, `fork_turns="none"`, no model/effort overrides, and no participant coaching/followup/interruption calls. Each actual spawn occurred after the preceding participant completed. Provenance-selected complete raw-log SHA256 values match the saved records. The twelve actual actors are distinct, at `/root/v5_execution/v5_t01` through `/root/v5_execution/v5_t12`; raw session and turn metadata consistently report `gpt-6-astra`, `openai`, `high`, CLI `0.153.4`.

Regenerated frozen prompts, prepared prompts, prelaunch messages, archived UTF-8 files and controller launch records are byte-identical with matching hashes for all twelve cells. Each initial inventory equals the frozen template plus the implementation participant harness where applicable, and its seed binding. Every final inventory reconstructs from that baseline plus preserved `changes/tNN` files. Read-only tasks retain 50 files; implementation tasks retain 56, including their scratch test. Actual shell inputs independently match the trace reviews: 243 nested commands across 80 outer calls, all submitted command strings beginning with `rtk`. The declared v5 pipeline rule is applied consistently.

Quality and protocol validity are separate report fields. The four implementation grades each retain exactly 4 visible and 20 hidden mandatory passes, no failures or reported skipped/todo tests, plus five complete independent source-review criteria. Trace and impact reviews retain the frozen criterion IDs and the clarifications declared before exposure. Participant Bun execution and validation claims are separately evidenced; controller hidden grading is not attributed to participants. This audit checks those evidence gates, not the substantive answers again.

## Independently recalculated participant accounting

Raw unique response counters were summed directly, without relying on recollection by the existing collector alone. All 92 response IDs reconcile with saved response lists, per-actor cumulative counters and report totals; usage is known for 12/12 trials.

| Counter | Total |
| --- | ---: |
| Input | 3,249,552 |
| Cached input, included in input | 2,783,232 |
| Cache-write input | 0 |
| Output | 59,934 |
| Reasoning, included in this host's output | 3,342 |
| Input plus output | 3,309,486 |

Subset inequalities and input-plus-output totals hold at each response. Cached input and reasoning are not added again. No source-byte estimate substitutes for runtime counters.

All six report deltas were independently recalculated. Findings correctly normalize **assisted minus ordinary**, including reversed execution order in repetition two. Input is lower in 2/6 pairs and higher in 4/6; output is higher in 5/6, and elapsed time is longer in 5/6. The inherited gates require completed, protocol-valid, quality-passing rows and equal runtime dimensions. Synthetic report preflight records excluded, reversed-order, zero-baseline and unknown-quality cases. The final report retains per-pair observations without pooling them into a savings percentage or making causal/billing claims.

## Retrieval and unknown coverage

All 19 observed bridge requests reconcile one-to-one with scoped records and unique server work receipts: search 0, references 1, symbols 5, outline 4, read-symbol 9. All six assisted actors use the bridge; ordinary actors do not. No bridge errors, rejected requests, tool errors or invalid receipts are recorded. Scoped root/database/socket identities, response-text hashes and available source receipt hashes/full-file byte counts validate independently.

Verified handler elapsed totals 137 ms. Accumulated bridge elapsed totals 1,130.679008012521 ms, including handler time; neither is CPU time, and accumulated parallel durations are not wall time. Fractional bridge measurements remain in `bridge_metrics.timings_ms.total.total`; the frozen integer-only top-level field remains null where appropriate. Index setup and controller exports remain separate.

Complete source-output bytes remain null for t01, t03, t11 and t12 because outer truncation/splicing prevents complete attribution. Independently recounted conservative source lower bounds sum to 985,274 bytes; the eight complete rows sum to 645,361 bytes. Missing text is not zero. One command exit status is unavailable in each of t03, t11 and t12; no observed failure does not certify those missing statuses.

Actual delivered prompt plaintext equality remains unknown because both parent spawn message fields and participant task bodies are encrypted. Controller intended-byte equality is verified, not decryption. Installed-hook standalone duration and total model-delivered context bytes are also unknown. Original Vitest execution, semantic TypeScript checking and standalone transpilation are absent from participant implementation traces. t06 has formatting edits after its participant Bun run and no subsequent participant validation; that boundary is retained. Fresh actors do not prove cleared provider caches. This bridge pilot does not establish native MCP UX, live Claude Code/OpenCode performance, general quality equivalence or transferable token savings.

## Separately verified evaluator snapshot

The evaluator window is `2026-09-09T17:16:12.424Z` through `2026-09-09T17:51:04.930469+00:00`; execution begins at `2026-09-09T17:21:24.992104+00:00`. The start matches the root user's recorded continue event. All eight included actor logs match their exact snapshot-prefix hashes even where logs subsequently grow. Independent raw response selection and sums match every actor, phase and aggregate. Participant identities are excluded; explicitly reused v4 evaluator aliases contribute only responses in this v5 window. No actor total is unknown.

Evaluator totals are **40,468,928 input**, including **39,724,288 cached**, and **146,490 output**, including **24,597 reasoning**; cache-write input is zero and input plus output is **40,615,418**. Preparation contributes 6,831,696 input / 23,589 output; execution/review/reporting contributes 33,637,232 input / 122,901 output. Phase assignment uses response completion timestamps. Root prior-context replay is evaluation cost, not ordinary aide runtime cost. Responses after cutoff, including later audit/packaging work and final delivery, are explicitly excluded.

## Artifact identity

- `report.json` SHA256: `7a5373714fd07a5ff12c6f458c91daa257c97232a1a891cfff7dd48eecb713bb`
- `overhead.json` SHA256: `dcf9e98f502f87d0f31da5bf7aaf196572061830734842214dc1deaebcdf6d5d`

The final findings, README and renderer agree with these report totals and comparison directions. Browser evidence was inspected as controller-provided QA, not independently repeated by this accounting reviewer. V4 results are retained separately and are not pooled with v5.
