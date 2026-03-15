---
sidebar_label: Platform Comparison
sidebar_position: 7
title: Platform Comparison
---

# Platform Comparison

AIDE supports both Claude Code and OpenCode through platform-specific adapters. While the core functionality is shared, there are differences in how each platform integrates.

## Feature Matrix

| Feature                 | Claude Code             | OpenCode                                          |
| ----------------------- | ----------------------- | ------------------------------------------------- |
| Memory & decisions      | Full                    | Full                                              |
| Code indexing           | Full                    | Full                                              |
| Static analysis         | Full                    | Full                                              |
| Skill injection         | Via hooks               | Via system prompt transform + slash commands      |
| Swarm mode              | Native (subagent hooks) | Passive swarm-aware state; external orchestration |
| HUD / status line       | Native                  | Not supported (OpenCode TUI has no status line)   |
| Persistence (autopilot) | Stop-blocking           | Re-prompting via `session.prompt()` on idle       |
| Subagent lifecycle      | Full hooks              | Session-based tracking (observational, no spawn)  |
| Write guard             | Full                    | Full                                              |
| MCP sync                | Full                    | Full                                              |

## Detailed Comparison

### Skill Injection

**Claude Code:** Skills are injected via the `UserPromptSubmit` hook. When a user message matches a skill trigger, the skill content is returned as hook output and merged into the conversation context.

**OpenCode:** Skills are injected through system prompt transformation and are also available as `/aide:*` slash commands (e.g., `/aide:test`, `/aide:design`). This gives OpenCode users explicit control over skill activation.

### Swarm Mode

**Claude Code:** Full native support. The orchestrator spawns subagents using Claude's native subagent mechanism. Each subagent gets its own lifecycle hooks (`SubagentStart`, `SubagentStop`) for memory injection and state tracking.

**OpenCode:** Swarm-aware but externally orchestrated. The state system tracks agents and tasks, but spawning new agents requires external tooling (e.g., multiple OpenCode instances coordinated through AIDE's messaging system).

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
    "command": "bash ~/.claude/bin/aide-hud.sh"
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

## Choosing a Platform

Both platforms get the full AIDE feature set for individual development. Choose based on:

- **Claude Code** — Better for swarm mode (native subagents), status line monitoring, and teams using Claude's ecosystem
- **OpenCode** — Better if you prefer explicit slash commands for skills, or use OpenCode as your primary editor
