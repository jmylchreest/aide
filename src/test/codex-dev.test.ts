import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join, resolve } from "path";
import * as TOML from "smol-toml";
import { execFileSync, execSync } from "child_process";
import {
  codexDevMode,
  switchCodexDev,
  type CodexDevPaths,
} from "../cli/codex-dev.js";

describe("Codex dev toggle", () => {
  let root: string;
  let paths: CodexDevPaths;
  const binary = process.platform === "win32" ? "aide.exe" : "aide";
  const readConfig = () =>
    TOML.parse(
      readFileSync(join(paths.configDir, "config.toml"), "utf8"),
    ) as any;
  const writeConfig = (config: any) =>
    writeFileSync(join(paths.configDir, "config.toml"), TOML.stringify(config));
  const readHooks = () =>
    JSON.parse(readFileSync(join(paths.configDir, "hooks.json"), "utf8"));
  const writeHooks = (hooks: any) =>
    writeFileSync(join(paths.configDir, "hooks.json"), JSON.stringify(hooks));
  const commands = () =>
    Object.values(readHooks().hooks).flatMap((matchers: any) =>
      matchers.flatMap((matcher: any) =>
        matcher.hooks.map((hook: any) => hook.command),
      ),
    );
  const skill = (dir: string, name: string, content: string) => {
    mkdirSync(join(dir, name), { recursive: true });
    writeFileSync(join(dir, name, "SKILL.md"), content);
  };

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "aide-codex-dev-"));
    paths = {
      repo: join(root, "repo with spaces"),
      configDir: join(root, "codex"),
      skillsDir: join(root, "skills"),
    };
    mkdirSync(paths.configDir, { recursive: true });
    mkdirSync(join(paths.repo, "bin"), { recursive: true });
    writeFileSync(join(paths.repo, "bin", binary), "local build");
    skill(join(paths.repo, "skills"), "test", "dev skill");
  });

  afterEach(() => rmSync(root, { recursive: true, force: true }));

  it("switches a marketplace install and restores it without losing unrelated edits", () => {
    writeConfig({
      model: "original",
      plugins: {
        "aide@aide": { enabled: true, other: "keep" },
        "aide@disabled": { enabled: false },
        "other@market": { enabled: true },
      },
    });
    writeHooks({
      custom: "keep",
      hooks: {
        SessionStart: [
          {
            matcher: "*",
            hooks: [
              {
                type: "command",
                command: "bunx @jmylchreest/aide-plugin hook session-start",
                timeout: 60,
              },
              { type: "command", command: "other-hook" },
            ],
          },
        ],
      },
    });
    skill(paths.skillsDir, "test", "user-owned collision");
    skill(join(paths.repo, "skills"), "recall", "dev recall");

    expect(codexDevMode(paths)).toBe("prod");
    expect(switchCodexDev(paths, "dev")).toContain(
      "installed skills unchanged",
    );
    expect(codexDevMode(paths)).toBe("dev");
    expect(readConfig().mcp_servers.aide.command).toBe(
      join(paths.repo, "bin", binary),
    );
    expect(readConfig().plugins["aide@aide"].enabled).toBe(true);
    expect(readConfig().plugins["aide@aide"].mcp_servers.aide.enabled).toBe(
      false,
    );
    expect(readConfig().plugins["aide@disabled"].enabled).toBe(false);
    expect(commands()).toContain("other-hook");
    expect(
      commands().some(
        (command) =>
          command.includes(join(paths.repo, "src", "cli", "index.ts")) &&
          command.endsWith(" hook session-start"),
      ),
    ).toBe(true);
    expect(
      readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
    ).toBe("user-owned collision");
    expect(existsSync(join(paths.skillsDir, "recall"))).toBe(false);

    const config = readConfig();
    config.model = "changed during dev";
    config.mcp_servers.other = { command: "other" };
    config.plugins["aide@aide"].other = "updated";
    writeConfig(config);
    const hooks = readHooks();
    hooks.hooks.Stop.push({
      matcher: "*",
      hooks: [{ type: "command", command: "new-user-hook" }],
    });
    writeHooks(hooks);
    switchCodexDev(paths, "prod");

    expect(codexDevMode(paths)).toBe("prod");
    expect(readConfig()).toEqual({
      model: "changed during dev",
      plugins: {
        "aide@aide": { enabled: true, other: "updated" },
        "aide@disabled": { enabled: false },
        "other@market": { enabled: true },
      },
      mcp_servers: { other: { command: "other" } },
    });
    expect(commands().sort()).toEqual([
      "bunx @jmylchreest/aide-plugin hook session-start",
      "new-user-hook",
      "other-hook",
    ]);
    expect(readHooks().custom).toBe("keep");
    expect(existsSync(join(paths.skillsDir, "recall"))).toBe(false);
    expect(existsSync(join(paths.skillsDir, ".aide-skills.json"))).toBe(false);
    expect(
      readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
    ).toBe("user-owned collision");
    expect(existsSync(join(paths.configDir, "aide-dev-toggle"))).toBe(false);
  });

  it("leaves standalone skills untouched across repeated toggles", () => {
    const mcp = {
      command: "aide-plugin",
      args: ["mcp"],
      env: { CUSTOM: "value" },
      startup_timeout_sec: 30,
    };
    writeConfig({
      mcp_servers: { aide: mcp },
      features: { hooks: false, other: true },
    });
    skill(paths.skillsDir, "test", "published skill");
    skill(paths.skillsDir, "old", "removed from dev bundle");
    const manifest = JSON.stringify({ skills: ["test", "old"] });
    writeFileSync(join(paths.skillsDir, ".aide-skills.json"), manifest);
    switchCodexDev(paths, "dev");
    expect(existsSync(join(paths.skillsDir, "old"))).toBe(true);
    skill(join(paths.repo, "skills"), "test", "changed local skill");
    switchCodexDev(paths, "dev");
    expect(
      readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
    ).toBe("published skill");
    skill(paths.skillsDir, "test", "edited installed skill");
    expect(
      commands().filter((c) => c.endsWith(" hook session-start")),
    ).toHaveLength(1);
    switchCodexDev(paths, "prod");
    expect(readConfig()).toEqual({
      mcp_servers: { aide: mcp },
      features: { hooks: false, other: true },
    });
    expect(
      readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
    ).toBe("edited installed skill");
    expect(readFileSync(join(paths.skillsDir, "old", "SKILL.md"), "utf8")).toBe(
      "removed from dev bundle",
    );
    expect(
      readFileSync(join(paths.skillsDir, ".aide-skills.json"), "utf8"),
    ).toBe(manifest);
    expect(commands()).toEqual([]);
    expect(switchCodexDev(paths, "prod")).toBe("already in prod mode");
  });

  it("does not create a loose skills directory in either scope", () => {
    for (const configDir of [paths.configDir, join(paths.repo, ".codex")]) {
      const scoped = {
        ...paths,
        configDir,
        skillsDir: join(configDir, "..", ".agents", "skills"),
      };
      mkdirSync(configDir, { recursive: true });
      writeFileSync(
        join(configDir, "config.toml"),
        TOML.stringify({ plugins: { "aide@aide": { enabled: true } } }),
      );
      switchCodexDev(scoped, "dev");
      switchCodexDev(scoped, "dev");
      expect(codexDevMode(scoped)).toBe("dev");
      expect(existsSync(scoped.skillsDir)).toBe(false);
      switchCodexDev(scoped, "prod");
      expect(existsSync(scoped.skillsDir)).toBe(false);
    }
  });

  it.each([true, false, undefined])(
    "restores bundled MCP enablement (%s) and preserves policy edits",
    (enabled) => {
      writeConfig({
        plugins: {
          "aide@aide": {
            enabled: true,
            mcp_servers: {
              aide: {
                ...(enabled === undefined ? {} : { enabled }),
                disabled_tools: ["old"],
              },
            },
          },
        },
      });
      switchCodexDev(paths, "dev");
      expect(codexDevMode(paths)).toBe("dev");
      expect(readConfig().plugins["aide@aide"].mcp_servers.aide.enabled).toBe(
        false,
      );
      const config = readConfig();
      config.plugins["aide@aide"].mcp_servers.aide.disabled_tools = ["new"];
      writeConfig(config);
      switchCodexDev(paths, "dev");
      switchCodexDev(paths, "prod");
      expect(readConfig().plugins["aide@aide"].mcp_servers.aide).toEqual({
        ...(enabled === undefined ? {} : { enabled }),
        disabled_tools: ["new"],
      });
    },
  );

  it.each(["dev", "prod"] as const)(
    "migrates old skill-copying snapshots when switching to %s",
    (mode) => {
      writeConfig({ plugins: { "aide@aide": { enabled: true } } });
      switchCodexDev(paths, "dev");
      const backupDir = join(paths.configDir, "aide-dev-toggle");
      const statePath = join(backupDir, "state.json");
      const state = JSON.parse(readFileSync(statePath, "utf8"));
      delete state.plugins["aide@aide"].mcp;
      state.skills = ["test"];
      state.devSkills = ["test", "recall"];
      state.manifest = JSON.stringify({ skills: ["test"] });
      skill(join(backupDir, "skills"), "test", "original installed skill");
      writeFileSync(statePath, JSON.stringify(state));
      const config = readConfig();
      config.plugins["aide@aide"] = { enabled: false };
      writeConfig(config);
      skill(paths.skillsDir, "test", "old dev copy");
      skill(paths.skillsDir, "recall", "old dev copy");
      skill(paths.skillsDir, "personal", "keep this");
      writeFileSync(
        join(paths.skillsDir, ".aide-skills.json"),
        JSON.stringify({ skills: state.devSkills }),
      );

      switchCodexDev(paths, mode);
      expect(readConfig().plugins["aide@aide"].enabled).toBe(true);
      expect(codexDevMode(paths)).toBe(mode);
      expect(existsSync(join(paths.skillsDir, "recall"))).toBe(false);
      expect(
        readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
      ).toBe("original installed skill");
      expect(
        readFileSync(join(paths.skillsDir, "personal", "SKILL.md"), "utf8"),
      ).toBe("keep this");
      expect(
        readFileSync(join(paths.skillsDir, ".aide-skills.json"), "utf8"),
      ).toBe(state.manifest);
      if (mode === "dev") {
        skill(paths.skillsDir, "test", "edited after migration");
        switchCodexDev(paths, "dev");
        switchCodexDev(paths, "prod");
        expect(
          readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
        ).toBe("edited after migration");
        expect(readConfig().plugins["aide@aide"]).toEqual({ enabled: true });
      }
    },
  );

  it("removes old generated copies and their manifest when there were no installed skills", () => {
    writeConfig({
      mcp_servers: { aide: { command: "aide-plugin", args: ["mcp"] } },
    });
    switchCodexDev(paths, "dev");
    const statePath = join(paths.configDir, "aide-dev-toggle", "state.json");
    const state = JSON.parse(readFileSync(statePath, "utf8"));
    state.devSkills = ["test"];
    writeFileSync(statePath, JSON.stringify(state));
    skill(paths.skillsDir, "test", "old generated copy");
    writeFileSync(
      join(paths.skillsDir, ".aide-skills.json"),
      JSON.stringify({ skills: ["test"] }),
    );
    switchCodexDev(paths, "dev");
    expect(existsSync(join(paths.skillsDir, "test"))).toBe(false);
    expect(existsSync(join(paths.skillsDir, ".aide-skills.json"))).toBe(false);
    switchCodexDev(paths, "dev");
    expect(existsSync(join(paths.skillsDir, "test"))).toBe(false);
    switchCodexDev(paths, "prod");
    expect(existsSync(join(paths.skillsDir, "test"))).toBe(false);
  });

  it("restores plugin skills after a migration interrupted before config was written", () => {
    writeConfig({ plugins: { "aide@aide": { enabled: true } } });
    switchCodexDev(paths, "dev");
    const config = readConfig();
    // Snapshot migration finished, but the old whole-plugin disable is still on disk.
    config.plugins["aide@aide"].enabled = false;
    writeConfig(config);
    switchCodexDev(paths, "dev");
    expect(readConfig().plugins["aide@aide"].enabled).toBe(true);
    expect(readConfig().plugins["aide@aide"].mcp_servers.aide.enabled).toBe(
      false,
    );
    expect(codexDevMode(paths)).toBe("dev");
  });

  it("does not install aide into an unconfigured scope", () => {
    writeConfig({ model: "keep" });
    expect(codexDevMode(paths)).toBe("not-installed");
    expect(switchCodexDev(paths, "dev")).toBe("not installed - skipping");
    expect(switchCodexDev(paths, "prod")).toBe("not installed - skipping");
    expect(readConfig()).toEqual({ model: "keep" });
    expect(existsSync(join(paths.configDir, "hooks.json"))).toBe(false);
  });

  it("recognizes the same checkout through a directory alias without accepting another checkout", () => {
    mkdirSync(join(paths.repo, "src", "cli"), { recursive: true });
    writeFileSync(join(paths.repo, "src", "cli", "index.ts"), "local cli");
    writeFileSync(join(paths.repo, "bin", "aide-wrapper.ts"), "local wrapper");
    const alias = join(
      root,
      process.platform === "win32" ? "alias with spaces" : "alias's checkout",
    );
    symlinkSync(
      paths.repo,
      alias,
      process.platform === "win32" ? "junction" : "dir",
    );
    const quote = (value: string) =>
      process.platform === "win32"
        ? `"${value.replace(/"/g, '\\"')}"`
        : `'${value.replace(/'/g, "'\\''")}'`;
    for (const mcp of [
      { command: join(alias, "bin", binary), args: ["mcp"] },
      { command: "bun", args: [join(alias, "bin", "aide-wrapper.ts"), "mcp"] },
      { command: "bun", args: [join(alias, "src", "cli", "index.ts"), "mcp"] },
    ]) {
      writeConfig({ mcp_servers: { aide: mcp } });
      writeHooks({
        hooks: {
          SessionStart: [
            {
              matcher: "*",
              hooks: [
                {
                  type: "command",
                  command: `bun ${quote(join(alias, "src", "cli", "index.ts"))} hook session-start`,
                },
              ],
            },
          ],
        },
      });
      expect(codexDevMode(paths)).toBe("dev");
    }
    const other = join(root, "other checkout");
    mkdirSync(join(other, "src", "cli"), { recursive: true });
    writeFileSync(join(other, "src", "cli", "index.ts"), "foreign cli");
    writeConfig({
      mcp_servers: {
        aide: {
          command: "bun",
          args: [join(other, "src", "cli", "index.ts"), "mcp"],
        },
      },
    });
    writeHooks({
      hooks: {
        SessionStart: [
          {
            matcher: "*",
            hooks: [
              {
                type: "command",
                command: `bun ${quote(join(other, "src", "cli", "index.ts"))} hook session-start`,
              },
            ],
          },
        ],
      },
    });
    expect(codexDevMode(paths)).toBe("prod");
  });

  it("recognizes hooks-only installs and reports conflicting components", () => {
    writeHooks({
      hooks: {
        Stop: [
          {
            matcher: "*",
            hooks: [
              { type: "command", command: "aide-plugin hook persistence" },
            ],
          },
        ],
      },
    });
    expect(codexDevMode(paths)).toBe("prod");
    switchCodexDev(paths, "dev");
    const config = readConfig();
    config.plugins = { "aide@aide": { enabled: true } };
    writeConfig(config);
    expect(codexDevMode(paths)).toBe("mixed");
    switchCodexDev(paths, "dev");
    expect(codexDevMode(paths)).toBe("dev");
    switchCodexDev(paths, "prod");
    expect(readConfig().plugins["aide@aide"].enabled).toBe(true);
    expect(commands()).toEqual(["aide-plugin hook persistence"]);
  });

  it("refuses corrupt config and hooks without changing files", () => {
    writeFileSync(join(paths.configDir, "config.toml"), "[broken");
    expect(() => switchCodexDev(paths, "dev")).toThrow();
    expect(readFileSync(join(paths.configDir, "config.toml"), "utf8")).toBe(
      "[broken",
    );
    writeConfig({ plugins: { "aide@aide": {} } });
    writeFileSync(join(paths.configDir, "hooks.json"), "{broken");
    expect(() => switchCodexDev(paths, "dev")).toThrow();
    expect(readConfig().plugins["aide@aide"]).toEqual({});
    expect(existsSync(join(paths.configDir, "aide-dev-toggle"))).toBe(false);
  });

  it("refuses to overwrite a different checkout's restoration data", () => {
    writeConfig({ plugins: { "aide@aide": {} } });
    switchCodexDev(paths, "dev");
    const other = join(root, "other");
    mkdirSync(other);
    expect(() => switchCodexDev({ ...paths, repo: other }, "dev")).toThrow(
      "run its prod toggle first",
    );
    expect(() => switchCodexDev({ ...paths, repo: other }, "prod")).toThrow(
      "run its prod toggle first",
    );
    switchCodexDev(paths, "prod");
    expect(readConfig().plugins["aide@aide"]).toEqual({});
  });

  it("rejects unsafe skill manifest paths before writing config", () => {
    writeConfig({ plugins: { "aide@aide": {} } });
    mkdirSync(paths.skillsDir);
    writeFileSync(
      join(paths.skillsDir, ".aide-skills.json"),
      JSON.stringify({ skills: ["../outside"] }),
    );
    expect(() => switchCodexDev(paths, "dev")).toThrow(
      "Invalid aide skills manifest",
    );
    expect(readConfig().plugins["aide@aide"]).toEqual({});
  });

  it.each(["dev", "prod"] as const)(
    "migrates pre-toggle local installs when first switching to %s",
    (mode) => {
      writeConfig({
        mcp_servers: {
          aide: {
            command: "bun",
            args: [join(paths.repo, "bin", "aide-wrapper.ts"), "mcp"],
            env: { AIDE_PLUGIN_ROOT: paths.repo, CUSTOM: "keep" },
          },
        },
      });
      writeHooks({
        hooks: {
          Stop: [
            {
              matcher: "*",
              hooks: [
                {
                  type: "command",
                  command: `bun ${join(paths.repo, "src", "cli", "index.ts")} hook persistence`,
                },
              ],
            },
          ],
        },
      });
      expect(codexDevMode(paths)).toBe("dev");
      switchCodexDev(paths, mode);
      if (mode === "dev") switchCodexDev(paths, "prod");
      expect(codexDevMode(paths)).toBe("prod");
      expect(readConfig().mcp_servers.aide).toEqual({
        command: "bunx",
        args: ["-y", "@jmylchreest/aide-plugin", "mcp"],
        env: { CUSTOM: "keep" },
      });
      expect(commands()).toEqual([
        "bunx -y @jmylchreest/aide-plugin hook persistence",
      ]);
    },
  );

  it("generates executable hook commands for checkout paths containing shell characters", () => {
    paths.repo = join(
      root,
      process.platform === "win32"
        ? "repo with spaces"
        : "repo's $local `build`",
    );
    mkdirSync(join(paths.repo, "bin"), { recursive: true });
    writeFileSync(join(paths.repo, "bin", binary), "local build");
    skill(join(paths.repo, "skills"), "test", "dev");
    mkdirSync(join(paths.repo, "src", "cli"), { recursive: true });
    writeFileSync(
      join(paths.repo, "src", "cli", "index.ts"),
      "console.log(JSON.stringify(process.argv.slice(2)));",
    );
    writeConfig({ plugins: { "aide@aide": {} } });
    switchCodexDev(paths, "dev");
    expect(codexDevMode(paths)).toBe("dev");
    const command = commands().find((command) =>
      command.endsWith(" hook session-start"),
    );
    expect(JSON.parse(execSync(command!, { encoding: "utf8" }))).toEqual([
      "hook",
      "session-start",
    ]);
    switchCodexDev(paths, "prod");
    expect(commands()).toEqual([]);
  });
});

