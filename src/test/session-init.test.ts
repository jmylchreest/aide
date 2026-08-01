/**
 * Tests for session initialization helpers
 *
 * Run with: npx vitest run src/test/session-init.test.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  mkdtempSync,
  rmSync,
  writeFileSync,
  readFileSync,
  mkdirSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

let tempHome = "";

vi.mock("os", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("os");
  return {
    ...actual,
    homedir: () => tempHome,
  };
});

describe("ensureDirectories", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-session-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-home-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("creates required directories and gitignore", async () => {
    const { ensureDirectories } = await import("../core/session-init.js");

    const result = ensureDirectories(projectDir);
    expect(result.created).toBeGreaterThan(0);

    const gitignorePath = join(projectDir, ".aide", ".gitignore");
    const gitignoreContent = readFileSync(gitignorePath, "utf-8");
    expect(gitignoreContent).toContain("!shared/");
    expect(gitignoreContent).toContain("config/mcp.json");
    expect(gitignoreContent).toContain("grammars/");
    expect(gitignoreContent).toContain("aide.sock");
  });

  it("migrates old gitignore to include grammars and aide.sock", async () => {
    const { ensureDirectories } = await import("../core/session-init.js");

    // Create an old-style gitignore without grammars/ or aide.sock
    const aideDir = join(projectDir, ".aide");
    mkdirSync(aideDir, { recursive: true });
    writeFileSync(
      join(aideDir, ".gitignore"),
      `# AIDE local runtime files
_logs/
state/
bin/
worktrees/
memory/
code/

config/mcp.json
config/mcp-sync.journal.json

aide-memory.db

!shared/
`,
    );

    ensureDirectories(projectDir);

    const gitignoreContent = readFileSync(join(aideDir, ".gitignore"), "utf-8");
    expect(gitignoreContent).toContain("grammars/");
    expect(gitignoreContent).toContain("aide.sock");
  });
});

describe("loadConfig", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-session-config-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-home-config-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("returns default config when missing", async () => {
    const { loadConfig } = await import("../core/session-init.js");
    const { DEFAULT_CONFIG } = await import("../core/types.js");
    const config = loadConfig(projectDir);
    expect(config).toEqual(DEFAULT_CONFIG);
  });

  it("merges user config with defaults", async () => {
    const { loadConfig } = await import("../core/session-init.js");

    const configDir = join(projectDir, ".aide", "config");
    mkdirSync(configDir, { recursive: true });
    writeFileSync(
      join(configDir, "aide.json"),
      JSON.stringify({ share: { autoImport: true } }, null, 2),
    );

    const config = loadConfig(projectDir);
    expect(config.share?.autoImport).toBe(true);
  });
});

describe("buildWelcomeContext codebase map", () => {
  const state = {
    sessionId: "abcd1234efgh",
    cwd: "/tmp/proj",
    activeMode: null,
    agentCount: 0,
  };
  const emptyInjection = () => ({
    static: { global: [], project: [], decisions: [] },
    dynamic: { sessions: [] },
  });

  it("renders the map section with freshness note after content sections", async () => {
    const { buildWelcomeContext } = await import("../core/session-init.js");
    const injection = {
      ...emptyInjection(),
      codebaseMap: [
        { name: "observe", size: 74, hub: "aide/pkg/observe/observe.go" },
        { name: "logger", size: 65, hub: "src/lib/logger.ts" },
      ],
      codebaseMapNote: "as of a1b2c3d4 — 3 commits behind; run survey_run to refresh",
    };
    const ctx = buildWelcomeContext(state as never, injection as never);

    expect(ctx).toContain(
      "## Codebase Map (as of a1b2c3d4 — 3 commits behind; run survey_run to refresh)",
    );
    expect(ctx).toContain(
      "- **observe** — 74 files, hub: aide/pkg/observe/observe.go",
    );
    expect(ctx).toContain("- **logger** — 65 files, hub: src/lib/logger.ts");
    // Appended after memories/decisions, before the modes footer.
    expect(ctx.indexOf("## Codebase Map")).toBeLessThan(
      ctx.indexOf("## Available Modes"),
    );
  });

  it("omits the section entirely when no modules exist", async () => {
    const { buildWelcomeContext } = await import("../core/session-init.js");
    const ctx = buildWelcomeContext(state as never, emptyInjection() as never);
    expect(ctx).not.toContain("Codebase Map");
  });
});

describe("buildWelcomeContext decision precedence", () => {
  const state = {
    sessionId: "abcd1234efgh",
    cwd: "/tmp/proj",
    activeMode: null,
    agentCount: 0,
  };

  it("renders overriding decisions ahead of ordinary ones, with the override claim stated", async () => {
    const { buildWelcomeContext } = await import("../core/session-init.js");
    const injection = {
      static: {
        global: [],
        project: [],
        decisions: ["**python-testing**: Test with pytest"],
        overridingDecisions: [
          "**existing-codebase-precedence**: The repository is the authority",
        ],
      },
      dynamic: { sessions: [] },
    };
    const ctx = buildWelcomeContext(state as never, injection as never);

    expect(ctx).toContain("## Overriding Decisions");
    expect(ctx).toContain("## Project Decisions");
    // Ordering is the mechanism: guardrails must be read before what they override.
    expect(ctx.indexOf("## Overriding Decisions")).toBeLessThan(
      ctx.indexOf("## Project Decisions"),
    );
    // Ordering alone does not create precedence — the header must say so.
    expect(ctx).toContain("These take precedence over every decision below");
    expect(ctx).toContain("**existing-codebase-precedence**");
    expect(ctx).toContain("**python-testing**");
  });

  it("omits the overriding block when nothing claims precedence", async () => {
    const { buildWelcomeContext } = await import("../core/session-init.js");
    const injection = {
      static: {
        global: [],
        project: [],
        decisions: ["**python-testing**: Test with pytest"],
        overridingDecisions: [],
      },
      dynamic: { sessions: [] },
    };
    const ctx = buildWelcomeContext(state as never, injection as never);
    expect(ctx).not.toContain("## Overriding Decisions");
    expect(ctx).toContain("## Project Decisions");
  });

  it("tolerates an injection built without the field", async () => {
    const { buildWelcomeContext } = await import("../core/session-init.js");
    const injection = {
      static: { global: [], project: [], decisions: ["**a**: b"] },
      dynamic: { sessions: [] },
    };
    const ctx = buildWelcomeContext(state as never, injection as never);
    expect(ctx).not.toContain("## Overriding Decisions");
  });
});
