#!/usr/bin/env node
// preflight.mjs — ensure Bun exists before running an aide hook.
//
// Invoked as: node preflight.mjs <hook.ts> [args...]
//   - Bun on PATH  → delegates to `bun <hook.ts> ...` (stdio inherited, so the
//     hook's stdin JSON and stdout response pass straight through).
//   - Bun missing  → prints one clear, aide-branded notice and exits 0, so the
//     situation is obvious and traceable instead of a cryptic hook failure.
//
// Runs on plain Node (no Bun) so it works as the bootstrap guard. It prefers the
// `which` package (the project's cross-platform lookup convention) but falls
// back to a manual PATH scan: on a fresh marketplace install node_modules may
// not be restored yet, and the guard must not crash on the very thing it checks.
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join, delimiter } from "node:path";

async function findBun() {
  try {
    const which = (await import("which")).default;
    return which.sync("bun", { nothrow: true });
  } catch {
    const exe = process.platform === "win32" ? "bun.exe" : "bun";
    for (const dir of (process.env.PATH || "").split(delimiter)) {
      if (dir && existsSync(join(dir, exe))) return join(dir, exe);
    }
    return null;
  }
}

const [hook, ...rest] = process.argv.slice(2);
const bun = await findBun();

if (!bun) {
  process.stderr.write(
    "\n⚠ aide: 'bun' was not found on your PATH.\n" +
      "  aide's Claude Code hooks run on the Bun runtime.\n" +
      "  Install it from https://bun.sh, then restart Claude Code.\n\n",
  );
  process.exit(0);
}

const res = spawnSync(bun, [hook, ...rest], { stdio: "inherit" });
process.exit(res.status ?? 0);
