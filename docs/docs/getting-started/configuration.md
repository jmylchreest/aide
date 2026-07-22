---
sidebar_position: 6
---

# Configuration

AIDE is configured through environment variables. All variables are optional.

## Environment Variables

| Variable                         | Description                                      |
| -------------------------------- | ------------------------------------------------ |
| `AIDE_DEBUG=1`                   | Enable debug logging (logs to `.aide/_logs/`)    |
| `AIDE_FORCE_INIT=1`              | Force initialization in non-git directories      |
| `AIDE_CODE_WATCH=0`              | Disable file watching for auto-reindex (default: on; equivalent to `code.watch` in `.aide/config/aide.json`) |
| `AIDE_CODE_WATCH_DELAY=30s`      | Delay before re-indexing after file changes      |
| `AIDE_INDEX_NON_VCS=1`           | Allow watcher/indexing in non-VCS dirs (default: refuse) |
| `AIDE_INDEX_WORKERS=N`           | Parallel parser workers for code indexing (default: NumCPU, capped at 32) |
| `AIDE_MEMORY_INJECT=0`           | Disable memory injection                         |
| `AIDE_CASCADE_DISABLED=1`        | Disable all cross-project decision layers in session context: the ancestor cascade and the peer subscription layer (session-start fetch and session-end publish included) |
| `AIDE_MCP_SYNC=0`                | Disable cross-assistant MCP server sync (default: enabled) |
| `AIDE_MEMORY_SCORING_DISABLED=1` | Disable memory scoring (use chronological order) |
| `AIDE_MEMORY_DECAY_DISABLED=1`   | Disable recency decay in memory scoring          |
| `AIDE_SHARE_AUTO_IMPORT=1`       | Auto-import shared decisions/memories on start   |
| `AIDE_MAINTENANCE_COMPACT_ON_EXIT=0` | Disable automatic bolt-store compaction when the daemon/MCP server exits (default: on) |
| `AIDE_REFLECT=1`                 | Enable the reflect Stop hook (extracts instinct proposals from session observe events). Accepts any truthy value: `1`/`true`/`on`/`yes`. Equivalent to `reflect.enabled=true` in `.aide/config/aide.json`. Env wins when set; otherwise the config file value wins; otherwise default off. |

## Where env vars are read from

aide is invoked from three surfaces, each with a different env scope. Setting
an `AIDE_*` var in the wrong place is a common gotcha — this table maps each
var to where it actually needs to live:

| Variable               | Read by              | Set where                                           |
| ---------------------- | -------------------- | --------------------------------------------------- |
| `AIDE_CODE_WATCH`      | hooks + CLI + daemon | shell that launches the harness, **or** MCP env block |
| `AIDE_CODE_WATCH_DELAY`| daemon               | MCP env block (used at daemon startup)              |
| `AIDE_CASCADE_DISABLED`| CLI (`session init`/`session end`, spawned by hooks) | shell that launches the harness |
| `AIDE_DEBUG`           | hooks + CLI          | shell that launches the harness                     |
| `AIDE_FORCE_INIT`      | CLI                  | shell at CLI invocation time                        |
| `AIDE_INDEX_NON_VCS`   | daemon               | MCP env block                                       |
| `AIDE_MCP_SYNC`        | session-start hook   | shell that launches the harness                     |
| `AIDE_MEMORY_INJECT`   | hooks                | shell that launches the harness                     |
| `AIDE_MEMORY_SCORING_DISABLED` | daemon       | MCP env block                                       |
| `AIDE_MEMORY_DECAY_DISABLED`   | daemon       | MCP env block                                       |
| `AIDE_PROJECT_ROOT`    | CLI                  | shell at CLI invocation                             |
| `AIDE_REFLECT`         | hooks + CLI          | **either** shell **or** `reflect.enabled` in `.aide/config/aide.json` |
| `AIDE_SHARE_AUTO_IMPORT` | session-start hook | shell that launches the harness                     |

**MCP env block** = the `env` (Claude Code) or `environment` (OpenCode)
mapping under the aide MCP server in the harness's config. This block only
propagates to the MCP daemon subprocess — **not to hooks**.

**Shell env at harness launch** = what's exported in your shell when you run
`claude` / `opencode`. The harness's process env (and therefore its spawned
hooks) inherits these.

### Claude Code

To set an env var visible to both hooks and the MCP daemon, the cleanest
place is the user-level `~/.claude/settings.json`:

```json
{
  "env": {
    "AIDE_REFLECT": "1",
    "AIDE_CODE_WATCH": "1"
  }
}
```

