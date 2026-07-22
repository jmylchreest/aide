import { describe, expect, it } from "vitest";
import {
  composeStatusline,
  parsePayload,
  type StatuslineData,
  type StatuslinePayload,
} from "../lib/statusline.js";
import type { AgentState } from "../lib/hud.js";

// These goldens ARE the spec: each scenario pins the exact line the
// statusline renders for a representative session state.

const AGO_2M = new Date(Date.now() - 2 * 60_000).toISOString();
const AGO_45S = new Date(Date.now() - 45_000).toISOString();
const AGO_4M = new Date(Date.now() - 4 * 60_000).toISOString();

function base(over: Partial<StatuslineData> = {}): StatuslineData {
  return {
    version: "0.1.8",
    projectName: null,
    parentName: null,
    state: {
      activeMode: null,
      agentCount: 0,
      startedAt: AGO_2M,
      toolCalls: 0,
      lastTool: null,
    },
    currentTool: null,
    lastToolUse: null,
    modeIterations: null,
    agents: [],
    ...over,
  };
}

function payload(over: Partial<StatuslinePayload> = {}): StatuslinePayload {
  return { modelName: "Fable 5", contextPercent: 12, costUSD: null, ...over };
}

function agent(over: Partial<AgentState>): AgentState {
  return {
    agentId: "a1b2c3d4e5",
    mode: null,
    startedAt: AGO_4M,
    currentTool: null,
    tasksCompleted: 0,
    tasksTotal: 0,
    status: "running",
    type: null,
    task: null,
    skill: null,
    session: null,
    ...over,
  };
}

describe("composeStatusline scenarios", () => {
  it("quiet session: model, context, idle age", () => {
    expect(
      composeStatusline(payload(), base({ lastToolUse: AGO_2M }), "full"),
    ).toBe("[aide 0.1.8] Fable 5 | ctx 12% | idle 2m");
  });

  it("active tool: live command replaces the idle marker", () => {
    const d = base({
      currentTool: "Bash(go test ./pkg/subscription -count=1)",
      state: { ...base().state, toolCalls: 127 },
    });
    expect(composeStatusline(payload({ contextPercent: 38 }), d, "full")).toBe(
      "[aide 0.1.8] Fable 5 | ctx 38% | ▸ Bash: go test ./pkg/subscriptio… | ⚒127",
    );
  });

  it("mode engaged: name and iterations lead the line", () => {
    const d = base({
      state: { ...base().state, activeMode: "autopilot", toolCalls: 203 },
      modeIterations: "3/20",
      currentTool: "Edit(cmd_session.go)",
    });
    expect(composeStatusline(payload({ contextPercent: 71 }), d, "full")).toBe(
      "[aide 0.1.8] autopilot 3/20 | Fable 5 | ctx 71%⚠ | ▸ Edit: cmd_session.go | ⚒203",
    );
  });

  it("estate: containment shown only when a parent exists", () => {
    const d = base({
      projectName: "webshop",
      parentName: "tl",
      lastToolUse: AGO_45S,
    });
    expect(composeStatusline(payload({ contextPercent: 9 }), d, "full")).toBe(
      "[aide 0.1.8] webshop⊂tl | Fable 5 | ctx 9% | idle 45s",
    );
  });

  it("swarm: agent count on the main line, one row per running agent", () => {
    const d = base({
      state: { ...base().state, activeMode: "swarm", toolCalls: 88 },
      agents: [
        agent({
          agentId: "exec-1a2b3c4",
          type: "executor",
          currentTool: "Bash(bun run test)",
        }),
        agent({ agentId: "rev-9z8y7x6", type: "reviewer", task: "review story-3 diff" }),
      ],
    });
    expect(composeStatusline(payload({ contextPercent: 44 }), d, "full")).toBe(
      [
        "[aide 0.1.8] swarm | Fable 5 | ctx 44% | idle 2m | ⚒88 | agents:2",
        "└─ ▶[exec-1a] executor | 4m | ▸ Bash: bun run test",
        "└─ ▶[rev-9z8] reviewer | 4m | review story-3 diff",
      ].join("\n"),
    );
  });

  it("context pressure escalates the marker", () => {
    expect(
      composeStatusline(payload({ contextPercent: 93 }), base(), "full"),
    ).toContain("ctx 93%‼");
  });

  it("cost appears once it rounds to a cent", () => {
    expect(
      composeStatusline(payload({ costUSD: 4.512 }), base(), "full"),
    ).toContain("$4.51");
    expect(
      composeStatusline(payload({ costUSD: 0.001 }), base(), "full"),
    ).not.toContain("$");
  });

  it("minimal: mode/estate/activity only, bare tag", () => {
    const d = base({
      projectName: "webshop",
      parentName: "tl",
      currentTool: "Read(main.go)",
      state: { ...base().state, toolCalls: 30 },
    });
    expect(composeStatusline(payload({ contextPercent: 38 }), d, "minimal")).toBe(
      "[aide] webshop⊂tl | ctx 38% | ▸ Read: main.go",
    );
  });

  it("segments config drops payload-derived parts individually", () => {
    const d = base({
      projectName: "webshop",
      parentName: "tl",
      currentTool: "Bash(go build ./...)",
      state: { ...base().state, toolCalls: 12 },
    });
    expect(
      composeStatusline(
        payload({ contextPercent: 38, costUSD: 2.5 }),
        d,
        "full",
        ["estate", "context", "tools"],
      ),
    ).toBe("[aide 0.1.8] webshop⊂tl | ctx 38% | ▸ Bash: go build ./... | ⚒12");
  });

  it("no payload extras: still renders from aide data alone", () => {
    expect(composeStatusline({}, base({ lastToolUse: AGO_2M }), "full")).toBe(
      "[aide 0.1.8] idle 2m",
    );
  });
});

describe("dir segment", () => {
  it("renders ~-relative, truncated to the last two components", () => {
    const d = base({ lastToolUse: AGO_2M, homeDir: "/home/johnm" });
    expect(
      composeStatusline(
        payload({ cwd: "/home/johnm/src/github.com/jmylchreest/aide" }),
        d,
        "full",
      ),
    ).toBe("[aide 0.1.8] …/jmylchreest/aide | Fable 5 | ctx 12% | idle 2m");
    expect(
      composeStatusline(payload({ cwd: "/home/johnm/blog" }), d, "full"),
    ).toBe("[aide 0.1.8] ~/blog | Fable 5 | ctx 12% | idle 2m");
  });

  it("workspace.current_dir wins over cwd in the payload", () => {
    expect(
      parsePayload({ cwd: "/a", workspace: { current_dir: "/b" } }).cwd,
    ).toBe("/b");
  });
});

describe("parsePayload", () => {
  it("extracts model, context, and cost from the documented shape", () => {
    const p = parsePayload({
      session_id: "s1",
      cwd: "/x",
      model: { id: "claude-fable-5", display_name: "Fable 5" },
      context: { used_percent: 41.7 },
      cost: { total_cost_usd: 1.5 },
    });
    expect(p).toEqual({
      sessionId: "s1",
      cwd: "/x",
      modelName: "Fable 5",
      contextPercent: 41.7,
      costUSD: 1.5,
    });
  });

  it("derives context percent from used/max token shapes", () => {
    expect(
      parsePayload({ context_window: { used_tokens: 50_000, max_tokens: 200_000 } })
        .contextPercent,
    ).toBe(25);
  });

  it("tolerates junk", () => {
    expect(parsePayload(null)).toEqual({});
    expect(parsePayload({ model: 7, context: "nope" }).contextPercent).toBeNull();
  });
});
