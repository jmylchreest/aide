/**
 * Tests for OpenCode hooks (skill matching flow)
 *
 * Run with: npx vitest run src/test/opencode-hooks.test.ts
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { createHooks } from "../opencode/hooks.js";
import type { OpenCodeClient } from "../opencode/types.js";

const mockClient: OpenCodeClient = {
  app: {
    log: async () => {},
  },
  session: {
    create: async () => ({ id: "s-1" }),
    prompt: async () => ({}),
  },
  event: {
    subscribe: async () => ({ stream: [] as any }),
  },
};

describe("OpenCode hooks skill matching", () => {
  let projectDir: string;
  let tempHome: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-opencode-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-opencode-home-"));
    process.env.HOME = tempHome;
    mkdirSync(join(projectDir, ".aide", "skills"), { recursive: true });
    writeFileSync(
      join(projectDir, ".aide", "skills", "deploy.md"),
      `---
name: Deploy Skill
triggers:
  - deploy
---

Deploy instructions.
`,
    );
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("matches skills from message.part.updated and injects in system transform", async () => {
    const hooks = await createHooks(projectDir, projectDir, mockClient);

    await hooks.event?.({
      event: {
        type: "message.part.updated",
        properties: {
          part: {
            id: "p-1",
            sessionID: "s-1",
            messageID: "m-1",
            type: "text",
            text: "please deploy this",
          },
        },
      },
    });

    const output = { system: [] as string[] };
    await hooks["experimental.chat.system.transform"]?.(
      { sessionID: "s-1", model: { providerID: "x", modelID: "y" } },
      output,
    );

    const combined = output.system.join("\n");
    expect(combined).toContain("<aide-skills>");
    expect(combined).toContain("Deploy Skill");
  });

  it("config handler registers commands with template that avoids native skill tool", async () => {
    const hooks = await createHooks(projectDir, projectDir, mockClient);

    const config = { command: {} } as Record<string, unknown>;
    await (hooks as any).config?.(config);

    const commands = config.command as Record<
      string,
      { template: string; description: string }
    >;
    expect(commands["aide:Deploy Skill"]).toBeDefined();
    // Template must NOT contain "Activate" which previously triggered native skill tool
    expect(commands["aide:Deploy Skill"].template).not.toContain("Activate");
    // Template must instruct model NOT to use the skill tool
    expect(commands["aide:Deploy Skill"].template).toContain(
      "Do NOT use the skill tool",
    );
  });

  it("config handler registers native skill paths as fallback", async () => {
    const hooks = await createHooks(projectDir, projectDir, mockClient);

    const config = { command: {} } as Record<string, unknown>;
    await (hooks as any).config?.(config);

    const skills = config.skills as { paths?: string[] };
    expect(skills).toBeDefined();
    expect(skills.paths).toBeInstanceOf(Array);
    // Should include the .aide/skills and skills directories
    expect(skills.paths!.some((p: string) => p.endsWith(".aide/skills"))).toBe(
      true,
    );
    expect(
      skills.paths!.some(
        (p: string) => p.endsWith("/skills") && !p.includes(".aide"),
      ),
    ).toBe(true);
  });

  it("command handler injects skill into output parts and pending context", async () => {
    const hooks = await createHooks(projectDir, projectDir, mockClient);

    const parts: Array<{ type: string; text: string }> = [];
    await hooks["command.execute.before"]?.(
      { command: "aide:Deploy Skill", sessionID: "s-1", arguments: "to prod" },
      { parts },
    );

    // Should inject skill content into output parts
    expect(parts.length).toBeGreaterThan(0);
    expect(parts[0].text).toContain("Deploy instructions");
    expect(parts[0].text).toContain("<aide-instructions>");

    // System transform should also have the pending context
    const output = { system: [] as string[] };
    await hooks["experimental.chat.system.transform"]?.(
      { sessionID: "s-1", model: { providerID: "x", modelID: "y" } },
      output,
    );
    const combined = output.system.join("\n");
    expect(combined).toContain("Deploy instructions");
  });
});
