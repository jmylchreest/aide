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
    "command": "bash ~/.claude/bin/aide-hud.sh"
  }
}
```

Example output:

```
[aide(0.0.40)] mode:idle | 12m | tasks:done(6) wip(0) todo(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status, 5-hour token usage, and estimated time to rate limit.
