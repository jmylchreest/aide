from pathlib import Path
import json,html,sys
b=Path(sys.argv[1]);r=json.loads((b/'report.json').read_text());trials=r['trials']
esc=html.escape
known=[t for t in trials if t['runtime_usage'] is not None]
passed=sum(t['quality_verified'] and t['quality'].get('passed') is True for t in trials)
evaluated=sum(t['quality_verified'] for t in trials)
uptake=sum(t['aide_uptake'] is True for t in trials if t['treatment']!='ordinary')
parts=['''<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Aide retrieval outcomes · pilot v3</title><style>
:root{color-scheme:light dark;font:16px/1.5 system-ui;background:#10151d;color:#e7edf5}body{max-width:1120px;margin:auto;padding:32px 24px}h1{font-size:32px;margin-bottom:8px}h2{font-size:22px;margin-top:36px}.muted{color:#b6c0cf}a{color:#a8cdff}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(185px,1fr));gap:12px;margin:24px 0}.card{border:1px solid #374252;border-radius:12px;padding:16px}.number{font-size:28px;font-weight:650}.bar{height:16px;display:flex;background:#253144;border-radius:3px;overflow:hidden;min-width:1px}.cached{background:#5885d9}.other{background:#e5a458}.legend{display:flex;gap:20px;font-size:13px}.swatch{display:inline-block;width:12px;height:12px;margin-right:6px}table{width:100%;border-collapse:collapse;font-size:14px}th,td{text-align:left;padding:10px 8px;border-bottom:1px solid #2b3544}td.chart{width:34%}th{color:#b6c0cf;font-weight:500}.scroll{overflow:auto}.pass{color:#a6e3bd}.fail{color:#ffafad}details{border-top:1px solid #374252;padding:18px 0;margin-top:24px}summary{cursor:pointer;font-weight:600}code{font-size:13px}footer{margin-top:28px;font-size:13px;color:#b6c0cf}@media(max-width:650px){body{padding:20px 12px}.chart{min-width:140px}table{font-size:12px}th,td{padding:8px 5px}}
</style><h1>Aide retrieval outcomes</h1><p class="muted">Broader retrieval pilot · 3 tasks × 3 retrieval conditions × 2 repetitions · Codex</p><div class="cards">''']
input_total=sum(t['runtime_usage']['input_tokens'] for t in known)
output_total=sum(t['runtime_usage']['output_tokens'] for t in known)
coverage='' if len(known)==len(trials) else f' (known {len(known)}/{len(trials)})'
for number,label in [(f'{passed}/{evaluated}','Passed frozen quality checks'),(f'{uptake}/12','Optional/guided retrieval uptake'),(f'{input_total:,}','Task input tokens'+coverage),(f'{output_total:,}','Task output tokens'+coverage)]:
 parts.append(f'<div class="card"><div class="number">{number}</div>{label}</div>')
parts.append(f'</div><p class="muted">{sum(t["status"]=="completed" for t in trials)}/{len(trials)} completed · {len(known)}/{len(trials)} with known usage. Task totals exclude <a href="overhead.json">preparation and review usage</a>.</p>')

tool_mix={name:0 for name in ['code_search','code_references','code_symbols','code_outline','code_read_symbol']}
for trial in trials:
 for call in (trial.get('bridge_metrics') or {}).get('calls',[]):
  if call.get('tool') in tool_mix and call.get('handler_requested'):tool_mix[call['tool']]+=1
