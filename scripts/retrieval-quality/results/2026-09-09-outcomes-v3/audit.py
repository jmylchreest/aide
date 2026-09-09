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

def write_review(tid, categories, counts, notes, violations=None, aide_details=None, retries=0, pre_failures=0):
    import hashlib,re
    bs=bodies(tid)
    assert len(bs)==len(categories),(len(bs),len(categories))
    cap=json.loads((BASE/(tid+'-capture.json')).read_text())
    provenance=json.loads((BASE/(tid+'-provenance.json')).read_text())
    ledger=json.loads((BASE.parents[3]/'.aide/state/evaluation/2026-09-09-outcomes-v3/ledger.json').read_text())
    trial=next(t for t in ledger['trials'] if t['id']==tid)
    package=BASE.parents[1]/'outcomes-v3'
    protocol=json.loads((package/'protocol.json').read_text())
    values={'ROOT':trial['root'],'BRIDGE':ledger['retrieval_bridge']['path'],'BINARY':ledger['retrieval_bridge']['binary'],'EVIDENCE':trial['bridge_evidence_dir']}
    expected='\n\n'.join((protocol['shared_instructions'],protocol['treatments'][trial['treatment']],(package/trial['task']/'prompt.md').read_text()))
    expected=re.sub(r'\{\{([A-Z_]+)\}\}',lambda m:values[m[1]],expected).rstrip()+'\n'
    prompt=Path(trial['prompt_path']).read_text()
    assert prompt==expected
    assert hashlib.sha256(Path(provenance['raw_log']).read_bytes()).hexdigest()==provenance['raw_log_sha256']
    items=[]; partial=False; total=0
    for b,cat in zip(bs,categories):
        body=b['output']; size=len(body.encode()); truncated='truncated' in body and ('tokens truncated' in body or 'Warning: truncated output' in body)
        row={k:b[k] for k in ['call_id','content_index','part_index']};row.update(category=cat,utf8_bytes=size,exit_code=b.get('exit_code'),truncated=truncated,unattributable=bool(b.get('unattributable')))
        if cat in ['source','listing','search','aide']:
            if b.get('unattributable'):partial=True
            else:total+=size
            if truncated:partial=True
        if cat=='wrapper' and truncated:partial=True
        if cat=='mixed_source':partial=True
        items.append(row)
    aide_details=aide_details or {}; operations=sum(aide_details.values())
    r={'trial':tid,'reviewer':'v3_trace_audit','scope_verified':not violations,'trace_reviewed':True,'protocol_violations':violations or [],'aide_operations':operations,'aide_operation_details':aide_details,'aide_uptake':bool(operations),**counts,'call_target_exceeded':counts['source_edit_test_calls']>24,'tool_failure_details':{'nonzero_shell_exits':sum(b.get('exit_code') not in [0,None] for b in bs),'expected_pre_fix_test_failures':pre_failures,'retry_after_tool_error_calls':retries},'retrieval_output_bytes':None if partial else total,'byte_evidence':{'method':'Decode captured input_text JSON and nested fulfilled/value/result wrappers. Count UTF-8 bytes of attributable exec_command.output only; bridge payload unwrapping is applied where present. Exclude bootstrap, discovery, edit/test output and protocol wrappers. Inputs are inspected as text, never executed.','captured_attributable_retrieval_bytes':total,'partial':partial,'items':items,'boundary':'Captured result bodies, not original source size, provider-delivered bytes or token savings. Any captured truncation leaves complete retrieval_output_bytes unknown. Bridge full-return receipts are a separate boundary summarized independently.'},'prepared_prompt_verified':True,'prepared_prompt_sha256':hashlib.sha256(prompt.encode()).hexdigest(),'source_integrity':provenance['integrity'],'aide_duration_ms':None,'hook_metrics':None,'notes':notes+['Scope verification describes observed calls and source provenance; it is not syscall-level proof. Mandatory RTK bootstrap is separately allowed. Controller-created .aide exclusion from immutable-file grading does not authorize participant access.','Prepared prompt matches the frozen protocol expansion. Actual spawn-message equality is not established by this review; see separate controller evidence/attestation. Raw log SHA256 matches capture provenance.']}
    with (BASE/(tid+'-trace-review.json')).open('x') as f:json.dump(r,f,indent=2);f.write('\n')
    return r

if __name__=='__main__':
    import sys
    tid=sys.argv[1]; bs=bodies(tid)
    if len(sys.argv)==2:
        for i,b in enumerate(bs): print(i,b['call_id'],b['content_index'],len(b['output'].encode()),b.get('exit_code'),bool(b.get('unattributable')),b['output'][:100].replace(chr(10),' / '))
    else:
        i=int(sys.argv[2]); s=bs[i]['output']; start=int(sys.argv[3]) if len(sys.argv)>3 else 0; end=int(sys.argv[4]) if len(sys.argv)>4 else len(s);print(s[start:end])
