"""Render a compact, standalone v6 report without inferring missing measurements."""
import html
import json
import math
from pathlib import Path
import sys

BASE = Path(__file__).resolve().parent
esc = html.escape
COUNTERS = ("input_tokens", "cached_input_tokens", "cache_write_input_tokens",
            "output_tokens", "reasoning_output_tokens", "total_tokens")


def numeric(value):
    return type(value) in (int, float) and math.isfinite(value)


def fmt(value, decimals=0, signed=False):
    if not numeric(value):
        return "Unknown"
    return format(value, ("+" if signed else "") + ",." + str(decimals) + "f")


def read(path):
    return json.loads(path.read_text())


def findings(report):
    trials = report["trials"]
    pairs = []
    eligible = {(p["task"], p["repetition"]): {p["left"], p["right"]} for p in report["comparisons"]}
    for repetition in (1, 2):
        rows = {t["treatment"]: t for t in trials if t["repetition"] == repetition}
        ordinary, assisted = rows["ordinary"], rows["assisted"]
        valid = eligible.get(("implement", repetition)) == {ordinary["id"], assisted["id"]}
        pair = {"task":"implement", "repetition":repetition, "ordinary":ordinary["id"],
                "assisted":assisted["id"], "eligible":valid, "input_delta":None,
                "input_percent":None, "cached_input_delta":None, "uncached_input_delta":None,
                "output_delta":None, "elapsed_ms_delta":None}
        if valid:
            left, right = ordinary["runtime_usage"], assisted["runtime_usage"]
            for name, field in (("input_delta", "input_tokens"), ("cached_input_delta", "cached_input_tokens"),
                                ("output_delta", "output_tokens")):
                if numeric(left.get(field)) and numeric(right.get(field)):
                    pair[name] = right[field] - left[field]
            if numeric(pair["input_delta"]) and left["input_tokens"] > 0:
                pair["input_percent"] = 100 * pair["input_delta"] / left["input_tokens"]
            if numeric(pair["input_delta"]) and numeric(pair["cached_input_delta"]):
                pair["uncached_input_delta"] = pair["input_delta"] - pair["cached_input_delta"]
            if numeric(ordinary.get("elapsed_ms")) and numeric(assisted.get("elapsed_ms")):
                pair["elapsed_ms_delta"] = assisted["elapsed_ms"] - ordinary["elapsed_ms"]
        pairs.append(pair)
    known = [t for t in trials if isinstance(t.get("runtime_usage"), dict)]
    sums, coverage = {}, {}
    for key in COUNTERS:
        values = [t["runtime_usage"].get(key) for t in known if numeric(t["runtime_usage"].get(key))]
        sums[key], coverage[key] = (sum(values) if values else None), len(values)
    assisted = [t for t in trials if t["treatment"] == "assisted"]
    lower = sum(p["eligible"] and numeric(p["input_delta"]) and p["input_delta"] < 0 for p in pairs)
    higher = sum(p["eligible"] and numeric(p["input_delta"]) and p["input_delta"] > 0 for p in pairs)
    equal = sum(p["eligible"] and p["input_delta"] == 0 for p in pairs)
    count = sum(p["eligible"] for p in pairs)
    passed = sum(t["quality_verified"] and (t.get("quality") or {}).get("passed") is True for t in trials)
    return {"headline":f"{passed}/{len(trials)} passed quality; {count}/2 eligible paired comparisons.",
            "qualification":"Four implementation trials under the current host, two repetitions per condition. Descriptive resource differences, not causal savings or billing estimates. Historical v5 results are not pooled.",
            "resource_summary":f"Assisted input was lower in {lower}/{count} eligible pairs, higher in {higher}/{count}, and equal in {equal}/{count}." if count else "No eligible paired input comparison is available yet.",
            "quality_passed":passed, "quality_graded":sum(t["quality_verified"] for t in trials),
            "assisted_uptake":sum(t.get("aide_uptake") is True for t in assisted),
            "assisted_uptake_known":sum(type(t.get("aide_uptake")) is bool for t in assisted),
            "assisted_trials":len(assisted), "eligible_pairs":count,
            "known_task_usage":sums, "counter_trial_coverage":coverage,
            "known_usage_trials":len(known), "paired_observations":pairs}


