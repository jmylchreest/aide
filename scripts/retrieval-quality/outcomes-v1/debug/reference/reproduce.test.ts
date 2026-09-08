import { test, expect } from 'bun:test';
import { retrievalEvidence } from './src/core/retrieval-evidence.ts';
test('a successful no-match search stays a search', () => {
  const result = retrievalEvidence(import.meta.dir, 'Bash',
    { command: 'rg -n missing sample.txt' }, '', { exit_code: 1 }, false);
  expect(result.retrieval_method).toBe('shell_search');
  expect(result.retrieval_status).toBe('search');
});
