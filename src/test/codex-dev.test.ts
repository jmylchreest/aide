import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join } from "path";
import * as TOML from "smol-toml";
import { execSync } from "child_process";
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
    expect(switchCodexDev(paths, "dev")).toContain("kept 1 user-owned skills");
    expect(codexDevMode(paths)).toBe("dev");
    expect(readConfig().mcp_servers.aide.command).toBe(
      join(paths.repo, "bin", binary),
    );
    expect(readConfig().plugins["aide@aide"].enabled).toBe(false);
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
    expect(
      readFileSync(join(paths.skillsDir, "recall", "SKILL.md"), "utf8"),
    ).toBe("dev recall");

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

  it("refreshes dev skills without overwriting the original standalone backup", () => {
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
    expect(existsSync(join(paths.skillsDir, "old"))).toBe(false);
    skill(join(paths.repo, "skills"), "test", "changed local skill");
    switchCodexDev(paths, "dev");
    expect(
      readFileSync(join(paths.skillsDir, "test", "SKILL.md"), "utf8"),
    ).toBe("changed local skill");
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
    ).toBe("published skill");
    expect(readFileSync(join(paths.skillsDir, "old", "SKILL.md"), "utf8")).toBe(
      "removed from dev bundle",
    );
    expect(
      readFileSync(join(paths.skillsDir, ".aide-skills.json"), "utf8"),
    ).toBe(manifest);
    expect(commands()).toEqual([]);
    expect(switchCodexDev(paths, "prod")).toBe("already in prod mode");
  });

  it("does not install aide into an unconfigured scope", () => {
    writeConfig({ model: "keep" });
    expect(codexDevMode(paths)).toBe("not-installed");
    expect(switchCodexDev(paths, "dev")).toBe("not installed - skipping");
    expect(switchCodexDev(paths, "prod")).toBe("not installed - skipping");
    expect(readConfig()).toEqual({ model: "keep" });
    expect(existsSync(join(paths.configDir, "hooks.json"))).toBe(false);
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
