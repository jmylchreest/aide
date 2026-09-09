"""Trace audit helper: decode captured result bodies; never evaluate captured inputs."""
import json
from pathlib import Path
BASE=Path(__file__).resolve().parent

def unwrap(v):
    if isinstance(v,dict):
        if 'exit_code' in v and 'output' in v: return [v]
        if v.get('status')=='fulfilled': return unwrap(v['value'])
        if 'result' in v: return unwrap(v['result'])
    if isinstance(v,list):
        return [x for item in v for x in unwrap(item)]
    return []

def bodies(tid):
    found=[]
    for row in json.loads((BASE/(tid+'-tool-results.json')).read_text()):
        for i,item in enumerate(row['output']):
            s=item.get('text','')
            if s.startswith('Script completed') or s.startswith('Script running'):continue
            try: parts=unwrap(json.loads(s))
            except ValueError:
                parts=[]
                for line in s.splitlines():
                    try: parts+=unwrap(json.loads(line))
                    except ValueError:
                        if line.strip(): parts.append({'output':line,'unattributable':True})
            if not parts:parts=[{'output':s,'unattributable':True}]
            for j,v in enumerate(parts):
                found.append({'call_id':row['call_id'],'content_index':i,'part_index':j,**v})
    return found

