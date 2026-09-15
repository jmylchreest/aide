/** EXPERIMENT HIDDEN CHECKS. Copy to workspace/.grader/hidden.test.ts.
 * Harness mocks I/O only; actual production writer, recorder and adapters run.
 */
import { beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { harness, resetHarness } from "../harness/preload.ts";
import { recordModelUsage, createOpenCodeUsageRecorder, openCodeUsageEvent,
  claudeUsageEvent, collectTranscriptUsage } from "../src/core/model-usage.ts";

beforeEach(resetHarness);
const event = {kind:"session",name:"model_usage",session:"s",ts:"2024-02-29T12:00:00.123456Z"};
const request = (events: any[] = [event]) => ({binary:"/fixture/aide",cwd:"/fixture/project",events});
const part = (id = "p", input = 11) => ({type:"step-finish",id,sessionID:"s",messageID:"m",
  tokens:{input,output:13,reasoning:5,cache:{read:17,write:3}}});
const invoke = (writer: any, req: any) => writer(req);

describe("writer result contract", () => {
  test("empty-batch", () => {
    expect(invoke(recordModelUsage,request([]))).toEqual({status:"acknowledged",recorded:0});
    expect(harness.calls).toHaveLength(0);
  });
  test("full-ack-and-transport", () => {
    harness.response="Recorded 2 event(s)\n";
    const req=request([event,{...event,session:"t"}]);
    expect(invoke(recordModelUsage,req)).toEqual({status:"acknowledged",recorded:2});
    expect(harness.calls).toEqual([{binary:req.binary,args:["observe","record","--stdin"],
      options:{cwd:req.cwd,input:req.events.map(e=>JSON.stringify(e)).join("\n")+"\n",
        timeout:10000,stdio:["pipe","pipe","pipe"]}}]);
  });
  test("compatible-ack-spelling", () => {
    for(const response of ["  recorded 1 EVENT(S), skipped 0 \n","Recorded 1 event(s)","\nRecorded 1 event(s)\n"]){
      harness.response=response;
      expect(invoke(recordModelUsage,request())).toEqual({status:"acknowledged",recorded:1});
    }
  });
  test("invalid-ack-classification", () => {
    for(const response of ["","OK","Recorded 0 event(s)","Recorded 2 event(s)",
      "Recorded 1 event(s), skipped 1","Recorded 1 event(s)\nwarning","Recorded -1 event(s)"]){
      harness.response=response;
      expect(invoke(recordModelUsage,request())).toEqual({status:"unacknowledged",reason:"invalid-ack"});
    }
  });
  test("write-error-classification", () => {
    harness.fail=true;
    expect(invoke(recordModelUsage,request())).toEqual({status:"unacknowledged",reason:"write-error"});
    expect(harness.calls).toHaveLength(1);
  });
  test("serialization-error-classification", () => {
    const circular:any={...event}; circular.attrs=circular;
    expect(invoke(recordModelUsage,request([circular]))).toEqual({status:"unacknowledged",reason:"write-error"});
    expect(harness.calls).toHaveLength(0);
  });
});

describe("recorder contract and existing cache", () => {
  test("object-forwarding-retry-and-dedup", () => {
    const calls:any[]=[];
    const record=createOpenCodeUsageRecorder(((req:any)=>{calls.push(req);return calls.length===1
      ? {status:"unacknowledged",reason:"invalid-ack"} : {status:"acknowledged",recorded:1};}) as any) as any;
    const req={binary:"bin",cwd:"/a",part:part()};
    record(req);record(req);record(req);
    expect(calls).toEqual([0,1].map(()=>({binary:"bin",cwd:"/a",events:[openCodeUsageEvent(part())]})));
  });
  test("write-error-results-remain-retryable", () => {
    let calls=0;
    const record=createOpenCodeUsageRecorder((()=>{calls++;return {status:"unacknowledged",reason:"write-error"};}) as any) as any;
    const req={binary:"bin",cwd:"/a",part:part()};record(req);record(req);
    expect(calls).toBe(2);
  });
  test("revisions-cwd-and-independent-instances", () => {
    const calls:any[]=[]; const write=(req:any)=>{calls.push(req);return {status:"acknowledged",recorded:1};};
    const a=createOpenCodeUsageRecorder(write as any) as any,b=createOpenCodeUsageRecorder(write as any) as any;
    a({binary:"bin",cwd:"/a",part:part()});
    a({binary:"bin",cwd:"/a",part:part("p",12)});
    a({binary:"bin",cwd:"/b",part:part()});
    b({binary:"bin",cwd:"/a",part:part()});
    expect(calls).toHaveLength(4);
  });
  test("invalid-parts-do-not-write", () => {
    let calls=0;const record=createOpenCodeUsageRecorder((()=>{calls++;return {status:"acknowledged",recorded:1};}) as any) as any;
    for(const p of [undefined,null,{}, {...part(),type:"text"}, {...part(),sessionID:"unknown"}, {...part(),messageID:""}])
      record({binary:"bin",cwd:"/a",part:p});
    expect(calls).toBe(0);
  });
  test("oldest-first-capacity", () => {
    const ids:string[]=[];const record=createOpenCodeUsageRecorder(((r:any)=>{ids.push(r.events[0].attrs.usage_id);return {status:"acknowledged",recorded:1};}) as any) as any;
    const send=(id:string)=>record({binary:"bin",cwd:"/a",part:part(id)});
    for(let i=0;i<1024;i++)send(String(i));
    send("0"); expect(ids).toHaveLength(1024);
    send("1024");send("1");expect(ids).toHaveLength(1025);
    send("0");expect(ids).toHaveLength(1026);
    expect(ids.at(-1)).toBe("0");
  });
});

async function hooks(skipInit=false) {
  const {createHooks}=await import("../src/opencode/hooks.ts");
  return createHooks("/fixture/project","/fixture/project",{
    app:{log:async()=>{}},session:{create:async()=>({id:"s"}),prompt:async()=>({})},
    event:{subscribe:async()=>({stream:[] as any})},
  },undefined,{skipInit});
}
describe("actual OpenCode adapter",()=>{
  test("opencode-object-call-and-retry",async()=>{
    const h=await hooks(); harness.calls.length=0;
    harness.response="Recorded 0 event(s)";
    const input={event:{type:"message.part.updated",properties:{part:part()}}};
    await h.event!(input);harness.response="Recorded 1 event(s)";
    await h.event!(input);await h.event!(input);
    expect(harness.calls).toHaveLength(2);
    expect(JSON.parse(harness.calls[1].options.input)).toEqual(openCodeUsageEvent(part()));
    expect(harness.calls[1].binary).toBe("/fixture/aide");
    expect(harness.calls[1].options.cwd).toBe("/fixture/project");
  });
  test("opencode-ignored-message-and-no-binary",async()=>{
    const h=await hooks();harness.calls.length=0;
    await h.event!({event:{type:"message.updated",properties:{info:{tokens:{input:999}}}}});
    expect(harness.calls).toHaveLength(0);
    const skipped=await hooks(true);
    await skipped.event!({event:{type:"message.part.updated",properties:{part:part()}}});
    expect(harness.calls).toHaveLength(0);
  });
});

let invocation=0;
async function stop(input:object) {
  harness.input=JSON.stringify(input);
  await import(`../src/hooks/session-summary.ts?fixture=${++invocation}`);
  for(let i=0;i<20 && !harness.emitted.length;i++)await new Promise(resolve=>setTimeout(resolve,1));
  expect(harness.emitted).toEqual([{continue:true}]);
}
async function withTranscript(host:"claude-code"|"codex",run:(cwd:string,path:string)=>Promise<void>) {
  const cwd=mkdtempSync(join(tmpdir(),"usage-hidden-")),path=join(cwd,"transcript.jsonl");
  try{
    harness.host=host;
    const row=host==="codex"?{type:"token_usage_record",payload:{thread_id:"s",response_id:"r",usage:{input_tokens:7}}}
      :{type:"assistant",sessionId:"s",message:{id:"r",usage:{input_tokens:7}}};
    writeFileSync(path,JSON.stringify(row)+"\n");await run(cwd,path);
  }finally{rmSync(cwd,{recursive:true,force:true});}
}
describe("actual Stop adapter",()=>{
  for(const host of ["claude-code","codex"] as const)
    test(`stop-${host}-records-and-logs`,async()=>{
      await withTranscript(host,async(cwd,path)=>{
        await stop({hook_event_name:"Stop",session_id:"s",cwd,transcript_path:path});
        expect(harness.calls).toHaveLength(1);
        expect(harness.calls[0].options.cwd).toBe(cwd);
        expect(JSON.parse(harness.calls[0].options.input).attrs).toMatchObject({host,usage_id:"r"});
        expect(harness.logs).toContain("Usage scan: partial; limited=false; malformed=0; records=1; write=acknowledged");
        expect(harness.summaries).toBe(1);
      });
    });
  test("stop-failed-write-still-captures-summary",async()=>{
    await withTranscript("claude-code",async(cwd,path)=>{
      harness.fail=true;
      await stop({hook_event_name:"Stop",session_id:"s",cwd,transcript_path:path});
      expect(harness.logs).toContain("Usage scan: partial; limited=false; malformed=0; records=1; write=unacknowledged");
      expect(harness.summaries).toBe(1);
    });
  });
  test("stop-recursion-only-suppresses-summary",async()=>{
    await withTranscript("claude-code",async(cwd,path)=>{
      await stop({hook_event_name:"Stop",session_id:"s",cwd,transcript_path:path,stop_hook_active:true});
      expect(harness.calls).toHaveLength(1);expect(harness.summaries).toBe(0);
    });
  });
  test("non-stop-and-missing-binary-remain-noop",async()=>{
    await withTranscript("claude-code",async(cwd,path)=>{
      await stop({hook_event_name:"Start",session_id:"s",cwd,transcript_path:path});
      expect(harness.calls).toHaveLength(0);expect(harness.summaries).toBe(0);
      resetHarness();harness.binary=null;
      await stop({hook_event_name:"Stop",session_id:"s",cwd,transcript_path:path});
      expect(harness.calls).toHaveLength(0);expect(harness.summaries).toBe(0);
    });
  });
});

describe("normalization and scanning regression",()=>{
  test("timestamps-and-invalid-counters",()=>{
    const e=claudeUsageEvent({type:"assistant",sessionId:"s",timestamp:"2026-02-30T12:00:00Z",
      message:{id:"m",usage:{input_tokens:-1,cache_read_input_tokens:3,cache_creation_input_tokens:0}}},"s")!;
    expect(e.ts).toBeUndefined();expect(e.attrs?.usage_invalid).toBe("1");
    expect(e.attrs?.input_tokens).toBeUndefined();expect(e.attrs?.uncached_input_tokens).toBeUndefined();
  });
  test("bounded-tail-and-malformed-records",async()=>{
    await withTranscript("claude-code",async(_cwd,path)=>{
      const row=(id:string)=>JSON.stringify({type:"assistant",sessionId:"s",message:{id,usage:{input_tokens:1}}});
      writeFileSync(path,[row("old"),"{bad json}",row("new")].join("\n")+"\n");
      const limited=collectTranscriptUsage(path,"claude-code","s",{maxEvents:1});
      expect(limited.events.map(e=>e.attrs?.usage_id)).toEqual(["new"]);
      expect(limited.limited).toBe(true);expect(limited.malformed).toBe(1);
      const tail=collectTranscriptUsage(path,"claude-code","s",{maxBytes:row("new").length+1});
      expect(tail.events.map(e=>e.attrs?.usage_id)).toEqual(["new"]);expect(tail.limited).toBe(true);
    });
  });
});
