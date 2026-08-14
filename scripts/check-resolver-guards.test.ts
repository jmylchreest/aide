/**
 * Fixture tests for the resolver guards.
 *
 * The shell version these replaced used `grep ... || true`, so a regex that
 * stopped matching reported "OK" forever with no signal. Each rule is asserted
 * to catch a known-bad snippet AND to respect its allowlist, so a broken
 * pattern fails loudly here instead of silently passing in CI.
 */

import { describe, expect, it, beforeAll, afterAll } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "fs";
import { tmpdir } from "os";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

import { RULES, scanRule, scanAll } from "./check-resolver-guards.ts";

let base: string;

function write(relPath: string, content: string): void {
  const full = join(base, relPath);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, content);
}

const rule = (name: string) => {
  const found = RULES.find((r) => r.name === name);
  if (!found) throw new Error(`no such rule: ${name}`);
  return found;
};

beforeAll(() => {
  base = mkdtempSync(join(tmpdir(), "resolver-guards-"));

  // ts-marker-probe
  write("src/hooks/rogue.ts", 'if (existsSync(join(dir, ".git"))) return dir;\n');
  write("src/lib/project-root.ts", 'if (existsSync(join(dir, ".aide"))) return dir;\n');
  write("src/lib/paths.ts", 'const p = join(root, ".aide", "state");\n');

  // go-getwd
  write("aide/cmd/aide/cmd_rogue.go", "cwd, _ := os.Getwd()\n");
  write("aide/cmd/aide/cmd_anchor.go", "cwd, _ := os.Getwd()\n");

  // go-triple-dir
  write("aide/pkg/thing/rogue.go", "root := filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))\n");
  write("aide/pkg/store/store.go", "return filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))\n");
  write("aide/cmd/aide/helpers.go", "root := filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))\n");
});

afterAll(() => rmSync(base, { recursive: true, force: true }));

describe("ts-marker-probe", () => {
  it("catches an inline marker probe outside the allowlist", () => {
    const hits = scanRule(rule("ts-marker-probe"), base);
    expect(hits.map((h) => h.file)).toContain("src/hooks/rogue.ts");
  });

  it("allows the blessed resolver files", () => {
    const hits = scanRule(rule("ts-marker-probe"), base);
    expect(hits.map((h) => h.file)).not.toContain("src/lib/project-root.ts");
  });

  it("ignores paths beneath .aide/ that are not resolution probes", () => {
    const hits = scanRule(rule("ts-marker-probe"), base);
    expect(hits.map((h) => h.file)).not.toContain("src/lib/paths.ts");
  });
});

describe("go-getwd", () => {
  it("catches os.Getwd() outside the resolver", () => {
    const hits = scanRule(rule("go-getwd"), base);
    expect(hits.map((h) => h.file)).toContain("aide/cmd/aide/cmd_rogue.go");
  });

  it("allows the reviewed callers", () => {
    const hits = scanRule(rule("go-getwd"), base);
    expect(hits.map((h) => h.file)).not.toContain("aide/cmd/aide/cmd_anchor.go");
  });
});

describe("go-triple-dir", () => {
  it("catches a new triple-Dir dbPath inversion", () => {
    const hits = scanRule(rule("go-triple-dir"), base);
    expect(hits.map((h) => h.file)).toContain("aide/pkg/thing/rogue.go");
  });

  it("allows only the canonical definition", () => {
    const hits = scanRule(rule("go-triple-dir"), base);
    expect(hits.map((h) => h.file)).not.toContain("aide/pkg/store/store.go");
  });

  // The allowlist used to name every file holding a copy, which let the
  // duplication accumulate. Callers go through store.ProjectRootFromDB now.
  it("catches a copy in a former allowlist entry", () => {
    const hits = scanRule(rule("go-triple-dir"), base);
    expect(hits.map((h) => h.file)).toContain("aide/cmd/aide/helpers.go");
  });
});

describe("the repo itself", () => {
  it("passes every guard", () => {
    for (const [r, hits] of scanAll(fileURLToPath(new URL("..", import.meta.url)))) {
      expect(hits, `${r.name}: ${hits.map((h) => `${h.file}:${h.line}`).join(", ")}`).toEqual([]);
    }
  });
});
