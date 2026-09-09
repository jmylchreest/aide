"""Describe balanced pilot outcomes and eligible pairs without causal savings claims."""
from collections import Counter
import json
from pathlib import Path

BASE = Path(__file__).resolve().parent
report = json.loads((BASE / "report.json").read_text())
rows = report["trials"]
if len(rows) != 12 or any(r["status"] not in ("completed","failed","cancelled") or not r["trace_reviewed"] for r in rows):
    raise ValueError("All twelve terminal outcomes and trace reviews are required")
passed = sum(r["quality_verified"] and (r["quality"] or {}).get("passed") is True for r in rows)
graded = sum(r["quality_verified"] for r in rows)
assisted = [r for r in rows if r["treatment"] == "assisted"]
uptake = sum(r["aide_uptake"] is True for r in assisted)
known = [r for r in rows if r["runtime_usage"] is not None]
totals = {k:sum(r["runtime_usage"][k] for r in known) for k in (
    "input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens",
    "reasoning_output_tokens", "total_tokens")}
tools = Counter(c["tool"] for r in rows for c in (r.get("bridge_metrics") or {}).get("calls",[]) if c.get("handler_requested"))
eligible = {frozenset((c["left"],c["right"])) for c in report["comparisons"]}
pairs = []
for task in ("trace","impact","implement"):
    for repetition in (1,2):
        selected = {r["treatment"]:r for r in rows if r["task"]==task and r["repetition"]==repetition}
        ordinary, aided = selected["ordinary"], selected["assisted"]
        pair = {"task":task,"repetition":repetition,"ordinary":ordinary["id"],"assisted":aided["id"],
                "eligible":frozenset((ordinary["id"],aided["id"])) in eligible}
        if pair["eligible"]:
            left,right = ordinary["runtime_usage"],aided["runtime_usage"]
            pair.update(input_delta=right["input_tokens"]-left["input_tokens"],
                        input_percent=100*(right["input_tokens"]-left["input_tokens"])/left["input_tokens"] if left["input_tokens"] else None,
                        output_delta=right["output_tokens"]-left["output_tokens"],
                        elapsed_ms_delta=aided["elapsed_ms"]-ordinary["elapsed_ms"])
        pairs.append(pair)
headline = f"{passed}/12 outcomes passed the frozen quality checks ({graded}/12 graded); {uptake}/6 assisted trials chose aide."
qualification = (f"{sum(r['protocol_valid'] for r in rows)}/12 trials met the protocol; "
                 f"{len(eligible)}/6 pairs qualify for descriptive comparisons. Two repetitions per task do not establish a general savings rate.")
measured = [p for p in pairs if p["eligible"]]
resource_summary = (f"Assisted input was lower in {sum(p['input_delta'] < 0 for p in measured)}/{len(measured)} eligible pairs "
                    f"and higher in {sum(p['input_delta'] > 0 for p in measured)}/{len(measured)}. "
                    f"Output was higher in {sum(p['output_delta'] > 0 for p in measured)}/{len(measured)}; "
                    f"elapsed time was longer in {sum(p['elapsed_ms_delta'] > 0 for p in measured)}/{len(measured)}. "
                    "These counts describe this pilot and do not pool token differences into a savings percentage."
                    if measured else "No eligible pairs establish a resource difference.")
findings = {"headline":headline,"qualification":qualification,"known_task_usage":totals,
            "resource_summary":resource_summary,"known_usage_trials":len(known),"tool_calls":dict(tools),"paired_observations":pairs}
(BASE / "findings.json").write_text(json.dumps(findings,indent=2)+"\n")
lines = ["# Full-prompt retrieval rerun (v5)","",headline,"",qualification,"",resource_summary,"",
         "[Compact report](index.html) · [Machine-readable report](report.json) · [Frozen protocol](../../outcomes-v5/README.md)","",
         "## Outcomes","","| Task | Ordinary quality passes | Assisted quality passes | Assisted uptake |",
         "| --- | ---: | ---: | ---: |"]
for task in ("trace","impact","implement"):
    task_rows = [r for r in rows if r["task"]==task]
    counts = [sum(r["quality_verified"] and (r["quality"] or {}).get("passed") is True for r in task_rows if r["treatment"]==t) for t in ("ordinary","assisted")]
    chosen = sum(r["aide_uptake"] is True for r in task_rows if r["treatment"]=="assisted")
    lines.append(f"| {task.capitalize()} | {counts[0]}/2 | {counts[1]}/2 | {chosen}/2 |")
