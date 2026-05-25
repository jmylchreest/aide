---
name: swarm-status
description: Inspect a running swarm — show the agent tree, current tools, halts/pauses, and recent task/message activity for the orchestrator's own swarm.
triggers:
  - swarm status
  - what are the agents doing
  - show swarm
  - agent status
  - check agents
---

# Swarm Status

Quick orchestrator-side inspection of a live swarm. Run this when you want
to see what your spawned subagents are doing right now without halting them
or opening the aide-web dashboard.

## Steps

### 1. Resolve current session (your own)

```bash
./.aide/bin/aide reflect current-session
```

Use that ID as `<parent>` below — it's the orchestrator session that owns
the swarm.

### 2. List agents in this swarm

```bash
./.aide/bin/aide agent list --parent=<parent> --json
```

Returns one record per registered subagent with `parent_session`,
`namespace`, `status`, `halt`, `paused`, `deadline`.

### 3. Active tasks for this swarm

```bash
./.aide/bin/aide task list --parent-session=<parent> --json
```

Filters the project-wide task bucket to ones tagged with this swarm's
parent. Look for tasks stuck in `claimed` without progress.

### 4. Recent messages

```bash
./.aide/bin/aide message list --parent-session=<parent>
```

Cross-agent comms within this swarm. High-priority messages from the
orchestrator are surfaced to subagents on their next tool call by the
signals hook.

### 5. Summarise for the user

Group by agent. For each, report:

- Status (running / paused / halted)
- Current tool (read `agent:<id>:currentTool` via `aide state get currentTool --agent=<id>` if you want this)
- Any flags (halt with reason, pause)
- Open tasks they hold
- Deadline if set

Example output:

```
Swarm <parent-id-short> — 3 agents

agent-auth (running):
  current: Edit src/auth/handler.ts
  tasks: 2 in_progress
  no flags

agent-payments (paused):
  reason: scope drift — investigating
  tasks: 1 claimed (#42)
  paused 3m ago

agent-docs (halted):
  reason: repeated rustdoc — see new instinct
  halted 12m ago
```

### When to use halts vs. messages

- **Halt** if the agent is clearly off-track or burning budget with no
  progress. The halt blocks tool calls; the model will respond once with
  the halt reason then stop.
- **High-priority message** if you want to redirect without stopping —
  arrives as `additionalContext` on the next tool call so the model can
  read it and adjust.

Both are sent via `aide` CLI:

```bash
./.aide/bin/aide agent halt <agent-id> --reason="..."
./.aide/bin/aide message send --from=orchestrator --to=<agent-id> \
  --priority=high "redirect: focus on auth.ts only"
```

## Not for

This skill is read-only orchestration. Use the `swarm` skill itself to
launch new agents or resolve worktrees at the end.
