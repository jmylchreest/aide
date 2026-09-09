"""Offline usage snapshot for explicitly selected evaluation actors; no billing estimates."""
from pathlib import Path
import json,sys,datetime,importlib.util
repo=Path(__file__).resolve().parents[4];base=repo/'scripts/retrieval-quality/results/2026-09-09-outcomes-v3'
spec=importlib.util.spec_from_file_location('collector',repo/'scripts/retrieval-quality/collect_codex.py');c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
start='2026-09-09T00:44:28.065Z';end=datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00','Z')
actors={'/root','/root/retrieval_guidance','/root/isolated_index_plan','/root/opencode_hint_delivery','/root/v3_execution','/root/v3_trace_audit','/root/v3_blind_grade','/root/v3_execution/blind_grade','/root/v3_final_audit'}
rows=[]
for day in ['07','08','09']:
 for path in Path('/home/johnm/.codex/sessions/2026/09/'+day).glob('*.jsonl'):
  with path.open() as f:
   try:meta=json.loads(f.readline())['payload']
   except:continue
  if meta.get('agent_path') not in actors and meta.get('id')!='01a07b15-668c-7453-b59c-9be3d32b2005':continue
  seen={};conflicts=[];cumulative=None;all_seen={};window=[]
  for line in path.open():
   row=c._strict_json(line);v=row.get('payload',{})
   if row.get('type')!='token_usage_record':continue
   rid=v.get('response_id');usage=c._counters(v.get('usage'));cum=c._counters(v.get('thread_token_usage'))
   if not rid or usage is None or cum is None or v.get('thread_id')!=meta.get('id'):conflicts.append('invalid_record');continue
   if rid in all_seen and all_seen[rid]!=usage:conflicts.append('conflicting_response')
   all_seen[rid]=usage;cumulative=cum
   if start<=row.get('timestamp','')<=end:seen[rid]=usage;window.append({'response_id':rid,'timestamp':row['timestamp'],'counters':usage})
  all_sum={k:sum(v[k] for v in all_seen.values()) for k in c.COUNTERS}
  if all_sum!=cumulative:conflicts.append('cumulative_mismatch')
  counts={k:sum(v[k] for v in seen.values()) for k in c.COUNTERS} if not conflicts else None
  rows.append({'agent_path':meta.get('agent_path') or '/root','actor_id':meta['id'],'raw_log':str(path),'window_response_count':len(seen),'runtime_usage':counts,'unknown_reasons':sorted(set(conflicts)),'responses':window})
result={'basis':'Runtime-reported evaluation overhead in an explicit response-timestamp window; not task participant usage, billing, elapsed active work or aide server compute. Snapshot excludes responses finishing after its end, including this reporting command and final delivery. Root session reused earlier context, so input includes replay of that context.','window_start':start,'window_end':end,'actors':rows,'known_actor_sum':{k:sum(r['runtime_usage'][k] for r in rows if r['runtime_usage'] is not None) for k in c.COUNTERS},'unknown_actors':sum(r['runtime_usage'] is None for r in rows)}
with Path(sys.argv[1]).open('x') as f:json.dump(result,f,indent=2);f.write('\n')
print({'actors':len(rows),'unknown':result['unknown_actors'],'sum':result['known_actor_sum']})
