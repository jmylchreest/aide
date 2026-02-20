/**
 * Tests for MCP sync behavior
 *
 * Run with: npx vitest run src/test/mcp-sync.test.ts
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

function readJson(path: string): any {
  return JSON.parse(readFileSync(path, "utf-8"));
}

describe("syncMcpServers", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-mcp-project-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-mcp-home-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("writes empty assistant config when servers removed", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    // Existing assistant config with a server
    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify(
        {
          mcpServers: {
            test: { type: "stdio", command: "npx", args: ["foo"] },
          },
        },
        null,
        2,
      ) + "\n",
    );

    // Journal entry with previous server list so removal is detected
    const journalDir = join(projectDir, ".aide", "config");
    mkdirSync(journalDir, { recursive: true });
    writeFileSync(
      join(journalDir, "mcp-sync.journal.json"),
      JSON.stringify(
        {
          entries: [
            {
              ts: new Date().toISOString(),
              servers: ["test"],
            },
          ],
          removed: [],
        },
        null,
        2,
      ) + "\n",
    );

    // Canonical config with no servers
    writeFileSync(
      join(journalDir, "mcp.json"),
      JSON.stringify({ mcpServers: {} }, null, 2) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const updatedClaude = readJson(join(projectDir, ".mcp.json"));
    expect(updatedClaude.mcpServers).toEqual({});
  });
});
