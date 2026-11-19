import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { generateHooksJson } from "../cli/codex-config.js";

describe("host hook execution policy", () => {
  const claude = JSON.parse(
    readFileSync(
      new URL("../../.claude-plugin/plugin.json", import.meta.url),
      "utf8",
    ),
  );
  for (const [host, config] of [
    ["claude", claude],
    ["codex", generateHooksJson("aide-plugin hook")],
  ] as const) {
    it(`${host} keeps observation work in the background and control decisions synchronous`, () => {
      const hooks = Object.values(config.hooks).flatMap((groups: any) =>
        groups.flatMap((group: any) => group.hooks),
      );
      for (const name of [
        "tool-tracker",
        "tool-observe",
        "hud-updater",
        "usage-catchup",
        "session-summary",
      ]) {
        const entries = hooks.filter((hook: any) =>
          hook.command.includes(name),
        );
        expect(entries.length).toBeGreaterThan(0);
        for (const entry of entries) expect(entry.async).toBe(true);
      }
      for (const name of [
        "skill-injector",
        "write-guard",
        "pre-tool-enforcer",
        "context-guard",
        "persistence",
        "subagent-tracker",
      ]) {
        const entries = hooks.filter((hook: any) =>
          hook.command.includes(name),
        );
        expect(entries.length).toBeGreaterThan(0);
        for (const entry of entries) expect(entry.async).not.toBe(true);
      }
    });
  }
});
