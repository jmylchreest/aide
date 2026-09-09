from pathlib import Path
import json,sys,importlib.util,hashlib,difflib
REPO=Path(__file__).resolve().parents[4]
BASE=REPO/'scripts/retrieval-quality/outcomes-v2'
RESULTS=REPO/'scripts/retrieval-quality/results/2026-09-08-outcomes-v2'
STATE=REPO/'.aide/state/evaluation/2026-09-08-outcomes-v2'
def module(name,path):
 spec=importlib.util.spec_from_file_location(name,path); m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
collector=module('collector',BASE.parent/'collect_codex.py'); grader=module('grader',BASE/'grade.py')
tid,agent=sys.argv[1:3]
trial=next(t for t in json.loads((STATE/'ledger.json').read_text())['trials'] if t['id']==tid)
logs=[]
for day in ['08','09']:
 for p in Path('/home/johnm/.codex/sessions/2026/09/'+day).glob('*.jsonl'):
  with p.open() as stream:
   try: meta=json.loads(stream.readline())
   except ValueError:continue
  if meta.get('type')=='session_meta' and meta['payload'].get('agent_path')==agent:logs.append(p)
if len(logs)!=1:raise SystemExit(f'Expected one exact actor log; found {len(logs)}')
p=logs[0]; capture=collector.collect_log(p)
if not capture['completion']['complete']:raise SystemExit('Not completed yet')
def write(name,value):
 with (RESULTS/name).open('x') as f:json.dump(value,f,indent=2,ensure_ascii=False,allow_nan=False);f.write('\n')
write(tid+'-capture.json',capture)
with (RESULTS/(tid+'-answer.md')).open('x') as f:f.write(capture['answer']['raw_text']+'\n')
outputs=[]
for line in p.read_text().splitlines():
 row=json.loads(line);v=row.get('payload',{})
 if row.get('type')=='response_item' and v.get('type') in ['custom_tool_call_output','function_call_output']:
  outputs.append({k:v.get(k) for k in ['type','call_id','output']})
write(tid+'-tool-results.json',outputs)
root=Path(trial['root']);template=BASE/trial['task']/'template'
if trial['task']!='navigation': grade=grader.grade(trial['task'],root);write(tid+'-grade.json',grade)
else:grade={'integrity':grader.integrity(template,root,set())}
patch=''
for source in sorted(template.rglob('*')):
 if not source.is_file():continue
 rel=source.relative_to(template);dest=root/rel
 if dest.is_file() and dest.read_bytes()!=source.read_bytes():patch+=''.join(difflib.unified_diff(source.read_text().splitlines(True),dest.read_text().splitlines(True),fromfile='a/'+str(rel),tofile='b/'+str(rel)))
with (RESULTS/(tid+'.patch')).open('x') as f:f.write(patch)
write(tid+'-provenance.json',{'raw_log':str(p),'raw_log_sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'agent_path':agent,'runtime_actor_id':capture['runtime']['actor_id'],'integrity':grade['integrity']})
print(json.dumps({'id':tid,'runtime':capture['runtime'],'usage':capture['runtime_usage']['counters'],'unknown':capture['runtime_usage']['unknown_reasons'],'quality':grade.get('quality'),'integrity':grade['integrity'],'calls':len(capture['tool_calls'])}))
