#!/usr/bin/env bun
/**
 * check-resolver-guards.ts — guard against new inline project-root resolvers.
 *
 * The Go binary's resolver is the single resolution authority; the TS mirror
 * lives in src/lib/project-root.ts and the anchor reader in src/lib/anchor.ts.
 * History shows every "quick inline walk" drifts (session-end's stale fork,
 * the HUD's .aide-only walk, the OpenCode bypass) — so new resolution logic
 * outside the allowlist fails the build.
 *
 * Rules are exported and covered by check-resolver-guards.test.ts: a guard
 * that silently stops matching is worse than no guard, so the patterns are
 * asserted against known-bad fixtures rather than trusted.
 *
 * Usage: bun run scripts/check-resolver-guards.ts [baseDir]
 */

import { readdirSync, readFileSync, existsSync } from "fs";
import { join, relative, sep } from "path";

export interface Rule {
  name: string;
  message: string;
  pattern: RegExp;
  roots: string[];
  extension: string;
  allow: RegExp;
}

export const RULES: Rule[] = [
  {
    name: "ts-marker-probe",
    message:
      "marker probe outside the resolver allowlist (use findProjectRoot/getAnchoredRoot)",
    // Marker as the FINAL path segment is a resolution probe; join(root, ".aide", "state") is not.
    pattern: /existsSync\(join\([^)]*["']\.(git|aide)["']\s*\)\)/,
    roots: ["src", "scripts"],
    extension: ".ts",
    // session-end.ts keeps a documented inline fallback (no-ES-imports startup
    // constraint); aide-hud.ts keeps a minimal .aide walk as its last rung.
    // Test files are fixtures, not resolution logic.
    allow:
      /^(src\/lib\/project-root\.ts|src\/lib\/anchor\.ts|src\/hooks\/session-end\.ts|scripts\/aide-hud\.ts|src\/test\/|packages\/)|\.test\.ts$/,
  },
  {
    name: "go-getwd",
    message: "os.Getwd() outside the resolver (derive from the resolved root/dbPath)",
    pattern: /os\.Getwd\(\)/,
    roots: ["aide/cmd/aide"],
    extension: ".go",
    // Resolution, plus explicitly reviewed uses: cwd-vs-root validation, instance-info display.
    allow: /(cmd_anchor\.go|main\.go|_test\.go|cmd_session\.go|cmd_mcp_instance\.go)$/,
  },
  {
    name: "go-triple-dir",
    message:
      "new triple-Dir dbPath inversion (use store.ProjectRootFromDB(dbPath))",
    pattern: /Dir\(filepath\.Dir\(filepath\.Dir/,
    roots: ["aide"],
    extension: ".go",
    // Only the canonical definition may spell the inversion out.
    allow: /(_test\.go|pkg\/store\/store\.go)$/,
  },
];

export interface Hit {
  rule: string;
  file: string;
  line: number;
  text: string;
}

const SKIP_DIRS = new Set(["node_modules", "dist", ".git", "vendor"]);

function walk(dir: string, extension: string): string[] {
  if (!existsSync(dir)) return [];
  const out: string[] = [];
  // withFileTypes uses lstat semantics, so a dangling symlink (.aide/bin/aide
  // on a machine that has not built yet) does not throw.
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (SKIP_DIRS.has(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full, extension));
    else if (entry.isFile() && entry.name.endsWith(extension)) out.push(full);
  }
  return out;
}

/** Scan one rule, returning hits that are not covered by its allowlist. */
export function scanRule(rule: Rule, baseDir = "."): Hit[] {
  const hits: Hit[] = [];
  for (const root of rule.roots) {
    for (const file of walk(join(baseDir, root), rule.extension)) {
      // Allowlists are written with forward slashes; normalise for Windows.
      const rel = relative(baseDir, file).split(sep).join("/");
      if (rule.allow.test(rel)) continue;
      readFileSync(file, "utf-8")
        .split("\n")
        .forEach((text, i) => {
          if (rule.pattern.test(text)) {
            hits.push({ rule: rule.name, file: rel, line: i + 1, text: text.trim() });
          }
        });
    }
  }
  return hits;
}

export function scanAll(baseDir = "."): Map<Rule, Hit[]> {
  const results = new Map<Rule, Hit[]>();
  for (const rule of RULES) results.set(rule, scanRule(rule, baseDir));
  return results;
}

if (import.meta.main) {
  const baseDir = process.argv[2] ?? join(import.meta.dir, "..");
  let failed = false;

  for (const [rule, hits] of scanAll(baseDir)) {
    if (hits.length === 0) continue;
    failed = true;
    console.error(`ERROR: ${rule.message}`);
    for (const hit of hits) console.error(`  ${hit.file}:${hit.line}:${hit.text}`);
  }

  if (failed) process.exit(1);
  console.log("resolver guards OK");
}
