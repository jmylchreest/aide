from pathlib import Path
import sys, json, hashlib, importlib.util, subprocess
from datetime import datetime, timezone
REPO=Path(__file__).resolve().parents[4]
RESULTS=Path(__file__).resolve().parent
BASE=REPO/'scripts/retrieval-quality/outcomes-v4'
mode,tid=sys.argv[1:3]
now=lambda:datetime.now(timezone.utc).isoformat()
def write(name,data):
 with (RESULTS/name).open('x') as f: json.dump(data,f,indent=2);f.write('\n')
if mode=='preflight':
 result=subprocess.run(['rtk','proxy','python3',str(BASE/'run.py'),'verify'],cwd=REPO,capture_output=True,text=True,check=True)
 ledger=json.loads((RESULTS/'ledger.json').read_text())
 row=next(r for r in ledger['trials'] if r['id']==tid)
 assert row['status']=='planned',row['status']
 digest=hashlib.sha256(Path(row['prompt_path']).read_bytes()).hexdigest()
 assert digest==row['prompt_sha256']
 spec=importlib.util.spec_from_file_location('bridge',BASE.parent/'outcomes-v3/bridge.py')
 bridge=importlib.util.module_from_spec(spec);spec.loader.exec_module(bridge)
 root,binary,evidence,binding=bridge.bound_configuration(Path(row['root']),Path(ledger['retrieval_bridge']['binary']),Path(row['bridge_evidence_dir']))
 assert binding['binary_sha256']==ledger['retrieval_bridge']['binary_sha256']
 message=f"Read the complete frozen task instructions at /tmp/aide-outcomes-v4-trials/{tid}/participant-prompt.md and carry them out. This instruction-file read is explicitly authorized. Your task workspace is /tmp/aide-outcomes-v4-trials/{tid}/root. Follow the prepared prompt exactly; do not inspect other experiment/controller files. Finish with your requested answer."
 record={'id':tid,'verified_at':now(),'package_verification':result.stdout.strip(),'prompt_sha256':digest,'binding':binding,'launcher_message':message}
 write(tid+'-prelaunch.json',record)
 print(json.dumps({'id':tid,'package_verified':True,'prompt_verified':True,'binding_verified':True}))
elif mode=='launched':
 pre=json.loads((RESULTS/(tid+'-prelaunch.json')).read_text())
 write(tid+'-launch.json',{'id':tid,'launch_recorded_at':now(),'agent_path':sys.argv[3],'message':pre['launcher_message'],'fork_turns':'none','model_override':None,'effort_override':None})
 print(tid+' launch recorded')
else: raise SystemExit('unknown mode')
