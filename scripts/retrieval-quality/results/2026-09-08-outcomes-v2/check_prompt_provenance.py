from pathlib import Path
import json,sys,hashlib
repo=Path(__file__).resolve().parents[4];state=repo/'.aide/state/evaluation/2026-09-08-outcomes-v2'
logs=[Path('/home/johnm/.codex/sessions/2026/09/07/rollout-2026-09-07T09-56-35-01a07b15-668c-7453-b59c-9be3d32b2005.jsonl')]
for p in Path('/home/johnm/.codex/sessions/2026/09/09').glob('*.jsonl'):
 with p.open() as f:meta=json.loads(f.readline())['payload']
 if meta.get('agent_path')=='/root/pilot_v2_execution':logs.append(p)
review=[]
for p in logs:
 for line in p.open():
  r=json.loads(line);v=r.get('payload',{})
  if r.get('type')!='response_item' or v.get('type')!='function_call' or 'spawn_agent' not in v.get('name',''):continue
  a=json.loads(v['arguments']);name=a.get('task_name','');tid='t01' if name=='pilot_v2_t01' else name if p!=logs[0] and name.startswith('t') else None
  if not tid or not (state/tid/'participant-prompt.md').exists():continue
  expected=(state/tid/'participant-prompt.md').read_text().rstrip();actual=a.get('message','').rstrip()
  review.append({'trial':tid,'fork_turns':a.get('fork_turns'),'model_override':a.get('model'),'effort_override':a.get('reasoning_effort'),'exact_prompt_match_ignoring_trailing_whitespace':None if actual.startswith('gAAAA') else actual==expected, 'message_evidence':'protected runtime assignment; compare unavailable' if actual.startswith('gAAAA') else 'plaintext runtime assignment','expected_sha256_trimmed':hashlib.sha256(expected.encode()).hexdigest(),'source_log':str(p),'spawn_call_id':v.get('call_id')})
if len(sys.argv)>1:
 with Path(sys.argv[1]).open('x') as f:json.dump({'trials':review},f,indent=2);f.write('\n')
print({'reviewed':len(review),'mismatches':[r['trial'] for r in review if r['exact_prompt_match_ignoring_trailing_whitespace'] is False],'protected_messages':sum(r['exact_prompt_match_ignoring_trailing_whitespace'] is None for r in review),'not_fresh':[r['trial'] for r in review if r['fork_turns']!='none' or r['model_override'] or r['effort_override']]})
