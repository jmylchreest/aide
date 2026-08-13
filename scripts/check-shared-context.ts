#!/usr/bin/env bun
/**
 * check-shared-context.ts — guard the context published in .aide/shared/.
 *
 * Fails if a published record hard-codes a developer-specific absolute path.
 * Version files are write-once, so bad text must be caught before it is
 * committed. Run by the lefthook pre-commit hook and by CI.
 *
 * Usage: bun run scripts/check-shared-context.ts [sharedDir]
 */

import { readdirSync, readFileSync, statSync, existsSync } from "fs";
import { join } from "path";

const sharedDir = process.argv[2] ?? ".aide/shared";

// Narrow on purpose: ~/.aide and $HOME are the portable forms and stay allowed.
const PATTERN = /\/home\/[a-z_][a-z0-9_-]*|\/Users\/[A-Za-z][A-Za-z0-9_-]*|\/root\//;

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...walk(full));
    else if (entry.endsWith(".md") || entry.endsWith(".json")) out.push(full);
  }
  return out;
}

if (!existsSync(sharedDir)) {
  console.log(`check-shared-context: ${sharedDir} does not exist yet - nothing to check`);
  process.exit(0);
}

const files = walk(sharedDir);
const hits: string[] = [];

for (const file of files) {
  const lines = readFileSync(file, "utf-8").split("\n");
  lines.forEach((line, i) => {
    if (PATTERN.test(line)) hits.push(`${file}:${i + 1}:${line.trim()}`);
  });
}

if (hits.length > 0) {
  console.error("ERROR: shared context contains developer-specific absolute paths.\n");
  for (const hit of hits) console.error(hit);
  console.error("\nFix at the source, then re-export:");
  console.error("  aide decision delete <topic>");
  console.error("  aide decision set <topic> ...   # use $HOME or ~/");
  console.error("  aide share export --decisions");
  process.exit(1);
}

console.log(`check-shared-context: OK (${files.length} files checked)`);