Or in your shell rc (`~/.zshrc` / `~/.bashrc`):

```bash
export AIDE_REFLECT=1
export AIDE_CODE_WATCH=1
```

Either works — Claude Code's process env inherits both.

### OpenCode

OpenCode reads env from the launching shell. Same `export` pattern in your
shell rc. The aide plugin's `environment` block in `opencode.json` only
propagates to the aide MCP daemon, not to OpenCode's own process or its
spawned hooks.

### `.aide/config/aide.json` for project-local settings

For settings that should travel with the project (not the developer):

```json
{
  "reflect": { "enabled": true },
  "memory":  { "scoring_enabled": true, "decay_enabled": true }
}
```

The Go-side config layer reads this file. The TS-side `reflect.enabled`
check (used by `skill-injector.ts` for the convergence user-prompt emit)
also reads this file directly. Env vars take precedence over file values.

## Project Configuration

Project-level settings can be stored in `.aide/config/aide.json`:

```json
{
  "findings": {
    "complexity": {
      "threshold": 10
    },
    "coupling": {
      "fanOut": 15,
      "fanIn": 20
    },
    "clones": {
      "windowSize": 50,
      "minLines": 6
    }
  }
}
```

| Setting                         | Default | Description                                  |
| ------------------------------- | ------- | -------------------------------------------- |
| `findings.complexity.threshold` | 10      | Cyclomatic complexity threshold per function |
| `findings.coupling.fanOut`      | 15      | Maximum outgoing imports before flagging     |
| `findings.coupling.fanIn`       | 20      | Maximum incoming imports before flagging     |
| `findings.clones.windowSize`    | 50      | Sliding window size in tokens for detection  |
| `findings.clones.minLines`      | 6       | Minimum clone size in lines to report        |

| `cleanup.enabled`               | true    | Master switch for retention pruning (daemon loop + session-init sweep) |
| `cleanup.observe_max_age`       | 2160h   | TTL for observe/telemetry events, 90 days (`0` = keep forever) |
| `cleanup.task_max_age`          | 2160h   | TTL for completed tasks, 90 days (pending/claimed are never pruned) |
| `cleanup.state_max_age`         | 2160h   | TTL for per-agent session state, 90 days |
| `cleanup.token_max_age`         | 2160h   | TTL for token events, 90 days |
| `maintenance.compact_on_exit`   | true    | Rewrite bolt stores to reclaim free pages when the daemon/MCP server exits |
| `hud.format`                    | full    | Statusline format: `full` or `minimal` (minimal drops model, tool count, and cost) |
| `hud.segments`                  | (all)   | Whitelist of optional statusline segments: `dir`, `estate`, `mode`, `model`, `context`, `tools`, `agents`, `cost`. The activity segment always renders. Set via `aide config set hud.segments dir context tools` |

Retention runs in the background loop of whichever long-lived process holds
the store (the daemon or the MCP primary, every 15m) and, when neither is
running, as a rate-limited sweep at session init — so data older than the
configured TTLs is always removed either way. Memories and decisions are
never retention-pruned: they are knowledge, not telemetry.

Values in `aide.json` serve as project-level defaults. CLI flags override config file values. If neither is set, the built-in defaults apply.

## Managing Configuration from the CLI

Rather than editing `aide.json` by hand, use the `aide config` command family. It
reads and writes the same `.aide/config/aide.json` file (or the user-global file
with `--global`), and reports where each value comes from.

```bash
aide config show                              # Print effective config + each value's source
aide config get cleanup.observe_max_age       # Read one value
aide config set maintenance.compact_on_exit false   # Write a project value
aide config set memory.scoring_enabled true   # Booleans, strings, numbers
aide config unset cleanup.observe_max_age     # Remove a project override
aide config path                              # Show the config file path
```

Precedence (lowest to highest): built-in defaults → user-global
`~/.aide/config/aide.json` → project `.aide/config/aide.json` → `AIDE_*`
environment variables. Use `aide config set --global <key> <value>` to write the
user-global file, which applies to every project unless that project overrides
it. The global file is **config only** — never data or a database.

## File Exclusions

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax:

```gitignore
# Exclude generated files
*.generated.ts
*.min.js

# Exclude vendor
vendor/

# But include important vendor files
!vendor/important.go
```

Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Troubleshooting

```bash
aide version                              # Check binary
aide status                               # Full system dashboard
AIDE_DEBUG=1 claude                       # Debug logging (or AIDE_DEBUG=1 opencode)
```
