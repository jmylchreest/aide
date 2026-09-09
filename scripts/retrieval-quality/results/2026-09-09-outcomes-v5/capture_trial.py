from pathlib import Path
import json,sys,importlib.util,hashlib,difflib,shutil,subprocess,time
REPO=Path(__file__).resolve().parents[4]
PACKAGE=REPO/'scripts/retrieval-quality/outcomes-v5'
CORPUS=REPO/'scripts/retrieval-quality/outcomes-v4'
RESULTS=REPO/'scripts/retrieval-quality/results/2026-09-09-outcomes-v5'
STATE=RESULTS
def module(name,path):
 spec=importlib.util.spec_from_file_location(name,path); m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
collector=module('collector',CORPUS.parent/'collect_codex.py'); grader=module('grader',CORPUS/'grade.py')
tid,agent=sys.argv[1:3]
trial=next(t for t in json.loads((STATE/'ledger.json').read_text())['trials'] if t['id']==tid)
logs=[]
for month in [Path('/home/johnm/.codex/sessions/2026/09')]:
 for p in month.glob('*/*.jsonl'):
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
root=Path(trial['root']);template=CORPUS/'common/template'
grade=grader.grade(trial['task'],root,CORPUS);write(tid+'-grade.json',grade)
patch=''
for source in sorted(template.rglob('*')):
 if not source.is_file():continue
 rel=source.relative_to(template);dest=root/rel
 if dest.is_file() and dest.read_bytes()!=source.read_bytes():patch+=''.join(difflib.unified_diff(source.read_text().splitlines(True),dest.read_text().splitlines(True),fromfile='a/'+str(rel),tofile='b/'+str(rel)))
with (RESULTS/(tid+'.patch')).open('x') as f:f.write(patch)
write(tid+'-provenance.json',{'raw_log':str(p),'raw_log_sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'agent_path':agent,'runtime_actor_id':capture['runtime']['actor_id'],'integrity':grade['integrity']})
snapshotter=module('snapshotter',RESULTS/'snapshot_candidate.py')
snapshotter.snapshot(RESULTS,CORPUS,trial)
bridge=module('bridge',CORPUS.parent/'outcomes-v3/bridge.py')
evidence=Path(trial['bridge_evidence_dir'])
binding=json.loads((evidence/'setup.json').read_text())['binding']
started=time.monotonic()
export_record={'boundary':'Controller export after participant completion; outside participant elapsed time.',
 'returncode':None,'stdout':None,'stderr':None,'error':None}
events=None
try:
 export=subprocess.run([binding['binary'],'observe','list','--json','--limit=0'],cwd=root,
  env=bridge.child_environment(root),capture_output=True,text=True,timeout=30)
 export_record.update(returncode=export.returncode,stderr=export.stderr,stdout=export.stdout)
 if export.returncode==0:events=json.loads(export.stdout)
except (OSError,subprocess.TimeoutExpired,ValueError) as error:
 export_record['error']=str(error)
export_record['elapsed_ms']=(time.monotonic()-started)*1000
write(tid+'-observe-export.json',export_record)
if isinstance(events,list):write(tid+'-server-work.json',events)
target=RESULTS/'bridge'/tid
target.parent.mkdir(exist_ok=True)
shutil.copytree(evidence,target)
print(json.dumps({'id':tid,'runtime':capture['runtime'],'usage':capture['runtime_usage']['counters'],'unknown':capture['runtime_usage']['unknown_reasons'],'quality':grade.get('quality'),'integrity':grade['integrity'],'calls':len(capture['tool_calls'])}))

ledger=json.loads((RESULTS/'ledger.json').read_text())
row=next(row for row in ledger['trials'] if row['id']==tid)
row.update(status='completed',agent_id=capture['runtime']['actor_id'],runtime_session_id=capture['runtime']['actor_id'],log_path=str(p),agent_path=agent)
for key in ('quality_verified','quality','functional_quality_verified','functional_quality','integrity'):
 if key in grade:row[key]=grade[key]
(RESULTS/'ledger.json').write_text(json.dumps(ledger,indent=2)+'\n')
