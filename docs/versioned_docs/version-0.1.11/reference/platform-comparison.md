---
sidebar_label: Platform Comparison
sidebar_position: 7
title: Platform Comparison
---

# Platform Comparison

AIDE supports Claude Code, OpenCode, and Codex CLI through platform-specific adapters. While the core functionality is shared, there are differences in how each platform integrates.

## Feature Matrix

| Feature                 | Claude Code             | OpenCode                                          | Codex CLI                               |
| ----------------------- | ----------------------- | ------------------------------------------------- | --------------------------------------- |
| Memory & decisions      | Full                    | Full                                              | Full                                    |
| Code indexing           | Full                    | Full                                              | Full                                    |
| Static analysis         | Full                    | Full                                              | Full                                    |
| Skill injection         | Via hooks               | Via system prompt transform + slash commands      | Via hooks + native `$` mention (plugin) |
| Swarm mode              | Native (subagent hooks) | Passive swarm-aware state; external orchestration | Limited (no subagent hooks)             |
| HUD / status line       | Native                  | Not supported (OpenCode TUI has no status line)   | File-based only                         |
| Persistence (autopilot) | Stop-blocking           | Re-prompting via `session.prompt()` on idle       | Stop-blocking                           |
| Subagent lifecycle      | Full hooks              | Session-based tracking (observational, no spawn)  | Not available                           |
| Write guard             | Full                    | Full                                              | Full                                    |
| MCP sync                | Full                    | Full                                              | Full                                    |

## Detailed Comparison

### Skill Injection

**Claude Code:** Skills are injected via the `UserPromptSubmit` hook. When a user message matches a skill trigger, the skill content is returned as hook output and merged into the conversation context.

**OpenCode:** Skills are injected through system prompt transformation and are also available as `/aide:*` slash commands (e.g., `/aide:test`, `/aide:design`). This gives OpenCode users explicit control over skill activation.

**Codex CLI:** Skills are injected via the `UserPromptSubmit` hook, same as Claude Code. When installed as a Codex plugin, skills are additionally discovered natively (namespaced `aide:<name>`) and can be invoked explicitly with a `$` mention or the `/skills` picker — Codex does not create per-skill slash commands.

### Swarm Mode

**Claude Code:** Full native support. The orchestrator spawns subagents using Claude's native subagent mechanism. Each subagent gets its own lifecycle hooks (`SubagentStart`, `SubagentStop`) for memory injection and state tracking.

**OpenCode:** Swarm-aware but externally orchestrated. The state system tracks agents and tasks, but spawning new agents requires external tooling (e.g., multiple OpenCode instances coordinated through AIDE's messaging system).

**Codex CLI:** Limited. Codex has no subagent lifecycle hooks, so swarm mode is restricted to the same passive, externally orchestrated model as OpenCode.

### Status Line / HUD

**Claude Code:** AIDE can display session info in Claude Code's status line:

```
[aide(0.0.40)] mode:idle | 12m | tasks:done(6) wip(0) todo(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status, token usage, and estimated time to rate limit.

**Setup:** Add to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "bun ~/.claude/bin/aide-hud.ts"
  }
}
```

**OpenCode:** Not supported. OpenCode's TUI doesn't have a status line mechanism. Use `aide status` for the same information.

### Persistence (Autopilot Mode)

**Claude Code:** Uses stop-blocking to prevent the AI from ending the conversation early. The hook intercepts stop signals and re-injects the task prompt.

**OpenCode:** Uses `session.prompt()` to re-prompt the AI when it goes idle. When the AI stops responding, AIDE detects the idle state and sends a continuation prompt.

### Subagent Lifecycle

**Claude Code:** Full lifecycle hooks:

- `SubagentStart` — Fires when a subagent is spawned, injects memories and decisions
- `SubagentStop` — Fires when a subagent finishes, handles cleanup

**OpenCode:** Observational tracking only. AIDE tracks sessions and can detect when new sessions start, but doesn't control the spawning. Memory injection happens through shared state rather than hooks.

## Installation Differences

### Claude Code

```bash
# From marketplace (recommended)
claude plugin marketplace add jmylchreest/aide
claude plugin install aide@aide

# Permissions needed in ~/.claude/settings.json
{
  "permissions": {
    "allow": [
      "Bash(aide *)",
      "Bash(**/aide *)",
      "Bash(git worktree *)",
      "mcp__plugin_aide_aide__*"
    ]
  }
}
```

### OpenCode

```bash
# From npm
bunx @jmylchreest/aide-plugin install

# Check status
bunx @jmylchreest/aide-plugin status

# Uninstall
bunx @jmylchreest/aide-plugin uninstall
```

### Codex CLI

```bash
# Recommended: Codex plugin (MCP server + skills), then hooks
codex plugin marketplace add jmylchreest/aide
codex plugin add aide@aide
bunx @jmylchreest/aide-plugin install --platform codex

# Standalone (older Codex, no plugin support) — configures everything
bunx @jmylchreest/aide-plugin install --platform codex
```

See [Getting Started: Codex CLI](../getting-started/codex.md) for details.

## Choosing a Platform

All platforms get the core AIDE feature set for individual development. Choose based on:

- **Claude Code** — Better for swarm mode (native subagents), status line monitoring, and teams using Claude's ecosystem
- **OpenCode** — Better if you prefer explicit slash commands for skills, or use OpenCode as your primary editor
- **Codex CLI** — Full memory, code intelligence, and MCP tooling; skills via `$` mention; no subagent orchestration. The default sandbox blocks shell/hook access to the aide daemon — see [Sandboxed Shells and the aide Daemon](../getting-started/codex.md#sandboxed-shells-and-the-aide-daemon)
