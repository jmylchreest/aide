#!/usr/bin/env node
/**
 * Copies TypeScript source files from the root project into the
 * opencode-plugin package for standalone distribution.
 *
 * Since OpenCode runs on Bun (which natively supports TypeScript),
 * we ship .ts source directly — no compilation step needed.
 *
 * Run from packages/opencode-plugin/ or via `npm run copy-src`.
 */

import { cpSync, mkdirSync, existsSync, copyFileSync, chmodSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgRoot = join(__dirname, "..");
const repoRoot = join(pkgRoot, "..", "..");

const rootSrc = join(repoRoot, "src");
if (!existsSync(rootSrc)) {
  console.error("ERROR: Root src/ not found.");
  process.exit(1);
}

// Source directories to copy wholesale
const dirs = ["opencode", "core", "cli", "lib"];
for (const dir of dirs) {
  const src = join(rootSrc, dir);
  const dest = join(pkgRoot, "src", dir);
  if (!existsSync(src)) {
    console.error(`ERROR: ${src} not found`);
    process.exit(1);
  }
  mkdirSync(dest, { recursive: true });
  cpSync(src, dest, { recursive: true });
  console.log(`  copied src/${dir}/`);
}

// Copy bin/aide-wrapper.sh
const binDest = join(pkgRoot, "bin");
mkdirSync(binDest, { recursive: true });

const wrapperSrc = join(repoRoot, "bin", "aide-wrapper.sh");
const wrapperDest = join(binDest, "aide-wrapper.sh");
if (existsSync(wrapperSrc)) {
  copyFileSync(wrapperSrc, wrapperDest);
  chmodSync(wrapperDest, 0o755);
  console.log("  copied bin/aide-wrapper.sh");
}

console.log("\nPackage src/ ready for publishing.");
