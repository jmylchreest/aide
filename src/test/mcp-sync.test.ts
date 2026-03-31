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
  utimesSync,
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

    // v2 journal: "test" was previously present in .mcp.json, now the aide
    // canonical config no longer has it — simulating the user removing it
    // from the canonical config (which has a newer mtime)
    const journalDir = join(projectDir, ".aide", "config");
    mkdirSync(journalDir, { recursive: true });

    const mcpJsonPath = join(projectDir, ".mcp.json");
    const aideMcpPath = join(journalDir, "mcp.json");

    // Canonical config with no servers (written after .mcp.json → newer mtime)
    writeFileSync(
      aideMcpPath,
      JSON.stringify({ mcpServers: {} }, null, 2) + "\n",
    );

    const oldMtime = Date.now() - 60_000;
    const newMtime = Date.now();

    // Set file mtimes so updatePresence uses the expected timestamps.
    // .mcp.json is old (server was present a while ago), aide config is new.
    const oldDate = new Date(oldMtime);
    const newDate = new Date(newMtime);
    utimesSync(mcpJsonPath, oldDate, oldDate);
    utimesSync(aideMcpPath, newDate, newDate);

    writeFileSync(
      join(journalDir, "mcp-sync.journal.json"),
      JSON.stringify(
        {
          version: 2,
          servers: {
            test: {
              [mcpJsonPath]: {
                present: true,
                mtime: oldMtime,
                ts: new Date(oldMtime).toISOString(),
              },
              [aideMcpPath]: {
                present: false,
                mtime: newMtime,
                ts: new Date(newMtime).toISOString(),
              },
            },
          },
        },
        null,
        2,
      ) + "\n",
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

  it("deletion from one source sticks when other sources have older copies", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const journalDir = join(projectDir, ".aide", "config");
    mkdirSync(journalDir, { recursive: true });

    const aideMcpPath = join(journalDir, "mcp.json");
    const mcpJsonPath = join(projectDir, ".mcp.json");

    // Both project-scope sources initially had cloudflare + keeper
    // aide canonical config (old mtime — written by previous sync)
    writeFileSync(
      aideMcpPath,
      JSON.stringify(
        {
          mcpServers: {
            cloudflare: {
              type: "local",
              command: "npx",
              args: ["cloudflare-mcp"],
            },
            keeper: { type: "local", command: "npx", args: ["keeper"] },
          },
        },
        null,
        2,
      ) + "\n",
    );

    // .mcp.json: user deleted cloudflare, only keeper remains (newer mtime)
    writeFileSync(
      mcpJsonPath,
      JSON.stringify(
        {
          mcpServers: {
            keeper: { type: "stdio", command: "npx", args: ["keeper"] },
          },
        },
        null,
        2,
      ) + "\n",
    );

    // Set explicit file mtimes: aide config is old, .mcp.json is new
    const oldTime = new Date(Date.now() - 60_000);
    const newTime = new Date();
    utimesSync(aideMcpPath, oldTime, oldTime);
    utimesSync(mcpJsonPath, newTime, newTime);

    const oldMtime = oldTime.getTime();

    // Journal: cloudflare was present in both project sources at old mtime
    writeFileSync(
      join(journalDir, "mcp-sync.journal.json"),
      JSON.stringify(
        {
          version: 2,
          servers: {
            cloudflare: {
              [aideMcpPath]: {
                present: true,
                mtime: oldMtime,
                ts: oldTime.toISOString(),
              },
              [mcpJsonPath]: {
                present: true,
                mtime: oldMtime,
                ts: oldTime.toISOString(),
              },
            },
            keeper: {
              [aideMcpPath]: {
                present: true,
                mtime: oldMtime,
                ts: oldTime.toISOString(),
              },
              [mcpJsonPath]: {
                present: true,
                mtime: oldMtime,
                ts: oldTime.toISOString(),
              },
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    // cloudflare should be gone (removal from .mcp.json is newer than
    // presence in aide config), keeper should remain
    const updatedProject = readJson(mcpJsonPath);
    expect(Object.keys(updatedProject.mcpServers)).toEqual(["keeper"]);

    const updatedCanonical = readJson(aideMcpPath);
    expect(Object.keys(updatedCanonical.mcpServers)).toEqual(["keeper"]);
  });

  it("re-adding a server after deletion brings it back", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const journalDir = join(projectDir, ".aide", "config");
    mkdirSync(journalDir, { recursive: true });

    const aideMcpPath = join(journalDir, "mcp.json");
    writeFileSync(
      aideMcpPath,
      JSON.stringify({ mcpServers: {} }, null, 2) + "\n",
    );

    // User re-adds cloudflare to .mcp.json (newest file)
    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify(
        {
          mcpServers: {
            cloudflare: {
              type: "stdio",
              command: "npx",
              args: ["cloudflare-mcp"],
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    // Journal: cloudflare was previously removed from aide config
    const removedMtime = Date.now() - 60_000;
    const mcpJsonPath = join(projectDir, ".mcp.json");

    writeFileSync(
      join(journalDir, "mcp-sync.journal.json"),
      JSON.stringify(
        {
          version: 2,
          servers: {
            cloudflare: {
              [aideMcpPath]: {
                present: false,
                mtime: removedMtime,
                ts: new Date(removedMtime).toISOString(),
              },
            },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    // cloudflare should be back — .mcp.json has a newer mtime
    const updatedProject = readJson(join(projectDir, ".mcp.json"));
    expect(updatedProject.mcpServers.cloudflare).toBeDefined();
    expect(updatedProject.mcpServers.cloudflare.command).toBe("npx");

    const updatedCanonical = readJson(aideMcpPath);
    expect(updatedCanonical.mcpServers.cloudflare).toBeDefined();
  });

  it("migrates v1 journal removed list to v2 removal entries", async () => {
    const { syncMcpServers, getRemovedServers } = await import(
      "../core/mcp-sync.js"
    );

    const journalDir = join(projectDir, ".aide", "config");
    mkdirSync(journalDir, { recursive: true });

    writeFileSync(
      join(journalDir, "mcp.json"),
      JSON.stringify({ mcpServers: {} }, null, 2) + "\n",
    );

    // v1 journal with "oldserver" in removed list
    writeFileSync(
      join(journalDir, "mcp-sync.journal.json"),
      JSON.stringify(
        {
          entries: [
            { ts: new Date().toISOString(), servers: ["oldserver"] },
          ],
          removed: ["oldserver"],
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const removed = getRemovedServers(projectDir);
    expect(removed.project).toContain("oldserver");

    // Verify journal was migrated to v2
    const journal = readJson(join(journalDir, "mcp-sync.journal.json"));
    expect(journal.version).toBe(2);
    expect(journal.servers.oldserver).toBeDefined();
  });

  it("creates v2 journal on fresh sync", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify(
        {
          mcpServers: {
            test: { type: "stdio", command: "npx", args: ["test"] },
          },
        },
        null,
        2,
      ) + "\n",
    );

    syncMcpServers("claude-code", projectDir);

    const journalPath = join(
      projectDir,
      ".aide",
      "config",
      "mcp-sync.journal.json",
    );
    expect(existsSync(journalPath)).toBe(true);
    const journal = readJson(journalPath);
    expect(journal.version).toBe(2);
    expect(journal.servers.test).toBeDefined();

    // The test server should be tracked as present in .mcp.json
    const mcpJsonPath = join(projectDir, ".mcp.json");
    const entry = journal.servers.test[mcpJsonPath];
    expect(entry).toBeDefined();
    expect(entry.present).toBe(true);
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
