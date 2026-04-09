/**
 * Tests for Codex CLI integration.
 *
 * Covers: TOML config reader/writer in mcp-sync, hook input normalization,
 * and Codex config generator.
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

describe("Codex TOML MCP sync", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-codex-project-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-codex-home-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("reads Codex TOML config and syncs to canonical format", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const codexDir = join(projectDir, ".codex");
    mkdirSync(codexDir, { recursive: true });

    writeFileSync(
      join(codexDir, "config.toml"),
      `[mcp_servers.localTool]
command = "npx"
args = ["some-tool"]

[mcp_servers.localTool.env]
TOOL_KEY = "abc"
`,
    );

    syncMcpServers("codex", projectDir);

    const canonical = readJson(
      join(projectDir, ".aide", "config", "mcp.json"),
    );
    expect(canonical.mcpServers.localTool).toBeDefined();
    expect(canonical.mcpServers.localTool.type).toBe("local");
    expect(canonical.mcpServers.localTool.command).toBe("npx");
    expect(canonical.mcpServers.localTool.args).toEqual(["some-tool"]);
    expect(canonical.mcpServers.localTool.env).toEqual({ TOOL_KEY: "abc" });
  });

  it("writes Codex TOML config preserving non-MCP content", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const codexDir = join(projectDir, ".codex");
    mkdirSync(codexDir, { recursive: true });

    // Existing config with model settings
    writeFileSync(
      join(codexDir, "config.toml"),
      `model = "gpt-5.4"
model_provider = "openai"
`,
    );

    // Add a server via Claude Code config (will sync to Codex)
    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify({
        mcpServers: {
          myTool: {
            type: "stdio",
            command: "npx",
            args: ["my-tool"],
            env: {},
          },
        },
      }) + "\n",
    );

    syncMcpServers("codex", projectDir);

    const tomlContent = readFileSync(
      join(codexDir, "config.toml"),
      "utf-8",
    );

    // Non-MCP content preserved
    expect(tomlContent).toContain('model = "gpt-5.4"');
    expect(tomlContent).toContain('model_provider = "openai"');

    // MCP server written
    expect(tomlContent).toContain("[mcp_servers.myTool]");
    expect(tomlContent).toContain('command = "npx"');
  });

  it("syncs servers from Codex TOML to Claude Code JSON", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const codexDir = join(projectDir, ".codex");
    mkdirSync(codexDir, { recursive: true });

    writeFileSync(
      join(codexDir, "config.toml"),
      `[mcp_servers.codexTool]
command = "bun"
args = ["run", "tool"]
`,
    );

    // Sync as Claude Code — should import from Codex
    syncMcpServers("claude-code", projectDir);

    const claudeConfig = readJson(join(projectDir, ".mcp.json"));
    expect(claudeConfig.mcpServers.codexTool).toBeDefined();
    expect(claudeConfig.mcpServers.codexTool.command).toBe("bun");
    expect(claudeConfig.mcpServers.codexTool.args).toEqual(["run", "tool"]);
  });

  it("handles remote Codex MCP servers with URL", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    const codexDir = join(projectDir, ".codex");
    mkdirSync(codexDir, { recursive: true });

    writeFileSync(
      join(codexDir, "config.toml"),
      `[mcp_servers.remoteTool]
url = "https://mcp.example.com/sse"

[mcp_servers.remoteTool.headers]
Authorization = "Bearer test-token"
`,
    );

    syncMcpServers("codex", projectDir);

    const canonical = readJson(
      join(projectDir, ".aide", "config", "mcp.json"),
    );
    expect(canonical.mcpServers.remoteTool).toBeDefined();
    expect(canonical.mcpServers.remoteTool.type).toBe("remote");
    expect(canonical.mcpServers.remoteTool.url).toBe(
      "https://mcp.example.com/sse",
    );
    expect(canonical.mcpServers.remoteTool.headers).toEqual({
      Authorization: "Bearer test-token",
    });
  });

  it("includes codex platform in gatherSources", async () => {
    const { syncMcpServers } = await import("../core/mcp-sync.js");

    // Create configs for multiple platforms
    writeFileSync(
      join(projectDir, ".mcp.json"),
      JSON.stringify({
        mcpServers: {
          claudeTool: {
            type: "stdio",
            command: "npx",
            args: ["claude-tool"],
            env: {},
          },
        },
      }) + "\n",
    );

    const codexDir = join(projectDir, ".codex");
    mkdirSync(codexDir, { recursive: true });
    writeFileSync(
      join(codexDir, "config.toml"),
      `[mcp_servers.codexTool]
command = "npx"
args = ["codex-tool"]
`,
    );

    // Sync as codex — should see both sources
    syncMcpServers("codex", projectDir);

    const toml = readFileSync(join(codexDir, "config.toml"), "utf-8");
    expect(toml).toContain("[mcp_servers.claudeTool]");
    expect(toml).toContain("[mcp_servers.codexTool]");
  });
});

describe("normalizeHookInput", () => {
  it("maps camelCase to snake_case", async () => {
    const { normalizeHookInput } = await import("../lib/hook-utils.js");

    const input = JSON.stringify({
      hookEventName: "PreToolUse",
      sessionId: "abc123",
      toolName: "Bash",
      agentId: "agent-1",
    });

    const result = JSON.parse(normalizeHookInput(input));
    expect(result.hook_event_name).toBe("PreToolUse");
    expect(result.session_id).toBe("abc123");
    expect(result.tool_name).toBe("Bash");
    expect(result.agent_id).toBe("agent-1");

    // camelCase keys removed
    expect(result.hookEventName).toBeUndefined();
    expect(result.sessionId).toBeUndefined();
  });

  it("preserves existing snake_case fields", async () => {
    const { normalizeHookInput } = await import("../lib/hook-utils.js");

    const input = JSON.stringify({
      hook_event_name: "SessionStart",
      session_id: "xyz",
      cwd: "/tmp",
    });

    const result = normalizeHookInput(input);
    // Should return original string unchanged (no normalization needed)
    expect(result).toBe(input);
  });

  it("does not overwrite existing snake_case with camelCase", async () => {
    const { normalizeHookInput } = await import("../lib/hook-utils.js");

    const input = JSON.stringify({
      hook_event_name: "correct",
      hookEventName: "wrong",
      session_id: "real-id",
    });

    const result = JSON.parse(normalizeHookInput(input));
    expect(result.hook_event_name).toBe("correct");
  });

  it("handles invalid JSON gracefully", async () => {
    const { normalizeHookInput } = await import("../lib/hook-utils.js");

    const result = normalizeHookInput("not json");
    expect(result).toBe("not json");
  });

  it("handles empty input", async () => {
    const { normalizeHookInput } = await import("../lib/hook-utils.js");

    const result = normalizeHookInput("{}");
    expect(result).toBe("{}");
  });
});

describe("Codex config generator", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-codex-gen-"));
    tempHome = mkdtempSync(join(tmpdir(), "aide-codex-gen-home-"));
    vi.resetModules();
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
    rmSync(tempHome, { recursive: true, force: true });
  });

  it("installs MCP config and hooks for Codex", async () => {
    const { installCodex, isCodexConfigured, getCodexConfigTomlPath, getCodexHooksJsonPath } =
      await import("../cli/codex-config.js");

    const result = installCodex("user");

    expect(result.configWritten).toBe(true);
    expect(result.hooksWritten).toBe(true);

    const status = isCodexConfigured("user");
    expect(status.mcp).toBe(true);
    expect(status.hooks).toBe(true);

    // Verify TOML config has aide MCP server (command varies: "aide-plugin" if global, "bun" if local dev)
    const toml = readFileSync(getCodexConfigTomlPath("user"), "utf-8");
    expect(toml).toContain("[mcp_servers.aide]");
    expect(toml).toMatch(/command = "(aide-plugin|bun)"/);

    // Verify hooks.json
    const hooks = readJson(getCodexHooksJsonPath("user"));
    expect(hooks.hooks.SessionStart).toBeDefined();
    expect(hooks.hooks.PreToolUse).toBeDefined();
    expect(hooks.hooks.PostToolUse).toBeDefined();
    expect(hooks.hooks.UserPromptSubmit).toBeDefined();
    expect(hooks.hooks.Stop).toBeDefined();

    // Verify Stop includes session-end
    const stopHooks = hooks.hooks.Stop[0].hooks;
    const hasSessionEnd = stopHooks.some(
      (h: any) => h.command.includes("session-end"),
    );
    expect(hasSessionEnd).toBe(true);
  });

  it("does not overwrite existing config on reinstall", async () => {
    const { installCodex, isCodexConfigured } =
      await import("../cli/codex-config.js");

    const first = installCodex("user");
    expect(first.configWritten).toBe(true);
    expect(first.hooksWritten).toBe(true);

    const second = installCodex("user");
    expect(second.configWritten).toBe(false);
    expect(second.hooksWritten).toBe(false);

    const status = isCodexConfigured("user");
    expect(status.mcp).toBe(true);
    expect(status.hooks).toBe(true);
  });

  it("uninstalls cleanly", async () => {
    const { installCodex, uninstallCodex, isCodexConfigured } =
      await import("../cli/codex-config.js");

    installCodex("user");
    const result = uninstallCodex("user");

    expect(result.configRemoved).toBe(true);
    expect(result.hooksRemoved).toBe(true);

    const status = isCodexConfigured("user");
    expect(status.mcp).toBe(false);
    expect(status.hooks).toBe(false);
  });

  it("merges hooks into existing hooks.json", async () => {
    const { installCodex, getCodexHooksJsonPath } =
      await import("../cli/codex-config.js");

    // Pre-existing hooks.json with custom hook
    const hooksDir = join(tempHome, ".codex");
    mkdirSync(hooksDir, { recursive: true });
    writeFileSync(
      join(hooksDir, "hooks.json"),
      JSON.stringify({
        hooks: {
          SessionStart: [
            {
              matcher: "*",
              hooks: [
                {
                  type: "command",
                  command: "python3 custom-hook.py",
                },
              ],
            },
          ],
        },
      }) + "\n",
    );

    installCodex("user");

    const hooks = readJson(getCodexHooksJsonPath("user"));
    // Should have both custom and aide SessionStart hooks
    expect(hooks.hooks.SessionStart.length).toBe(2);
    expect(hooks.hooks.SessionStart[0].hooks[0].command).toBe(
      "python3 custom-hook.py",
    );
    expect(hooks.hooks.SessionStart[1].hooks[0].command).toContain(
      "session-start",
    );
  });
});