describe("Codex dev toggle across inherited scopes", () => {
  let root: string;
  let repo: string;
  let globalDir: string;
  let projectDir: string;
  const binary = process.platform === "win32" ? "aide.exe" : "aide";
  const script = resolve("scripts/codex-dev-toggle.ts");
  const readHooks = (dir: string) =>
    JSON.parse(readFileSync(join(dir, "hooks.json"), "utf8"));
  const commands = (dir: string): string[] =>
    Object.values(readHooks(dir).hooks).flatMap((groups: any) =>
      groups.flatMap((group: any) =>
        group.hooks.map((hook: any) => hook.command),
      ),
    );
  const seed = (dir: string) => {
    mkdirSync(dir, { recursive: true });
    writeFileSync(
      join(dir, "config.toml"),
      TOML.stringify({
        mcp_servers: {
          aide: { command: "bunx", args: ["@jmylchreest/aide-plugin", "mcp"] },
        },
      }),
    );
    writeFileSync(
      join(dir, "hooks.json"),
      JSON.stringify({
        custom: "keep",
        hooks: {
          SessionStart: [
            {
              matcher: "*",
              hooks: [
                {
                  type: "command",
                  command: "bunx @jmylchreest/aide-plugin hook session-start",
                },
                { type: "command", command: "unrelated-hook" },
              ],
            },
          ],
        },
      }),
    );
  };
  const run = (action: string) =>
    execFileSync("bun", [script, action, repo], {
      env: { ...process.env, CODEX_HOME: globalDir },
      encoding: "utf8",
    });
  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "aide-codex-scopes-"));
    repo = join(root, "repo");
    globalDir = join(root, "global");
    projectDir = join(repo, ".codex");
    mkdirSync(join(repo, "bin"), { recursive: true });
    writeFileSync(join(repo, "bin", binary), "local build");
  });
  afterEach(() => rmSync(root, { recursive: true, force: true }));

  it("registers inherited hooks once, remains idempotent, and restores both saved scopes", () => {
    seed(globalDir);
    seed(projectDir);
    expect(run("status")).toContain("duplicate aide hooks");
    run("dev");
    run("dev");
    expect(commands(projectDir)).toEqual(["unrelated-hook"]);
    expect(
      commands(globalDir).filter((command) =>
        command.endsWith(" hook session-start"),
      ),
    ).toHaveLength(1);
    expect(run("status")).not.toContain("duplicate aide hooks");
    expect(run("mode").trim()).toBe("dev");
    const hooks = readHooks(projectDir);
    hooks.hooks.SessionStart[0].hooks.push({
      type: "command",
      command: "added-during-dev",
    });
    writeFileSync(join(projectDir, "hooks.json"), JSON.stringify(hooks));
    expect(run("prod")).toContain("duplicate aide hooks");
    expect(commands(projectDir)).toEqual([
      "unrelated-hook",
      "added-during-dev",
      "bunx @jmylchreest/aide-plugin hook session-start",
    ]);
    expect(readHooks(projectDir).custom).toBe("keep");
  });

  it("does not claim duplicate registrations for disjoint matchers", () => {
    seed(globalDir);
    seed(projectDir);
    for (const [dir, matcher] of [
      [globalDir, "startup"],
      [projectDir, "resume"],
    ]) {
      const hooks = readHooks(dir);
      hooks.hooks.SessionStart[0].matcher = matcher;
      writeFileSync(join(dir, "hooks.json"), JSON.stringify(hooks));
    }
    expect(run("status")).not.toContain("duplicate aide hooks");
  });

  it("does not suppress hooks when CODEX_HOME points at the project config", () => {
    seed(projectDir);
    execFileSync("bun", [script, "dev", repo], {
      env: { ...process.env, CODEX_HOME: projectDir },
      encoding: "utf8",
    });
    expect(
      commands(projectDir).filter((command) =>
        command.endsWith(" hook session-start"),
      ),
    ).toHaveLength(1);
  });

  it.each(["global", "project"])(
    "keeps a %s-only install in its existing scope",
    (scope) => {
      const installed = scope === "global" ? globalDir : projectDir;
      const absent = scope === "global" ? projectDir : globalDir;
      seed(installed);
      run("dev");
      run("dev");
      expect(
        commands(installed).filter((command) =>
          command.endsWith(" hook session-start"),
        ),
      ).toHaveLength(1);
      expect(existsSync(join(absent, "hooks.json"))).toBe(false);
      expect(run("status")).not.toContain("duplicate aide hooks");
      run("prod");
      expect(commands(installed)).toEqual([
        "unrelated-hook",
        "bunx @jmylchreest/aide-plugin hook session-start",
      ]);
    },
  );
});
