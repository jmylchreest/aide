/**
 * Tests for version-pinned OpenCode config.
 *
 * The regression these pin down: OpenCode caches a plugin install under a
 * directory keyed by the spec string and returns early if it exists, and
 * `bunx <name>` reuses its cached temp install without a registry check.
 * An unpinned entry is therefore installed once and frozen forever — the
 * exact version written into config is what makes an upgrade happen.
 *
 * Run with: npx vitest run src/test/config-pin.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  addAideToConfig,
  configuredPluginVersion,
  isAideConfigured,
  isAidePluginEntry,
  isMcpCommandCurrent,
  mcpCommand,
  pluginSpec,
  removeAideFromConfig,
  PLUGIN_NAME,
  type OpenCodeConfig,
} from "../cli/config.js";

describe("pluginSpec", () => {
  it("pins the exact version", () => {
    expect(pluginSpec("0.1.13")).toBe(`${PLUGIN_NAME}@0.1.13`);
  });

  it("stays unpinned for unversioned dev checkouts", () => {
    expect(pluginSpec(null)).toBe(PLUGIN_NAME);
    expect(pluginSpec("0.0.0")).toBe(PLUGIN_NAME);
  });
});

describe("isAidePluginEntry", () => {
  it("matches bare and pinned entries", () => {
    expect(isAidePluginEntry(PLUGIN_NAME)).toBe(true);
    expect(isAidePluginEntry(`${PLUGIN_NAME}@0.1.13`)).toBe(true);
    expect(isAidePluginEntry(`${PLUGIN_NAME}@latest`)).toBe(true);
  });

  it("does not match other packages", () => {
    expect(isAidePluginEntry("@jmylchreest/aide-plugin-extra")).toBe(false);
    expect(isAidePluginEntry("opencode-anthropic-auth")).toBe(false);
  });
});

describe("addAideToConfig", () => {
  it("writes a pinned plugin entry and MCP command", () => {
    const result = addAideToConfig({}, { version: "0.1.13" });
    expect(result.plugin).toEqual([`${PLUGIN_NAME}@0.1.13`]);
    expect(result.mcp?.aide?.command).toEqual([
      "bunx",
      "-y",
      `${PLUGIN_NAME}@0.1.13`,
      "mcp",
    ]);
  });

  it("upgrades an unpinned entry in place rather than duplicating it", () => {
    const existing: OpenCodeConfig = {
      plugin: ["opencode-anthropic-auth", PLUGIN_NAME],
      mcp: { aide: { type: "local", command: ["bunx", "-y", PLUGIN_NAME, "mcp"] } },
    };
    const result = addAideToConfig(existing, { version: "0.1.13" });
    expect(result.plugin).toEqual([
      "opencode-anthropic-auth",
      `${PLUGIN_NAME}@0.1.13`,
    ]);
    expect(result.mcp?.aide?.command?.[2]).toBe(`${PLUGIN_NAME}@0.1.13`);
  });

  it("re-pins an older pin", () => {
    const existing: OpenCodeConfig = { plugin: [`${PLUGIN_NAME}@0.0.60`] };
    const result = addAideToConfig(existing, { version: "0.1.13" });
    expect(result.plugin).toEqual([`${PLUGIN_NAME}@0.1.13`]);
  });

  it("preserves user customisation of the MCP entry", () => {
    const existing: OpenCodeConfig = {
      mcp: {
        aide: {
          type: "local",
          command: ["bunx", "-y", PLUGIN_NAME, "mcp"],
          environment: { AIDE_CODE_WATCH: "1" },
          enabled: false,
        },
      },
    };
    const result = addAideToConfig(existing, { version: "0.1.13" });
    expect(result.mcp?.aide?.environment).toEqual({ AIDE_CODE_WATCH: "1" });
    expect(result.mcp?.aide?.enabled).toBe(false);
    expect(result.mcp?.aide?.command).toEqual(mcpCommand("0.1.13"));
  });

  it("leaves other config keys alone", () => {
    const result = addAideToConfig(
      { theme: "tokyonight", plugin: ["other"] } as OpenCodeConfig,
      { version: "0.1.13" },
    );
    expect(result.theme).toBe("tokyonight");
    expect(result.plugin).toContain("other");
  });

  it("skips the MCP entry with --no-mcp", () => {
    const result = addAideToConfig({}, { version: "0.1.13", noMcp: true });
    expect(result.mcp).toBeUndefined();
  });
});

describe("configuredPluginVersion", () => {
  it("reports the pin, the unpinned case, and absence distinctly", () => {
    expect(configuredPluginVersion({ plugin: [`${PLUGIN_NAME}@0.1.13`] })).toBe("0.1.13");
    expect(configuredPluginVersion({ plugin: [PLUGIN_NAME] })).toBe("");
    expect(configuredPluginVersion({ plugin: ["other"] })).toBeNull();
    expect(configuredPluginVersion({})).toBeNull();
  });
});

describe("isMcpCommandCurrent", () => {
  const pinned: OpenCodeConfig = {
    mcp: { aide: { command: mcpCommand("0.1.13") } },
  };

  it("accepts the command for the wanted version", () => {
    expect(isMcpCommandCurrent(pinned, "0.1.13")).toBe(true);
  });

  it("rejects a command pinned to another version", () => {
    expect(isMcpCommandCurrent(pinned, "0.1.14")).toBe(false);
  });

  it("rejects legacy and unpinned command shapes", () => {
    expect(
      isMcpCommandCurrent(
        { mcp: { aide: { command: ["bunx", "-y", PLUGIN_NAME, "mcp"] } } },
        "0.1.13",
      ),
    ).toBe(false);
    expect(
      isMcpCommandCurrent(
        { mcp: { aide: { command: ["bun", "/old/path/aide-wrapper.ts", "mcp"] } } },
        "0.1.13",
      ),
    ).toBe(false);
    expect(isMcpCommandCurrent({}, "0.1.13")).toBe(false);
  });
});

describe("isAideConfigured / removeAideFromConfig", () => {
  it("detects a pinned plugin entry", () => {
    const config = addAideToConfig({}, { version: "0.1.13" });
    expect(isAideConfigured(config)).toEqual({ plugin: true, mcp: true });
  });

  it("removes pinned entries too", () => {
    const config = addAideToConfig({ plugin: ["other"] }, { version: "0.1.13" });
    const cleaned = removeAideFromConfig(config);
    expect(cleaned.plugin).toEqual(["other"]);
    expect(cleaned.mcp).toBeUndefined();
  });
});
