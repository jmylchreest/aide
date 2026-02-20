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
});
