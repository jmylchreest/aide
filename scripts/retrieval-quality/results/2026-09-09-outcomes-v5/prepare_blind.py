"""Prepare treatment-blinded review copies from captured completed trials."""
from pathlib import Path
import json,shutil,re
BASE=Path(__file__).resolve().parent
CORPUS=BASE.parents[1]/'outcomes-v4'
DEST=Path('/tmp/aide-v5-blind-review')
MAPPING=dict(zip([f't{i:02}' for i in range(1,13)], ['Q7','M2','V9','C4','T6','H3','N8','A5','R1','K9','W4','D8']))
DEST.mkdir(exist_ok=True)
clarification=BASE.parents[1]/'outcomes-v5/grading-clarifications.md'
if not (DEST/'grading-clarifications.md').exists():shutil.copy2(clarification,DEST/'grading-clarifications.md')
if not (DEST/'source').exists():shutil.copytree(CORPUS/'common/template',DEST/'source')
for task in ('trace','impact','implement'):
 d=DEST/'tasks'/task;d.mkdir(parents=True,exist_ok=True)
 for filename in ('prompt.md','rubric.json','reference.md','review-rubric.json'):
  src=CORPUS/task/filename
  if src.exists() and not (d/filename).exists():shutil.copy2(src,d/filename)
ledger=json.loads((BASE/'ledger.json').read_text())
for row in ledger['trials']:
 tid=row['id'];label=MAPPING[tid]
 capture=BASE/(tid+'-capture.json')
 if not capture.exists():continue
 target=DEST/'candidates'/label
 if target.exists():continue
 target.mkdir(parents=True)
 cap=json.loads(capture.read_text());answer=cap['answer']['raw_text']
 answer=answer.replace(row['root']+'/', 'source/')
 # Record redaction boundaries separately; never silently repair answer content.
 (target/'answer.md').write_text(answer+'\n')
 (target/'task.txt').write_text(row['task']+'\n')
 if row['task']=='implement':
  shutil.copytree(row['root'],target/'source',ignore=lambda d,n:['.aide'] if Path(d)==Path(row['root']) and '.aide' in n else [])
  shutil.copy2(BASE/(tid+'.patch'),target/'change.patch')
  g=json.loads((BASE/(tid+'-grade.json')).read_text())
  (target/'runtime-checks.json').write_text(json.dumps({k:g.get(k) for k in ('integrity','functional_quality_verified','functional_quality','checks','candidate_unchanged')},indent=2).replace(row['root'],'<workspace>')+'\n')
 print(label,row['task'])
(BASE/'blinding.json').write_text(json.dumps({'mapping':MAPPING,'review_root':str(DEST),'method':'Permuted opaque labels; no condition, usage, bridge or raw retrieval trace supplied. Absolute candidate source prefixes normalized. Answer prose otherwise unchanged; intrinsic mentions may limit blinding.'},indent=2)+'\n')

for row in ledger['trials']:
 if row['task']!='implement':continue
 target=DEST/'candidates'/MAPPING[row['id']]
 audit=BASE/(row['id']+'-trace-review.json')
 if target.exists() and audit.exists():
  value=json.loads(audit.read_text()).get('execution_verification')
  if value is not None:
   (target/'participant-verification.json').write_text(json.dumps(value,indent=2).replace(row['root'],'<workspace>')+'\n')