parts.append('<p>Only the two guided navigation runs selected aide retrieval. Their input differences change direction across repetitions, and both took longer than the corresponding ordinary runs. This pilot shows no consistent efficiency gain.</p>')
parts.append('<p class="muted">Recorded retrieval calls: '+ ' · '.join(f'{esc(name.removeprefix("code_"))} {count}' for name,count in tool_mix.items()) + '</p><p>Each bar is one trial’s runtime-reported input, scaled within its task. Cached input is a subset. Output, quality and time stay separate. Shorter bars do not establish lower billing or better results.</p><div class="legend"><span><i class="swatch cached"></i>Cached input</span><span><i class="swatch other"></i>Remaining input</span></div>')
for task in ['navigation','debug','edit']:
 rows=sorted([t for t in trials if t['task']==task],key=lambda t:(t['repetition'],['ordinary','available','guided'].index(t['treatment'])))
 maxv=max([t['runtime_usage']['input_tokens'] for t in rows if t['runtime_usage']] or [1])
 parts.append(f'<h2>{esc(task.capitalize())}</h2><div class="scroll"><table><thead><tr><th>Condition / repeat</th><th>Quality</th><th>Input tokens</th><th>Input split</th><th>Output tokens</th><th>Seconds</th><th>Aide calls</th></tr></thead><tbody>')
 for t in rows:
  usage=t['runtime_usage'];q=t['quality'];grade='Pass' if t['quality_verified'] and q.get('passed') else 'Fail' if t['quality_verified'] else 'Unknown';bar='Unknown';inp=out='—'
  if usage:
   inp=f'{usage["input_tokens"]:,}';out=f'{usage["output_tokens"]:,}';cached=usage['cached_input_tokens'];remaining=usage['input_tokens']-cached
   bar=f'<div class="bar" title="Cached {cached:,}; remaining {remaining:,}" style="width:{100*usage["input_tokens"]/maxv:.2f}%"><span class="cached" style="flex:{cached}"></span><span class="other" style="flex:{remaining}"></span></div>'
  seconds=f'{t["elapsed_ms"]/1000:.1f}' if t['elapsed_ms'] is not None else '—';aide=str(t['aide_operations']) if t['aide_operations'] is not None else '—'
  parts.append(f'<tr><td><a href="{t["id"]}-answer.md">{esc(t["treatment"])} / {t["repetition"]}</a></td><td class="{grade.lower()}">{grade}</td><td>{inp}</td><td class="chart">{bar}</td><td>{out}</td><td>{seconds}</td><td>{aide}</td></tr>')
 parts.append('</tbody></table></div>')
parts.append('<details><summary>Aide work and setup</summary><p>Handler elapsed time comes from verified server receipts. Bridge time also includes process startup, validation and transport. Index setup happened before each participant, including ordinary conditions. These are separate boundaries; do not add handler time again to bridge time. Integer handler milliseconds can be zero for sub-millisecond work.</p><div class="scroll"><table><thead><tr><th>Trial</th><th>Recorded bridge calls</th><th>Handler ms</th><th>Bridge ms</th><th>Index setup ms</th></tr></thead><tbody>')
for t in trials:
 bm=t.get('bridge_metrics') or {}; setup=bm.get('setup') or {}; timing=bm.get('timings_ms') or {}
 def measured(value):
  return f'{value:,.2f}' if isinstance(value,(int,float)) else 'Unknown'
 parts.append(f'<tr><td>{esc(t["id"])} · {esc(t["treatment"])}</td><td>{bm.get("attempts", "Unknown")}</td><td>{measured(t.get("aide_duration_ms"))}</td><td>{measured(timing.get("total",{}).get("total"))}</td><td>{measured(setup.get("index_elapsed_ms"))}</td></tr>')
parts.append('</tbody></table></div><p>No handler measurement is reported when no retrieval handler was called. Full operation, error and receipt records remain in the machine-readable report.</p></details>')
parts.append('''<details><summary>How to interpret this pilot</summary><p>Small fixed tasks, two repetitions per condition, uncontrolled provider cache and a shared machine. All conditions retain aide hooks. Quality is limited to the frozen checks and reviewed evidence. A failed or invalid trial remains visible; report deltas require passing quality and comparable runtime metadata.</p><p>Task counters exclude experiment preparation, orchestration, grading and report generation. Those additional model resources appear separately in <a href="overhead.json">the overhead snapshot</a>. Isolated indexing, per-call process startup, identity checks and handler work appear separately in the bridge evidence; unknown fields remain null. This broader-capability experiment changes the access method and is not a clean before/after comparison with v2.</p><p>These Codex observations establish no performance claim for Claude Code or OpenCode and no general token-savings percentage. The chart uses runtime counters, not the byte estimator.</p></details><details><summary>Evidence and method</summary><p><a href="README.md">Findings and limitations</a> · <a href="report.json">Machine-readable report</a> · <a href="reviewed-ledger.json">Reviewed ledger</a> · <a href="environment.json">Environment</a> · <a href="preflight-review.md">Preflight review</a></p><p>Exact prompts, source snapshots, grading and collector identity were frozen before trial one in the package manifest linked from the environment record. Individual captures, traces, source patches and grades remain alongside this report.</p></details><footer>Diagnostic experiment evidence. No savings headline is added to the product Overview.</footer></html>''')
with (b/'index.html').open('x') as f:f.write(''.join(parts))
