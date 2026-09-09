"""Render the completed pilot's descriptive findings without inventing savings."""
from collections import Counter
from pathlib import Path
import json

BASE = Path(__file__).resolve().parent
report = json.loads((BASE / "report.json").read_text())
rows = report["trials"]
if len(rows) != 12 or any(r["status"] != "completed" or not r["quality_verified"] or not r["trace_reviewed"] for r in rows):
    raise ValueError("All twelve outcomes and trace reviews must be present before final findings")
passed = sum(r["quality"]["passed"] for r in rows)
assisted = [r for r in rows if r["treatment"] == "assisted"]
uptake = sum(r["aide_uptake"] is True for r in assisted)
known = [r for r in rows if r["runtime_usage"] is not None]
totals = {k: sum(r["runtime_usage"][k] for r in known) for k in
          ("input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens")}
tools = Counter(call["tool"] for r in rows for call in (r.get("bridge_metrics") or {}).get("calls", []) if call.get("handler_requested"))
deviations = sum(bool(r.get("protocol_violations")) for r in rows)
headline = f"{passed}/12 outcomes passed the frozen quality checks; {uptake}/6 assisted trials chose aide."
qualification = (f"{deviations}/12 trials have recorded protocol deviations; the frozen reporter permits "
                 f"{len(report['comparisons'])} paired comparisons. Individual counters remain visible. No token-saving percentage is established.")
(BASE / "findings.json").write_text(json.dumps({"headline": headline, "qualification": qualification,
    "known_task_usage": totals, "known_usage_trials": len(known), "tool_calls": dict(tools)}, indent=2) + "\n")
lines = ["# Cross-file retrieval outcome pilot (v4)", "", headline, "", qualification, "",
         "[Open the compact report](index.html) · [Machine-readable evidence](report.json) · [Frozen protocol](../../outcomes-v4/README.md)", "",
         "## Outcomes", "", "| Task | Ordinary quality passes | Assisted quality passes | Assisted uptake |", "| --- | ---: | ---: | ---: |"]
for task in ("trace", "impact", "implement"):
    selected = [r for r in rows if r["task"] == task]
    ordinary = sum(r["quality"]["passed"] for r in selected if r["treatment"] == "ordinary")
    enabled = sum(r["quality"]["passed"] for r in selected if r["treatment"] == "assisted")
    chosen = sum(r["aide_uptake"] is True for r in selected if r["treatment"] == "assisted")
    lines.append(f"| {task.capitalize()} | {ordinary}/2 | {enabled}/2 | {chosen}/2 |")
lines += ["", "Each implementation was assessed with the same 4 visible and 20 hidden runtime checks, source integrity and five independent source-review criteria. Original Vitest suites and full TypeScript type checking were not executed. Optional participant scratch tests are retained separately and do not increase the frozen mandatory check count.", "",
          "Trace and impact grading checked factual routes, normalization, conflict handling, affected consumers and bounded claims. After answers were collected, the reviewer corrected an inconsistent interpretation of the existing prompt-scope rule while still blinded. [Initial grades and adjudication](quality-adjudication.md) are preserved; the frozen rubric was not changed.", "",
          "## What the measurements establish", "",
          f"The {len(known)}/12 trials with complete runtime counters consumed **{totals['input_tokens']:,} input tokens** and **{totals['output_tokens']:,} output tokens** in total. Cached input ({totals['cached_input_tokens']:,}) is a subset of input. Reported reasoning ({totals['reasoning_output_tokens']:,}) is a subset of this host's output. These are runtime counters, not billed amounts or estimated file tokens.", "",
          "These totals include participant instruction reads, retrieval, editing, tests, failures and retries. They exclude controller preparation, execution management, grading and reporting, which are recorded in the separate [overhead snapshot](overhead.json). Root context replay is evaluation overhead, not normal aide operating cost. Its timestamp cutoff excludes later responses and final delivery.", "",
          "Recorded aide operations: " + ", ".join(f"`{name}` {tools[name]}" for name in ("code_search", "code_references", "code_symbols", "code_outline", "code_read_symbol")) + ".", "",
          "Bridge evidence records scoped identity, returned text and local timings. Handler elapsed sits inside bridge elapsed and is not CPU time; index setup and controller exports are separate. Captured source-output bytes are a distinct boundary: truncated/unattributable results leave complete totals unknown and retain conservative lower bounds. No byte-to-token estimator replaces provider counters.", "",
          report["measurement_limitations"][0], "",
          "## Protocol limitation and next step", "",
          "The launcher pointed each participant to an instruction file. Initial bootstrap reads used raw shell commands before the participant had read the `rtk` requirement. This is a launcher design flaw, and the trace reviews retain the exact deviations. Authorized instruction-file reads remained within scope, but the frozen protocol does not exempt them from the prefix rule. No trial was coached, replaced or selectively rerun.", "",
          "The reporter therefore keeps the observed outcomes and counters but suppresses ineligible comparisons. Do not treat matching quality grades as general quality equivalence, fewer returned bytes as measured token savings, or greater uptake as an efficiency gain.", "",
          "For a future clean run, pass the complete prepared prompt directly at launch so the constraints are available before any tool call. Freeze that launch change before another balanced run, retain this dataset, and keep the same bounded outcome grading. The recorded prompt-scope interpretation should be made explicit in that future protocol. This report does not silently waive the current rules or replace failed protocol checks.", "",
          "The source fixture includes Claude Code and OpenCode adapters, but participants ran only in Codex. Access used an isolated stdio bridge, so this does not establish live cross-host performance or native MCP presentation effects. Provider cache was uncontrolled, even with fresh actors. Two repetitions per condition and one bounded source snapshot cannot establish a general savings rate.", "",
          "## Evidence", "",
          "- `ledger.json` preserves controller execution state; `reviewed-ledger.json` combines separate reviews.",
          "- `tNN-capture.json`, answers, tool results, provenance and patches preserve each actor's evidence.",
          "- `changes/tNN/` preserves changed and added files; final source inventories are in provenance records.",
          "- `tNN-trace-review.json` records scope, protocol deviations, nested calls, test execution and byte boundaries.",
          "- `quality-*.json` contains blinded criterion grades; `blinding.json` reveals the final mapping.",
          "- `blind-inputs/` retains the reviewed answers and check evidence; `blind-inputs-manifest.json` hashes the full reviewed corpus, including reconstructable source copies.",
          "- `bridge/tNN/` and server-work exports preserve setup, identity, receipts and timing evidence.",
          "- [Independent final audit](final-review.md), [browser checks](visual-check.json) and [artifact hashes](evidence-manifest.json) record the final verification boundaries.",
          "- The frozen package remains unchanged at commit `032b074`; production aide code was not modified by the trials.", ""]
(BASE / "README.md").write_text("\n".join(lines))
print(json.dumps({"passed": passed, "uptake": uptake, "deviations": deviations, "totals": totals, "tools": dict(tools)}))
