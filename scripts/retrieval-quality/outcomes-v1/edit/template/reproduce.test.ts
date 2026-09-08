import { test, expect } from 'bun:test';
import { replacementTarget } from './src/core/context-pruning/replacement.ts';
test('adapter bash output can be shortened without changing its envelope', () => {
  const original = { output: 'long command output', metadata: { exit: 0, title: 'build' }, id: 'tool-1' };
  const target = replacementTarget('bash', original);
  expect(target?.text).toBe(original.output);
  expect(target?.replace('short')).toEqual({ ...original, output: 'short' });
  expect(original.output).toBe('long command output');
});