def render(base=BASE):
    base = Path(base).resolve()
    report = read(base / "report.json")
    trials = report["trials"]
    if len(trials) != 4 or any(t["task"] != "implement" for t in trials):
        raise ValueError("Expected the four implementation-only v6 trials")
    data = findings(report)
    (base / "findings.json").write_text(json.dumps(data, indent=2, allow_nan=False) + "\n")
    parts = ['''<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="data:,"><title>Aide implementation experiment · v6</title><style>
:root{font:16px/1.5 system-ui;color:#e7edf5;background:#10151d;color-scheme:dark}*{box-sizing:border-box}body{max-width:1120px;margin:auto;padding:32px 24px}h1{font-size:clamp(25px,4vw,34px);line-height:1.2;margin:0 0 10px}h2{font-size:22px;margin-top:32px}p{max-width:85ch}.muted{color:#b8c4d5}a{color:#a8cdff}.cards{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:24px 0}.card{border:1px solid #46536a;border-radius:12px;padding:16px}.number{font-size:27px;font-weight:650}.sub{font-size:13px;color:#b8c4d5}.legend{display:flex;flex-wrap:wrap;gap:20px;font-size:13px}.swatch{display:inline-block;width:12px;height:12px;margin-right:6px}.cached{background:#80a7f0}.other{background:#f4b566}.bar{height:18px;display:flex;background:#435064;border-radius:3px;overflow:hidden}.bar span{height:100%}.barlabel{display:block;font-size:12px;color:#b8c4d5;margin-top:4px}table{width:100%;border-collapse:collapse;font-size:14px}th,td{text-align:left;padding:11px 8px;border-bottom:1px solid #354052;vertical-align:top}th{color:#b8c4d5;font-weight:550}.chart{width:28%;min-width:145px}.scroll{overflow-x:auto}.pass{color:#a6e3bd}.fail{color:#ffafad}details{border-top:1px solid #46536a;padding:18px 0;margin-top:18px}summary{cursor:pointer;font-weight:650}summary:focus-visible,a:focus-visible{outline:3px solid #f4b566;outline-offset:4px}pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;background:#182130;padding:12px;border-radius:6px}footer{margin-top:24px;font-size:13px;color:#b8c4d5}@media(max-width:650px){body{padding:24px 12px}.cards{grid-template-columns:1fr}.card{padding:12px}.number{font-size:24px}th,td{padding:8px 5px}table{font-size:12px}}
</style></head><body><main><h1>Aide implementation experiment</h1><p class="muted">Pilot v6 · one implementation task · two conditions × two repetitions · Codex</p><div class="cards">''']
    pair_labels = []
    for pair in data["paired_observations"]:
        percent = pair["input_percent"]
        pair_labels.append((fmt(percent, 1, True) + "%") if numeric(percent) else "Unknown")
    cards = [(f'{data["quality_passed"]}/4', "Passed quality checks", f'{data["quality_graded"]}/4 independently graded'),
             (f'{data["assisted_uptake"]}/{data["assisted_trials"]}' if data["assisted_uptake_known"] else "Unknown", "Assisted trials choosing aide", f'{data["assisted_uptake_known"]}/{data["assisted_trials"]} uptake outcomes known'),
             (" / ".join(pair_labels), "Paired input change", f'Assisted minus ordinary · repeats 1 / 2 · {data["eligible_pairs"]}/2 eligible')]
    for number, label, sub in cards:
        parts.append(f'<section class="card" aria-label="{esc(label)}"><div class="number">{esc(number)}</div><div>{esc(label)}</div><div class="sub">{esc(sub)}</div></section>')
    parts.append('</div>')
    if data["assisted_uptake_known"] == data["assisted_trials"] and data["assisted_trials"] and not data["assisted_uptake"]:
        parts.append('<p><strong>Neither assisted trial used aide retrieval.</strong> These resource differences do not demonstrate tool offloading or token savings from aide calls.</p>')
    parts.append('<p>' + esc(data["resource_summary"]) + '</p><p class="muted">' + esc(data["qualification"]) + '</p>')
    parts.append('<h2>Input, output and time</h2><p>Runtime-reported input on one shared scale. Cached input is included in total input; the amber segment is input minus cached input. Shorter bars do not establish lower billing or better work. Output and elapsed time remain separate.</p><div class="legend"><span><i class="swatch cached" aria-hidden="true"></i>Cached input</span><span><i class="swatch other" aria-hidden="true"></i>Uncached input</span></div><div class="scroll" tabindex="0" role="region" aria-label="Trial resource table"><table><thead><tr><th scope="col">Trial</th><th scope="col">Quality</th><th scope="col">Input tokens</th><th scope="col">Input split</th><th scope="col">Output</th><th scope="col">Seconds</th></tr></thead><tbody>')
    maximum = max([t["runtime_usage"]["input_tokens"] for t in trials if isinstance(t.get("runtime_usage"), dict) and numeric(t["runtime_usage"].get("input_tokens"))] + [1])
    for trial in trials:
        usage = trial.get("runtime_usage") or {}
        total, cached = usage.get("input_tokens"), usage.get("cached_input_tokens")
        bar = "Unknown"
        if numeric(total) and numeric(cached) and 0 <= cached <= total:
            remainder = total - cached
            label = f'Cached {fmt(cached)}; uncached {fmt(remainder)}'
            bar = f'<div class="bar" role="img" aria-label="{label}" style="width:{100 * total / maximum:.3f}%"><span class="cached" style="width:{100 * cached / total if total else 0:.3f}%"></span><span class="other" style="width:{100 * remainder / total if total else 0:.3f}%"></span></div><span class="barlabel">{label}</span>'
        quality = "Pass" if trial["quality_verified"] and (trial.get("quality") or {}).get("passed") is True else "Fail" if trial["quality_verified"] else "Unknown"
        elapsed = trial.get("elapsed_ms")
        parts.append(f'<tr><th scope="row">{esc(trial["id"])} · {esc(trial["treatment"])}<br><span class="sub">Repeat {trial["repetition"]}</span></th><td class="{quality.lower()}">{quality}</td><td>{fmt(total)}</td><td class="chart">{bar}</td><td>{fmt(usage.get("output_tokens"))}</td><td>{fmt(elapsed / 1000 if numeric(elapsed) else None, 1)}</td></tr>')
    parts.append('</tbody></table></div><h2>Paired differences</h2><p class="muted">Assisted minus ordinary. Eligibility requires completed, quality-passing, protocol-valid trials with matching actual runtime dimensions. Unknown or ineligible results never become zero.</p><div class="scroll" tabindex="0" role="region" aria-label="Paired resource differences"><table><thead><tr><th>Repeat</th><th>Input</th><th>Cached input</th><th>Uncached input</th><th>Output</th><th>Seconds</th></tr></thead><tbody>')
    for pair in data["paired_observations"]:
        if not pair["eligible"]:
            parts.append(f'<tr><th scope="row">{pair["repetition"]}</th><td colspan="5">Ineligible — see quality and protocol evidence below.</td></tr>')
        else:
            cells = [fmt(pair[key], signed=True) for key in ("input_delta", "cached_input_delta", "uncached_input_delta", "output_delta")]
            elapsed = pair["elapsed_ms_delta"]
            cells.append(fmt(elapsed / 1000 if numeric(elapsed) else None, 1, True))
            parts.append(f'<tr><th scope="row">{pair["repetition"]}</th>' + ''.join('<td>' + value + '</td>' for value in cells) + '</tr>')
    parts.append('</tbody></table></div><details><summary>Retrieval, verification and evidence</summary><p>Response counts include the whole task. Source bytes are returned text, not token estimates. Duplicate or overlapping reads require inspection of current bodies, intervening edits and missing context; overlap alone does not establish avoidable calls or saved tokens.</p><div class="scroll"><table><thead><tr><th>Trial</th><th>Protocol</th><th>Responses</th><th>Aide calls</th><th>Source bytes</th></tr></thead><tbody>')
    mapping_path = base / "blinding.json"
    mapping = read(mapping_path).get("mapping", {}) if mapping_path.exists() else {}
    for trial in trials:
        protocol = "Valid" if trial.get("protocol_valid") else "Invalid" if trial.get("trace_reviewed") else "Unknown"
        parts.append(f'<tr><th scope="row">{esc(trial["id"])}</th><td>{protocol}</td><td>{fmt(trial.get("response_count"))}</td><td>{fmt(trial.get("aide_operations"))}</td><td>{fmt(trial.get("retrieval_output_bytes"))}</td></tr>')
    parts.append('</tbody></table></div>')
    for trial in trials:
        tid = trial["id"]
        links = [f'<a href="{esc(tid)}-trace-review.json">Calls and scope</a>', f'<a href="{esc(tid)}-answer.md">Participant answer</a>']
        if tid in mapping:
            links.append(f'<a href="quality-{esc(mapping[tid])}.json">Independent quality review</a>')
        parts.append(f'<details><summary>{esc(tid)} · evidence and limitations</summary><p>' + ' · '.join(links) + '</p><pre>' + esc(json.dumps({key:trial.get(key) for key in ("unknown_reasons", "protocol_violations", "functional_quality", "implementation_review", "response_evidence", "overlap_review", "trace_evidence")}, indent=2)) + '</pre></details>')
    parts.append('</details><details><summary>Aide work and setup</summary><p>Handler elapsed is inside bridge elapsed: do not add it twice. Bridge time includes startup, identity checks and transport. Index setup was performed for every trial before participant work. Zero is a measured value; absent evidence remains unknown. Integer handler milliseconds can be zero for sub-millisecond work.</p><div class="scroll"><table><thead><tr><th>Trial</th><th>Attempts</th><th>Handler ms</th><th>Bridge ms</th><th>Index ms</th><th>Receipts complete</th></tr></thead><tbody>')
    for trial in trials:
        metrics = trial.get("bridge_metrics") or {}
        elapsed = ((metrics.get("timings_ms") or {}).get("total") or {}).get("total")
        setup = metrics.get("setup") or {}
        complete = metrics.get("evidence_complete_for_host_attempts")
        cells = [esc(trial["id"]), fmt(metrics.get("attempts")), fmt(trial.get("aide_duration_ms"), 2), fmt(elapsed, 2), fmt(setup.get("index_elapsed_ms"), 2), "Yes" if complete is True else "No" if complete is False else "Unknown"]
        parts.append('<tr>' + ''.join('<td>' + value + '</td>' for value in cells) + '</tr>')
    parts.append('</tbody></table></div><p>Full per-tool calls, timings, receipts and setup remain in <a href="report.json">report.json</a>. No handler timing is claimed when no handler was called.</p></details><details><summary>Task totals and evaluation resources</summary><p>Known participant counters include all task responses, including instructions, discovery, retries and verification. Partial coverage is explicit. Cached input is a subset of input; reasoning is a subset of output. These categories must not be added again to totals.</p><ul>')
    for key in COUNTERS:
        parts.append(f'<li>{esc(key)}: <strong>{fmt(data["known_task_usage"][key])}</strong> · {data["counter_trial_coverage"][key]}/4 trials known</li>')
    parts.append('</ul><p>Preparation, orchestration, grading and reporting use additional model resources. These evaluation resources are separate from participant totals and normal aide operating costs.</p>')
    overhead_path = base / "overhead.json"
    if overhead_path.exists():
        overhead = read(overhead_path)
        usage = overhead.get("known_actor_sum") or {}
        parts.append(f'<p>Known evaluator input: <strong>{fmt(usage.get("input_tokens"))}</strong>, including {fmt(usage.get("cached_input_tokens"))} cached. Output: <strong>{fmt(usage.get("output_tokens"))}</strong>. Actors with unknown totals: {fmt(overhead.get("unknown_actors"))}.</p><p>Response window: {esc(str(overhead.get("window_start", "Unknown")))} to {esc(str(overhead.get("window_end", "Unknown")))}. Later work and final delivery are excluded.</p><p><a href="overhead.json">Full evaluator evidence and coverage</a></p>')
    else:
        parts.append('<p>Evaluation overhead snapshot unavailable; this is unknown, not zero.</p>')
    parts.append('</details><details><summary>Method and limitations</summary><p>Both conditions retain the same installed hooks and current ambient guidance. Only assisted participants may use the isolated retrieval bridge. This is a prospective ordinary-versus-assisted comparison under the current host; it cannot isolate the revised wording from historical v5 results. Provider cache is uncontrolled, and a fresh actor is not a cleared provider cache.</p><p>Quality covers the frozen checks and independent review, not all possible correctness properties. This Codex experiment establishes no performance claim for Claude Code or OpenCode. The chart uses runtime counters, not a byte estimator.</p><ul>')
    for limitation in report.get("measurement_limitations", []):
        parts.append('<li>' + esc(limitation) + '</li>')
    parts.append('</ul><p><a href="README.md">Interpretation</a> · <a href="report.json">Machine-readable report</a> · <a href="findings.json">Descriptive findings</a> · <a href="reviewed-ledger.json">Reviewed ledger</a> · <a href="../../outcomes-v6/README.md">Frozen protocol</a> · <a href="../../outcomes-v5/grading-clarifications.md">Frozen grading clarification</a></p></details></main><footer>Diagnostic experiment evidence. No product savings headline is inferred.</footer></body></html>')
    (base / "index.html").write_text(''.join(parts), encoding="utf-8")
    return data


if __name__ == "__main__":
    result = render(Path(sys.argv[1]) if len(sys.argv) > 1 else BASE)
    print(result["headline"])
