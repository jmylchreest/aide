/**
 * Tests for session-scoped state: getScopedState reads, updateToolStats
 * scoping, and the HUD's JSON-based state parsing.
 *
 * Uses a stub `aide` binary (bun script emulating `state get/set/delete/list`
 * against a JSON file) so the TS logic is tested without the Go binary.
 * Mode is deliberately NOT session-scoped (global control-plane key — see
 * the note in core/aide-client.ts); these tests cover the counter keys.
 *
 * Run with: npx vitest run src/test/scoped-state.test.ts
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  mkdtempSync,
  rmSync,
  writeFileSync,
  readFileSync,
  chmodSync,
  existsSync,
  mkdirSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { getScopedState, setState } from "../core/aide-client.js";
import { updateToolStats } from "../core/tool-tracking.js";

const STUB = `#!/usr/bin/env bun
// Stub aide binary: emulates \`state get/set/delete/list\` against a JSON file.
const file = process.env.AIDE_STUB_STATE_FILE;
const load = () => {
  try { return JSON.parse(require("fs").readFileSync(file, "utf-8")); }
  catch { return {}; }
};
const save = (s) => require("fs").writeFileSync(file, JSON.stringify(s));
const args = process.argv.slice(2);
if (args[0] !== "state") process.exit(0);
const sub = args[1];
const rest = args.slice(2);
const agent = (rest.find((a) => a.startsWith("--agent=")) || "").slice(8);
const positional = rest.filter((a) => !a.startsWith("--"));
const key = positional[0];
const fullKey = agent ? \`agent:\${agent}:\${key}\` : key;
const state = load();
if (sub === "get") {
  if (fullKey in state) {
    console.log(agent ? \`[\${agent}] \${key} = \${state[fullKey]}\` : \`\${key} = \${state[fullKey]}\`);
  } else {
    console.log("No state found for key:", key);
  }
} else if (sub === "set") {
  state[fullKey] = positional[1] ?? "";
  save(state);
} else if (sub === "delete") {
  // Mirrors the Go CLI: the TS client passes the FULL key as positional.
  delete state[key];
  save(state);
} else if (sub === "list") {
  // Mirrors \`aide state list --json\`: array of {key, value, agent?}.
  const rows = Object.entries(state).map(([k, v]) => {
    const m = k.match(/^agent:([^:]+):/);
    return m ? { key: k, value: v, agent: m[1] } : { key: k, value: v };
  });
  if (rest.includes("--json") || rest.includes("--format=json")) {
    console.log(JSON.stringify(rows));
  } else {
    // Tabwriter-style table — deliberately NOT parseable as key=value,
    // matching the real CLI so parsers that regex '=' stay caught.
    console.log("AGENT  KEY  VALUE");
    for (const r of rows) console.log(\`\${r.agent || ""}  \${r.key}  \${r.value}\`);
  }
}
`;

describe("session-scoped state", () => {
  let tmp: string;
  let binary: string;
  let stateFile: string;

  const readStore = (): Record<string, string> =>
    existsSync(stateFile)
      ? (JSON.parse(readFileSync(stateFile, "utf-8")) as Record<string, string>)
      : {};

  beforeEach(() => {
    tmp = mkdtempSync(join(tmpdir(), "aide-scoped-"));
    // Plant the stub where findAideBinary looks first (pluginRoot/bin/aide)
    // so hud.ts's runAide(cwd, ...) also resolves it.
    mkdirSync(join(tmp, "bin"), { recursive: true });
    binary = join(tmp, "bin", "aide");
    stateFile = join(tmp, "state.json");
    writeFileSync(binary, STUB);
    chmodSync(binary, 0o755);
    process.env.AIDE_STUB_STATE_FILE = stateFile;
    process.env.AIDE_PLUGIN_ROOT = tmp;
  });

  afterEach(() => {
    rmSync(tmp, { recursive: true, force: true });
    delete process.env.AIDE_STUB_STATE_FILE;
    delete process.env.AIDE_PLUGIN_ROOT;
  });

  it("getScopedState prefers the session-scoped key over the global", () => {
    setState(binary, tmp, "startedAt", "global-ts");
    setState(binary, tmp, "startedAt", "scoped-ts", "s1");

    expect(getScopedState(binary, tmp, "s1", "startedAt")).toBe("scoped-ts");
    expect(getScopedState(binary, tmp, "s2", "startedAt")).toBe("global-ts");
    expect(getScopedState(binary, tmp, undefined, "startedAt")).toBe(
      "global-ts",
    );
  });

  it("getScopedState returns null when neither spelling exists", () => {
    expect(getScopedState(binary, tmp, "s1", "missing")).toBeNull();
  });

  it("updateToolStats writes counters session-scoped, not globally", () => {
    updateToolStats(binary, tmp, "Bash", "s1");
    updateToolStats(binary, tmp, "Edit", "s1");

    const store = readStore();
    expect(store["agent:s1:toolCalls"]).toBe("2");
    expect(store["agent:s1:lastTool"]).toBe("Edit");
    expect(store["agent:s1:startedAt"]).toBeTruthy();
    expect(store["toolCalls"]).toBeUndefined();
    expect(store["lastTool"]).toBeUndefined();
    expect(store["startedAt"]).toBeUndefined();
  });

  it("updateToolStats tracks concurrent sessions independently", () => {
    updateToolStats(binary, tmp, "Bash", "s1");
    updateToolStats(binary, tmp, "Read", "s2");
    updateToolStats(binary, tmp, "Edit", "s2");

    const store = readStore();
    expect(store["agent:s1:toolCalls"]).toBe("1");
    expect(store["agent:s2:toolCalls"]).toBe("2");
  });

  it("updateToolStats clears currentTool for the session's own scope", () => {
    // trackToolUse sets currentTool for agentId (incl. the session itself);
    // updateToolStats must clear the same scope on PostToolUse.
    setState(binary, tmp, "currentTool", "Bash(sleep 1)", "s1");
    updateToolStats(binary, tmp, "Bash", "s1", "s1");

    const store = readStore();
    expect(store["agent:s1:currentTool"]).toBe("");
  });

  it("updateToolStats records per-agent lastTool for real subagents", () => {
    updateToolStats(binary, tmp, "Bash", "s1", "sub-agent-9");
    const store = readStore();
    expect(store["agent:sub-agent-9:lastTool"]).toBe("Bash");
    expect(store["agent:sub-agent-9:currentTool"]).toBe("");
  });

  it("clears currentTool in the OpenCode call shape (sessionID as both scopes)", () => {
    // Regression guard: opencode/hooks.ts tool.execute.before registers
    // currentTool under agentId=input.sessionID; tool.execute.after must
    // pass input.sessionID as BOTH sessionId and agentId or the clear
    // targets nothing and the tool shows as forever-running.
    setState(binary, tmp, "currentTool", "Bash(sleep 1)", "oc-sess");
    updateToolStats(binary, tmp, "Bash", "oc-sess", "oc-sess");

    const store = readStore();
    expect(store["agent:oc-sess:currentTool"]).toBe("");
    expect(store["agent:oc-sess:toolCalls"]).toBe("1");
  });

  it("hud getSessionState reads scoped counters with global fallback via --json", async () => {
    const { getSessionState } = await import("../lib/hud.js");
    setState(binary, tmp, "mode", "autopilot"); // global (by design)
    setState(binary, tmp, "toolCalls", "42", "s1");
    setState(binary, tmp, "startedAt", "2026-07-20T10:00:00Z", "s1");
    setState(binary, tmp, "toolCalls", "7", "s2");

    const s1 = getSessionState(tmp, "s1");
    expect(s1.activeMode).toBe("autopilot");
    expect(s1.toolCalls).toBe(42);
    expect(s1.startedAt).toBe("2026-07-20T10:00:00Z");

    const s2 = getSessionState(tmp, "s2");
    expect(s2.toolCalls).toBe(7);
    expect(s2.startedAt).toBeNull();
  });

  it("hud getAgentStates parses --json rows and keeps sessions distinguishable", async () => {
    const { getAgentStates } = await import("../lib/hud.js");
    // A real subagent (has session linkage) and a session pseudo-row.
    setState(binary, tmp, "status", "running", "agent-1");
    setState(binary, tmp, "session", "s1", "agent-1");
    setState(binary, tmp, "toolCalls", "3", "s1");

    const agents = getAgentStates(tmp);
    const real = agents.find((a) => a.agentId === "agent-1");
    const pseudo = agents.find((a) => a.agentId === "s1");
    expect(real?.status).toBe("running");
    expect(real?.session).toBe("s1");
    // The session's own row exists but has no session linkage, so
    // refreshHud's `a.session === sessionId` filter excludes it.
    expect(pseudo?.session).toBeNull();
  });
});
