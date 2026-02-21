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
  existsSync,
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

  it("writes user scope to ~/.claude.json only", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    writeFileSync(
      join(tempHome, ".claude.json"),
      JSON.stringify(
        {
          mcpServers: {
            remoteSearch: {
              type: "http",
              url: "https://example.com/mcp",
              headers: { "X-API-Key": "test-api-key" },
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const updatedUser = readJson(join(tempHome, ".claude.json"));
    expect(updatedUser.mcpServers).toEqual({
      remoteSearch: {
        type: "http",
        url: "https://example.com/mcp",
        headers: { "X-API-Key": "test-api-key" },
      },
    });
    expect(existsSync(join(tempHome, ".mcp.json"))).toBe(false);
    expect(existsSync(join(projectDir, ".mcp.json"))).toBe(false);
  });

  it("keeps user and project scopes isolated", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    writeFileSync(
      join(tempHome, ".claude.json"),
      JSON.stringify(
        {
          mcpServers: {
            remoteSearch: {
              type: "http",
              url: "https://example.com/mcp",
              headers: { "X-API-Key": "test-api-key" },
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify(
        {
          mcpServers: {
            projectOnly: { type: "stdio", command: "npx", args: ["proj"] },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const updatedUser = readJson(join(tempHome, ".claude.json"));
    const updatedProject = readJson(join(projectDir, ".mcp.json"));

    expect(Object.keys(updatedUser.mcpServers).sort()).toEqual([
      "remoteSearch",
    ]);
    expect(Object.keys(updatedProject.mcpServers).sort()).toEqual([
      "projectOnly",
    ]);
  });

  it("maps OpenCode local/remote to Claude stdio/http", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    writeFileSync(
      join(projectDir, "opencode.json"),
      JSON.stringify(
        {
          mcp: {
            localTool: {
              type: "local",
              command: ["bunx", "-y", "local-tool"],
              environment: { LOCAL_ENV: "1" },
              enabled: true,
            },
            remoteSearch: {
              type: "remote",
              url: "https://example.com/mcp",
              headers: { "X-API-Key": "test-api-key" },
              enabled: true,
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const updatedClaude = readJson(join(projectDir, ".mcp.json"));
    expect(updatedClaude.mcpServers).toEqual({
      localTool: {
        type: "stdio",
        command: "bunx",
        args: ["-y", "local-tool"],
        env: { LOCAL_ENV: "1" },
      },
      remoteSearch: {
        type: "http",
        url: "https://example.com/mcp",
        headers: { "X-API-Key": "test-api-key" },
      },
    });
  });

  it("preserves sse transport in canonical config", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify(
        {
          mcpServers: {
            sseTool: {
              type: "sse",
              url: "https://sse.example.com/mcp",
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("opencode", projectDir);

    const canonical = readJson(join(projectDir, ".aide", "config", "mcp.json"));
    expect(canonical.mcpServers.sseTool.transport).toBe("sse");
  });
});
