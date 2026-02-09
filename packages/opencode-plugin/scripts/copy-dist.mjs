#!/usr/bin/env node
/**
 * Copies the required dist/ files from the root project build
 * into the opencode-plugin package for standalone distribution.
 *
 * Run from packages/opencode-plugin/ or via `npm run copy-dist`.
 */

import { cpSync, mkdirSync, existsSync, copyFileSync, chmodSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgRoot = join(__dirname, "..");
const repoRoot = join(pkgRoot, "..", "..");

const rootDist = join(repoRoot, "dist");
if (!existsSync(rootDist)) {
  console.error(
    "ERROR: Root dist/ not found. Run `npm run build` in the repo root first.",
  );
  process.exit(1);
}

// Directories to copy wholesale
const dirs = ["opencode", "core", "cli"];
for (const dir of dirs) {
  const src = join(rootDist, dir);
  const dest = join(pkgRoot, "dist", dir);
  if (!existsSync(src)) {
    console.error(`ERROR: ${src} not found`);
    process.exit(1);
  }
  mkdirSync(dest, { recursive: true });
  cpSync(src, dest, { recursive: true });
  console.log(`  copied dist/${dir}/`);
}

// Individual lib files needed by the wrapper/downloader
const libFiles = [
  "lib/aide-downloader.js",
  "lib/aide-downloader.js.map",
  "lib/aide-downloader.d.ts",
  "lib/aide-downloader.d.ts.map",
  "lib/hook-utils.js",
  "lib/hook-utils.js.map",
  "lib/hook-utils.d.ts",
  "lib/hook-utils.d.ts.map",
];

const libDest = join(pkgRoot, "dist", "lib");
mkdirSync(libDest, { recursive: true });

for (const file of libFiles) {
  const src = join(rootDist, file);
  const dest = join(pkgRoot, "dist", file);
  if (existsSync(src)) {
    copyFileSync(src, dest);
    console.log(`  copied dist/${file}`);
  }
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

// Copy the root package.json version into the package's dist for version lookup
// The downloader reads package.json to determine which release to fetch.
// It walks up from its own location (dist/lib/) to find package.json.
// Since the package root has package.json, this works out of the box.

console.log("\nPackage dist/ ready for publishing.");
