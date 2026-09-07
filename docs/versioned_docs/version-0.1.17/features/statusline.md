---
sidebar_position: 7
---

# Statusline

aide can render Claude Code's statusline — the single info bar at the bottom
of the terminal. Claude Code ships no default statusline; the slot holds
exactly one command, configured in `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "bun ~/.claude/bin/aide-hud.ts"
  }
}
```

`~/.claude/bin/aide-hud.ts` is a wrapper the aide session-start hook
installs and upgrades: it locates the newest plugin install and runs its
statusline script, so plugin updates flow through without reconfiguring.
Only recognizably aide-managed copies are ever overwritten (and only to
a newer wrapper version) — a hand-written file at that path is never
touched. (Developers running from a checkout can point at
`<checkout>/scripts/aide-hud.ts` directly instead.)

If you already use another statusline (ccusage, ccstatusline, a powerline),
keep it — aide doesn't need the slot and won't fight for it.

## What it shows

The line is composed at render time from three sources: Claude Code's own
statusline payload (model, context %, cost — native fields, never
re-derived), aide's session-scoped state (mode, live tool, counts,
subagents), and the session anchor (project identity and estate). Segments
render only when they carry signal — no `mode:idle` noise, no estate
segment in a standalone repo.

A quiet session:

```
[aide 0.1.x] …/jmylchreest/aide | Fable 5 | ctx 12% | idle 2m
```

A busy one, inside a nested project, with a mode engaged:

```
[aide 0.1.x] …/tl/webshop | webshop⊂tl | autopilot 3/20 | Fable 5 | ctx 71%⚠ | ▸ Edit: cmd_session.go | ⚒203
```

A swarm adds one row per running subagent:

```
[aide 0.1.x] swarm | Fable 5 | ctx 44% | idle 2m | ⚒88 | agents:2
└─ ▶[exec-1a] executor | 4m | ▸ Bash: bun run test
└─ ▶[rev-9z8] reviewer | 4m | review story-3 diff
```

## Segments

| Segment   | Data source                          | Renders when                     |
| --------- | ------------------------------------ | -------------------------------- |
| `dir`     | payload working directory            | payload provides it              |
| `estate`  | session anchor (parent chain)        | project sits inside another      |
| `mode`    | aide state (`mode`, iterations)      | a mode is engaged                |
| `model`   | payload model name                   | `full` format                    |
| `context` | payload context % (`⚠` ≥70, `‼` ≥90) | payload provides it              |
| activity  | aide state (live tool / idle age)    | **always** (not configurable)    |
| `tools`   | aide state (`⚒` tool-call count)     | `full` format, count > 0         |
| `agents`  | aide state (running subagents)       | any running (plus one row each)  |
| `cost`    | payload session cost                 | **opt-in**, `full` format, ≥ $0.01 |

The context percentage and cost are Claude Code's own numbers, passed
through verbatim. aide-side segments read a state snapshot at most 2
seconds old (the render cache).

## Configuration

Statusline settings live in aide's ordinary config (see
[Configuration](../getting-started/configuration.md)) under the `hud` key:

```bash
aide config set hud.format minimal            # full (default) or minimal
aide config set hud.segments dir context tools # whitelist segments
```

| Key            | Default            | Meaning                                          |
| -------------- | ------------------ | ------------------------------------------------ |
| `hud.format`   | `full`             | `minimal` drops the version tag detail, model, tool count, and cost |
| `hud.segments` | all except `cost`  | Whitelist of segments to render (activity always shows) |

`AIDE_HUD_FORMAT` and `AIDE_HUD_SEGMENTS` (comma-separated) override the
config files, matching aide's usual env-over-file precedence.

:::note
A legacy `.aide/config/hud.json` file, if present, still wins over both
`aide.json` layers (it predates the `hud` config section). If
`aide config set hud.format ...` appears to have no effect, delete or
migrate that file.
:::
