from pathlib import Path
import hashlib,json,os,shutil,difflib
from datetime import datetime,timezone

def inventory(root):
 out={}
 for directory,dirs,names in os.walk(root,followlinks=False):
  if Path(directory)==root: dirs[:]=[n for n in dirs if n!='.aide']
  for name in names:
   p=Path(directory)/name
   if p.is_symlink(): raise ValueError(f'Unexpected symlink: {p}')
   out[p.relative_to(root).as_posix()]=hashlib.sha256(p.read_bytes()).hexdigest()
 return out

def snapshot(results,base,trial,additive=False):
 tid=trial['id'];root=Path(trial['root']);target=results/'changes'/tid
 if target.exists(): raise ValueError('Snapshot destination exists')
 before=inventory(root)
 baseline=inventory(base/'common/template')
 extra=base/trial['task']/'participant'
 if extra.exists():baseline.update(inventory(extra))
 modified=sorted(p for p,h in before.items() if baseline.get(p)!=h)
 deleted=sorted(set(baseline)-set(before))
 if additive:
  patch=''
  common=base/'common/template'
  for source in sorted(common.rglob('*')):
   if not source.is_file():continue
   rel=source.relative_to(common);dest=root/rel
   if dest.is_file() and dest.read_bytes()!=source.read_bytes():patch+=''.join(difflib.unified_diff(source.read_text().splitlines(True),dest.read_text().splitlines(True),fromfile='a/'+str(rel),tofile='b/'+str(rel)))
  assert patch==(results/(tid+'.patch')).read_text(),'Source differs from captured patch'
  capture_mtime=(results/(tid+'-provenance.json')).stat().st_mtime_ns
  assert all((root/p).stat().st_mtime_ns<=capture_mtime for p in before),'Source modified after capture timestamp'
 for rel in modified:
  dest=target/rel;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(root/rel,dest)
 assert before==inventory(root),'Source changed while preserving snapshot'
 for rel in modified:assert hashlib.sha256((target/rel).read_bytes()).hexdigest()==before[rel]
 provenance_path=results/(tid+'-provenance.json')
 provenance=json.loads(provenance_path.read_text())
 provenance['candidate_snapshot']={'captured_at':datetime.now(timezone.utc).isoformat(),'scope':'All final regular files excluding controller .aide state','inventory_sha256':before,'preserved_changes_directory':str(target),'preserved_files':modified,'deleted_files':deleted,'copy_verified':True,'additive_after_capture':additive,'prior_patch_and_mtime_verified':True if additive else None}
 provenance_path.write_text(json.dumps(provenance,indent=2)+'\n')
 return {'id':tid,'preserved_files':modified,'final_inventory_count':len(before),'additive':additive}

if __name__=='__main__':
 import sys
 results=Path(__file__).resolve().parent;base=results.parents[1]/'outcomes-v4'
 trial=next(t for t in json.loads((results/'ledger.json').read_text())['trials'] if t['id']==sys.argv[1])
 print(json.dumps(snapshot(results,base,trial,additive=True)))
