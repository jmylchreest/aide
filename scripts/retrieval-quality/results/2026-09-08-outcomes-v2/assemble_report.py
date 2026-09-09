from pathlib import Path
import json,sys,importlib.util
repo=Path(__file__).resolve().parents[4];b=repo/'scripts/retrieval-quality/results/2026-09-08-outcomes-v2';pkg=repo/'scripts/retrieval-quality/outcomes-v2';state=repo/'.aide/state/evaluation/2026-09-08-outcomes-v2'
spec=importlib.util.spec_from_file_location('run',pkg/'run.py');runner=importlib.util.module_from_spec(spec);spec.loader.exec_module(runner)
ledger=json.loads((state/'ledger.json').read_text())
blind=json.loads((b/'blind-grading.json').read_text()) if (b/'blind-grading.json').exists() else {}
# Root supplies grades mapped from independently blinded candidate labels.
nav=blind.get('trials',{})
for row in ledger['trials']:
 tid=row['id'];cp=b/(tid+'-capture.json');tp=b/(tid+'-trace-review.json');pp=b/(tid+'-provenance.json')
 if not cp.exists():row['status']='missing';continue
 capture=json.loads(cp.read_text());provenance=json.loads(pp.read_text());trace=json.loads(tp.read_text()) if tp.exists() else {}
 quality=nav.get(tid) if row['task']=='navigation' else json.loads((b/(tid+'-grade.json')).read_text())['quality']
 row.update(agent_id=capture['runtime']['agent_path'],runtime_session_id=capture['runtime']['actor_id'],log_path=provenance['raw_log'],status='completed' if capture['completion']['complete'] else 'failed',quality_verified=quality is not None,quality=quality)
 for key in ['scope_verified','trace_reviewed','protocol_violations','aide_operations','aide_uptake','retrieval_output_bytes']:row[key]=trace.get(key)
 hp=b/(tid+'-host-events.json')
 if hp.exists():
  events=json.loads(hp.read_text());payloads=[int(e['attrs']['payload_bytes']) for e in events if e.get('attrs',{}).get('payload_bytes','').isdigit()]
  row['hook_metrics']={'actor_attributed_host_result_events':len(events),'known_payload_events':len(payloads),'host_result_payload_bytes':sum(payloads),'injected_context_bytes':None,'hook_duration_ms':None,'boundary':'Captured host-result events; not complete injected-context or hook-compute accounting.'}
 row['aide_duration_ms']=None
 rp=b/(tid+'-receipt-review.json')
 if rp.exists():
  receipt=json.loads(rp.read_text());row['receipt_review']=receipt
  if receipt.get('receipt_reviewed') is True and receipt.get('all_receipts_valid',receipt.get('all_nine_receipts_valid')) is True:
   row['aide_duration_ms']=receipt.get('summed_measured_work_elapsed_ms')
ledger['notes']=['Runtime_session_id denotes actor ID, even when nested actors share the parent host session ID.','Quality and trace evidence are stored in per-trial artifacts.','Expected pre-fix test failures are distinct from transport/retrieval failures.','Tool invocation counts and bytes do not establish avoided operations or provider savings.']
with (b/'reviewed-ledger.json').open('x') as f:json.dump(ledger,f,indent=2);f.write('\n')
report=runner.verified_report(pkg/'protocol.json',ledger)
with (b/'report.json').open('x') as f:json.dump(report,f,indent=2);f.write('\n')
print({'rows':len(report['trials']),'comparisons':len(report['comparisons']),'quality_pass':sum(t['quality_verified'] and t['quality']['passed'] for t in report['trials'])})