lines += ["", "Implementation quality combines 4 visible and 20 hidden mandatory runtime checks with five independent source-review criteria. The original Vitest suites and full TypeScript checking are outside the offline harness; participant validation claims are reviewed against recorded execution. Scratch checks are separate from mandatory checks.", "",
          "Trace and impact use the unchanged v4 rubric IDs and the [grading interpretation declared before exposure](../../outcomes-v5/grading-clarifications.md). Independent reviewers receive source and answers with treatment and resource counters withheld. Intrinsic answer wording can limit blinding. All failed or incomplete criteria remain visible.","",
          "## Matched observations","","Assisted minus ordinary; a negative input difference means fewer runtime-reported input tokens for that individual pair. This is descriptive, not a causal saving or billing reduction. Pair direction is normalized even where execution order is reversed.","",
          "| Task / repetition | Input difference | Output difference | Elapsed difference |",
          "| --- | ---: | ---: | ---: |"]
for p in pairs:
    if p["eligible"]:
        percent=f"{p['input_percent']:+.1f}%" if p["input_percent"] is not None else "undefined baseline"
        lines.append(f"| {p['task']} / {p['repetition']} | {p['input_delta']:+,} ({percent}) | {p['output_delta']:+,} | {p['elapsed_ms_delta']/1000:+.1f} s |")
    else:
        lines.append(f"| {p['task']} / {p['repetition']} | Ineligible | — | — |")
lines += ["", "## Measurement boundaries","",
          f"Known participant totals ({len(known)}/12 trials): **{totals['input_tokens']:,} input** and **{totals['output_tokens']:,} output** tokens. Cached input ({totals['cached_input_tokens']:,}) is included in input. Reported reasoning ({totals['reasoning_output_tokens']:,}) is included in this host's output. No byte-to-token estimator replaces runtime counters.","",
          "Task counters include all participant responses, direct instructions, retrieval, edits, tests, failures and retries. The separately timestamped [evaluator overhead snapshot](overhead.json) covers preparation, execution control, grading and reporting, including root context replays. It excludes responses after its explicit cutoff and final delivery; it is not normal aide runtime cost.","",
          "Aide operations: "+", ".join(f"`{n}` {tools[n]}" for n in ("code_search","code_references","code_symbols","code_outline","code_read_symbol"))+".","",
          "Handler elapsed is inside bridge elapsed, not CPU time or additional independent time. Accumulated parallel call durations are not wall time. Index setup and controller exports are separate. Returned UTF-8 bytes are a separate measurement boundary; truncation or ambiguous attribution leaves complete totals null with documented lower bounds.","",
          report["measurement_limitations"][0],"",
          "## Scope and limits","",
          "V5 supplies the complete prepared prompt directly at launch, including the RTK rule. Every trial has archived prompt bytes, hashes and prelaunch evidence. Raw host messages can be encrypted; controller archives establish intended delivery without claiming independently decrypted plaintext equality. The v4 invalid run remains unchanged and is not pooled with this rerun.","",
          "This tests a bundle of optional tools and guidance through an isolated stdio bridge in Codex. It does not measure native MCP presentation or live Claude Code/OpenCode sessions, even though both adapters are in the source fixture. Installed hooks are unchanged across conditions. Fresh actors do not establish cleared provider KV caches; cache and shared-machine effects remain uncontrolled.","",
          "Passing bounded checks is not general quality equivalence. Increased uptake, fewer calls or fewer returned bytes alone do not prove lower total model use. Individual paired measurements do not establish a broadly transferable savings percentage or a product change recommendation.","",
          "## Evidence","",
          "Controller ledger, exact launch messages, captures, answers, patches, source inventories, criterion reviews, trace audits and bridge receipts are retained alongside the report. Blinded input copies and their corpus hashes preserve what the quality reviewer saw. Production aide code and the frozen v4 corpus were not changed by these trials.","",
          "[Independent final audit](final-review.md) · [Browser checks](visual-check.json) · [Artifact hashes](evidence-manifest.json) · [Reviewed input hashes](blind-inputs-manifest.json)",""]
(BASE / "README.md").write_text("\n".join(lines))
print(json.dumps({"quality_passed":passed,"uptake":uptake,"eligible_pairs":len(eligible),"usage":totals}))
