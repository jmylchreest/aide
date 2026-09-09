# Independent final evidence audit

Reviewer: `/root/v4_final_audit`. Scope: evidence accounting and final claims, not a second participant-quality grade. No candidate code, new trials, participant messages or frozen-package changes were executed. Only provenance-selected runtime logs were read. Review completed against the final report with the additive fractional-duration limitation.

**Disposition: the descriptive report is supported, with the limitations below. No unresolved publication blocker was found. There are zero protocol-eligible comparisons, and no savings estimate is established.**

## Independently verified

- The package inventory matches every frozen manifest hash, and all four pinned helper hashes match. `git diff 032b074` for the frozen package and inherited helpers is empty; the working-tree status contains only the new results directory, with no production changes.
- The report contains exactly the 12 planned task/condition/repetition cells, in frozen order, with 12 distinct actual actor IDs. Raw session metadata and turn contexts agree on `gpt-6-astra`, `openai`, `high`, CLI `0.153.4`. Each raw log matches its full SHA256 in `tNN-provenance.json`; all actors completed before the next started, with observed gaps of 13–21 seconds. Saved launch records use fresh, unforked actors. Plaintext launch-message equality is not claimed where the host stores encrypted task messages.
- All 108 unique participant response records match captures, per-actor cumulative counters and final report rows. Independent sums: **3,542,763 input**, including **3,091,584 cached**; **62,246 output**, including **3,440 reasoning**; **0 cache-write input**; **3,605,009 total tokens**. Cached input and reasoning output are subsets, not additional totals. Findings and HTML use these counters without byte conversion.
- Every final source inventory reconstructs exactly from the frozen common template, task harness and preserved changed/added files. The eight read-only inventories contain 50 files. Each implementation inventory contains 56 files, with six changed original source/test files plus one preserved scratch test. No deletions are recorded. Snapshot evidence proves retained bytes and integrity, not unrestricted application correctness.
- All four implementation grade artifacts contain exactly four visible and 20 hidden `(pass)` records, successful exit codes and no failing/skipped/todo records. Each has all five exact independent source-review criteria with evidence. The eight answer reviews contain the frozen criterion sets. The report correctly separates these quality passes from protocol validity. Original Vitest suites and full TypeScript checking remain unexecuted; scratch checks do not increase the mandatory 24 checks. Grading adjudication was recorded after answers were collected; initial reviews remain available and the reviewer remained blinded. This audit verifies the evidence linkage, not the substantive regrading decision.
- Each of the 12 trace reviews supplies a nonempty string list of protocol deviations, totalling 16 bootstrap-prefix violations. The frozen reporter consequently marks every trial invalid and emits no comparisons. The separate t09 pipeline interpretation does not become an extra enforced rule.
- Host call inputs reconcile to 30 valid scoped bridge calls: 14 `code_symbols`, 14 `code_read_symbol`, two `code_outline`, and zero search/reference calls. The two four-element mapped batches in each of t04/t09 were counted by executed cardinality, not textual command occurrence. All six assisted actors used retrieval; ordinary actors used none. Binding, identity, returned-text hashes, unique server work receipts, referenced source hashes and exact matching snapshot byte sizes agree.
- Recorded handler time totals **113 ms** inside **2,874.813302 ms** summed bridge elapsed time; every individual handler duration is within its request and bridge interval. Parallel call sums are accumulated elapsed durations, not experiment wall time or CPU. Bridge returned text totals **98,289 UTF-8 bytes**, a different boundary from all source-output bytes and runtime tokens. Setup and observe exports remain separate. Unknown source-output totals for t01/t12 remain null, with lower-bound evidence retained; no-handler measurements remain null.
- The overhead snapshot contains eight nonparticipant actors and 471 selected responses. Every recorded snapshot SHA256 was matched to an exact line-aligned prefix of its growing raw log. Recomputed response-window sums and phase splits agree: **43,167,371 input**, **42,197,248 cached input**, **198,372 output**, **37,763 reasoning output**, **0 cache-write input**, **43,365,743 total**. Preparation input/output is 10,941,494/74,365; execution/review/reporting input/output is 32,225,877/124,007. The explicit cutoff is `2026-09-09T16:39:52.360054+00:00`; subsequent responses, including this audit’s closing responses and final delivery, are excluded. This is evaluation overhead with root context replay, not normal aide operating cost.

## Helper and claim limitations

`assemble_report.py`, `capture_overhead.py`, `write_findings.py`, `bridge_summary.py`, the frozen report gate and HTML renderer were inspected. Their sums, quality gating, missing-value handling and current dataset claims reconcile, with one discovered compatibility limitation now disclosed: the inherited frozen reporter accepts only integer top-level `bridge_duration_ms`, so all six assisted top-level values are null despite known fractional timings. The valid measurements remain in `bridge_metrics.timings_ms.total.total`; HTML uses those nested values. `report.measurement_limitations`, README and HTML now explain this without rounding or altering the frozen runner.

`t01` has no separate prelaunch binding artifact; its setup explicitly reports `expected_binding_verified: false`. Its root and binary digest agree with the ledger/protocol, and it made no bridge calls. This narrower evidence is not represented as independently verified prelaunch binding.

The README/findings/HTML preserve the distinction between bounded quality passes and invalid comparison protocol, uncontrolled provider cache, Codex-only execution and isolated stdio transport. They make no general quality-equivalence, billing, cross-host or token-saving claim. Browser layout/interaction checks were performed separately by the controller, not repeated in this audit.

## Per-trial cross-check

Full raw-log hashes and final file inventories are retained in each trial's provenance record. Hash prefixes below identify those independently verified records.

| Trial | Responses | Final files | Preserved files | Prefix deviations | Aide calls | Raw SHA256 prefix |
|---|---:|---:|---:|---:|---:|---|
| t01 | 9 | 50 | 0 | 1 | 0 | `89a49e6502fbacbb` |
| t02 | 9 | 50 | 0 | 1 | 1 | `a55ca6ce1907e467` |
| t03 | 8 | 50 | 0 | 2 | 0 | `a12e139f889757f1` |
| t04 | 9 | 50 | 0 | 1 | 13 | `7931e40eaa9e3cde` |
| t05 | 8 | 56 | 7 | 1 | 0 | `98dfd5c21ee5ce6e` |
| t06 | 9 | 56 | 7 | 1 | 2 | `3572f8b9b145118a` |
| t07 | 10 | 56 | 7 | 1 | 2 | `1bcb0ba69e61d1b4` |
| t08 | 9 | 56 | 7 | 1 | 0 | `ed2ff3776b7f2c29` |
| t09 | 10 | 50 | 0 | 2 | 10 | `a363b703756bcd3c` |
| t10 | 8 | 50 | 0 | 2 | 0 | `69e11d3940f2b263` |
| t11 | 9 | 50 | 0 | 2 | 2 | `b5256bdf75432973` |
| t12 | 10 | 50 | 0 | 1 | 0 | `4997e15fcbd2684a` |

Audit input report SHA256: `dbb06c8733d28e23ba45ca58639a71c68eae76d25658087d34e6f7448aa632ef`.
