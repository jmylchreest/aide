#!/usr/bin/env bun
/**
 * export-decisions.ts — publish the local decision store to .aide/shared/.
 *
 * Run by the lefthook pre-commit hook. No-op when nothing changed: every byte
 * of the tree, manifest included, is a function of the records it holds, so an
 * unchanged store re-exports byte-identically and stages nothing. Skips
 * silently when there is no aide binary or no local store, so a fresh clone
 * mid-bootstrap can still commit.
 *
 * Usage: bun run scripts/export-decisions.ts
 */

import { spawnSync } from "child_process";
import { existsSync } from "fs";
import { join } from "path";
import which from "which";

const exe = process.platform === "win32" ? ".exe" : "";
const local = join(".aide", "bin", `aide${exe}`);
const aideBin = existsSync(local) ? local : (which.sync("aide", { nothrow: true }) ?? null);

if (!aideBin) {
  console.log("export-decisions: no aide binary (.aide/bin/aide or PATH) - skipping");
  process.exit(0);
}
if (!existsSync(join(".aide", "memory"))) {
  console.log("export-decisions: no local aide store - skipping");
  process.exit(0);
}

const exported = spawnSync(aideBin, ["share", "export", "--decisions"], { stdio: "inherit" });
if (exported.status !== 0) process.exit(exported.status ?? 1);

if (existsSync(".aide/shared")) {
  const staged = spawnSync("git", ["add", ".aide/shared"], { stdio: "inherit" });
  if (staged.status !== 0) process.exit(staged.status ?? 1);
}
