import { test, expect, afterAll } from 'bun:test';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const { retrievalEvidence: observe } = await import(join(process.env.AIDE_TRIAL_ROOT!, 'src/core/retrieval-evidence.ts'));
const root = mkdtempSync(join(tmpdir(), 'aide-debug-oracle-'));
writeFileSync(join(root, 'sample.txt'), 'one\ntwo\nthree\n');
afterAll(() => rmSync(root, { recursive: true, force: true }));
for (const command of ['rg absent sample.txt', 'grep -n absent sample.txt', 'rtk rg absent sample.txt', 'rtk proxy grep absent sample.txt']) {
  test(`no-match: ${command}`, () => expect(observe(root, 'Bash', { command }, '', { exit_code: 1 }, false).retrieval_status).toBe('search'));
}
test('hook and camelCase exit statuses support no-match', () => {
  expect(observe(root,'Bash',{command:'rg absent sample.txt'},'',{},false,1).retrieval_status).toBe('search');
  expect(observe(root,'Bash',{command:'grep absent sample.txt'},'',{exitCode:1},false).retrieval_status).toBe('search');
});
test('real search errors still fail', () => expect(observe(root,'Bash',{command:'rg absent sample.txt'},'',{exit_code:2},false).retrieval_status).toBe('failed'));
test('explicit failure, timeout and interruption dominate no-match', () => {
  for (const [response,failed] of [[{exit_code:1},true],[{exit_code:1,timed_out:true},false],[{exit_code:1,interrupted:true},false]])
    expect(observe(root,'Bash',{command:'rg absent sample.txt'},'',response,failed).retrieval_status).toBe('failed');
});
test('a nonsearch status one is a failure', () => expect(observe(root,'Bash',{command:'cat sample.txt'},'one\ntwo\nthree\n',{exit_code:1},false).retrieval_status).toBe('failed'));
test('conflicting and malformed success signals stay unverified', () => {
  expect(observe(root,'Bash',{command:'rg absent sample.txt'},'',{exit_code:1,exitCode:0},false).retrieval_status).toBe('unverified');
  expect(observe(root,'Bash',{command:'rg absent sample.txt'},'',{exit_code:'1'},false).retrieval_status).toBe('unverified');
});
test('search evidence never certifies a full source baseline', () => {
  const result=observe(root,'Bash',{command:'rg absent sample.txt'},'',{exit_code:1},false);
  expect(result.source_references).toBeUndefined(); expect(result.source_verification).toBeUndefined();
});
test('full and bounded reads still verify against current source', () => {
  expect(observe(root,'Bash',{command:'cat sample.txt'},'one\ntwo\nthree\n',{exit_code:0},false).retrieval_status).toBe('full_file');
  expect(observe(root,'Bash',{command:"sed -n '2p' sample.txt"},'two\n',{exit_code:0},false).retrieval_status).toBe('range');
});
test('unrecognized shell syntax cannot gain search certification', () => {
  expect(observe(root,'Bash',{command:'cat sample.txt | rg absent'},'',{exit_code:1},false).retrieval_status).toBe('unclassified_shell');
});
