import { test, expect } from 'bun:test';
import { join } from 'node:path';
const { replacementTarget: target } = await import(join(process.env.AIDE_TRIAL_ROOT!, 'src/core/context-pruning/replacement.ts'));
test('valid adapter exit statuses and casing preserve all metadata', () => {
  for (const exit of [0,1,2,-1]) for (const tool of ['Bash','bash','BASH']) {
    const payload={output:'original',metadata:{exit,duration:14,custom:{x:1}},id:'a'};
    const picked=target(tool,payload);
    expect(picked?.text).toBe('original');
    expect(picked?.replace('new')).toEqual({...payload,output:'new'});
    expect(payload.output).toBe('original');
  }
});
test('known empty output is supported', () => expect(target('Bash',{output:'',metadata:{exit:0}})?.text).toBe(''));
test('malformed output or exit metadata is unsupported', () => {
  for (const payload of [null,[],{output:1,metadata:{exit:0}},{output:'x'}, {output:'x',metadata:null},{output:'x',metadata:[]}, {output:'x',metadata:{exit:'0'}},{output:'x',metadata:{exit:0.5}},{output:'x',metadata:{exit:NaN}},{output:'x',metadata:{exit:Infinity}},{output:'x',metadata:{exit:Number.MAX_SAFE_INTEGER+1}}])
    expect(target('Bash',payload)).toBeNull();
});
test('adapter envelopes with additional representations are unsupported', () => {
  for (const extra of [{content:[]},{structuredContent:{}},{stdout:'x'},{stderr:''},{isImage:true}])
    expect(target('Bash',{output:'x',metadata:{exit:0},...extra})).toBeNull();
});
test('legacy bash keeps longest-stream selection and ties favor stdout', () => {
  for (const [stdout,stderr,key] of [['long','x','stdout'],['x','long','stderr'],['same','same','stdout']]) {
    const payload={stdout,stderr,isImage:false,interrupted:false,extra:9};
    expect(target('Bash',payload)?.replace('new')).toEqual({...payload,[key]:'new'});
  }
});
test('legacy invalid media stays unsupported', () => expect(target('Bash',{stdout:'x',stderr:'',isImage:true,interrupted:false})).toBeNull());
test('MCP text behavior and block metadata survive', () => {
  const payload={content:[{type:'text',text:'one',annotations:{priority:1}},{type:'text',text:'two'}],_meta:{id:'a'}};
  const picked=target('mcp__demo__tool',payload);
  expect(picked?.text).toBe('onetwo');
  expect(picked?.replace('new')).toEqual({...payload,content:[{...payload.content[0],text:'new'},{...payload.content[1],text:''}]});
  expect(target('mcp__demo__tool','raw')?.replace('new')).toBe('new');
});
test('mixed MCP media and structured content remain unsupported', () => {
  expect(target('mcp__demo__tool',{content:[{type:'text',text:'x'},{type:'image',data:'abc'}]})).toBeNull();
  expect(target('mcp__demo__tool',{content:[{type:'text',text:'x'}],structuredContent:{}})).toBeNull();
});
test('native unrelated tools remain unsupported', () => expect(target('Read',{output:'x',metadata:{exit:0}})).toBeNull());
