---
sidebar_label: Overview
sidebar_position: 1
title: Modes
---

# Modes

AIDE supports several execution modes that change how your AI assistant operates. Modes are activated by keywords or explicit commands.

## Available Modes

| Mode          | Activation                    | Behavior                                     |
| ------------- | ----------------------------- | -------------------------------------------- |
| **Normal**    | Default                       | Standard single-agent operation              |
| **Swarm**     | `swarm 3 implement dashboard` | Parallel agents with SDLC pipeline per story |
| **Ralph**     | `ralph fix all tests`         | Persistent execution until verified complete |
| **Eco**       | `eco`                         | Token-efficient operation                    |
| **Autopilot** | `autopilot`                   | Full autonomous execution                    |

## Mode State

The current mode is tracked in AIDE's state system and can be queried:

```bash
aide state get mode              # Current global mode
aide state get mode --agent=x    # Per-agent mode
aide state set mode ralph        # Set mode explicitly
```

Modes are also visible in the [Status Dashboard](/docs/features/status-dashboard) and Claude Code's status line.

## Next Steps

- [Swarm Mode](./swarm.md) — Parallel multi-agent execution with SDLC pipelines
- [Ralph Mode](./ralph.md) — Persistent execution until tasks are complete
