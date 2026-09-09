---
sidebar_position: 5
---

# Status Dashboard

`aide status` shows a comprehensive view of AIDE's internal state.

## Usage

```bash
aide status          # Full dashboard
aide status --json   # Machine-readable JSON
```

## Output

The dashboard includes:

- **Version and project info** -- aide version, project root, current mode
- **Server** -- whether MCP/gRPC server is running, uptime
- **File watcher** -- watched paths, directory count, debounce delay, pending files
- **Code index** -- files, symbols, and references indexed
- **Findings analysers** -- per-analyser status, last run time, finding counts by severity
- **MCP tools** -- all tools with per-tool execution counts
- **Stores** -- paths and sizes of all data files
- **Environment** -- active `AIDE_*` environment variables

## Status Line (Claude Code)

AIDE can display session info in Claude Code's status line, showing mode, duration, task counts, and token usage.

Add to `.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "bun ~/.claude/bin/aide-hud.ts"
  }
}
```

Example output:

```
[aide(0.0.40)] mode:idle | 12m | tasks:done(6) wip(0) todo(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status, 5-hour token usage, and estimated time to rate limit.

## Web dashboard: token evidence

Run `aide dashboard` (or `aide dash`) and open **Tokens** for a project. Session
and date filters apply to the report.

**Overview** shows aide's contribution alongside recorded model usage:

- Output reductions compare measured text before and after processing. Proposed
  rewrites remain separate because the host may ignore them.
- Potential retrieval reductions compare recorded retrieval text with verified
  full-file references, counting each file version once per context window.
  Added text remains visible, and incomplete windows do not receive a savings
  estimate.
- Aide operations and prepared context show work performed and context overhead.
- Model counters include input, output, cached and uncached input, cache writes
  and reasoning where the host supplies them. Missing values remain unknown.

Token estimates are labelled. Cache and reasoning counters can be subsets of
input and output; text comparisons can also overlap. The report does not add
these values into a combined savings total or claim a billing reduction.
**Details** holds individual operations and context-window evidence;
**Accounting** explains measurement boundaries and coverage.

Recent events are filtered by session and date in the daemon before the result
limit is applied. A failed load shows unknown event coverage with a retry action;
it is not displayed as an empty history. Upgrade and restart both the daemon and
dashboard together so date-filtered queries use compatible versions.

### Context and subagents

Context windows are scoped by host, session and actor. Explicit subagent-start
hooks in Claude Code and Codex initialize the child's own window. Replayed starts
preserve existing state, including a pending compaction or newer reset. This
requires a CLI and daemon that support atomic state initialization; older or
unavailable components leave the child's context unknown.

Codex installations also need refreshed hook registration. In a development
checkout, rerun `./aide-dev-toggle.sh dev`; for an installed release, rerun
`aide-plugin install --platform codex`. Restart the host and daemon after the
upgrade. A rebuild alone does not add missing lifecycle hooks. Existing child
actors without a recorded start remain unknown; test with a newly started actor.

Compaction hooks use the reported actor identity. OpenCode uses each session's
identity, including child sessions, through creation and compaction. When a host
does not supply the necessary lifecycle or identity evidence, the report retains
that uncertainty. A provider cache expiry alone does not reset context evidence.

Historical observations are not reassigned after an upgrade. An unknown context
can prevent comparisons in otherwise matching windows: event receipt time alone
cannot prove which window produced a delayed result. These coverage limits do
not establish that aide saved nothing; they mean a reduction is not established
by the available records.
