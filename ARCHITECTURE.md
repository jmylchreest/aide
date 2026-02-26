# AIDE Architecture

## Overview

AIDE is a Claude Code plugin providing persistent memory, code indexing, and multi-agent coordination. It consists of three components:

1. **aide binary (Go)** - Storage, search, MCP server
2. **TypeScript hooks** - Event handlers for Claude Code
3. **Skills** - Markdown workflows injected by keyword

## Directory Structure

```
aide/
├── aide/                 # Go binary source
│   ├── cmd/aide/         # CLI commands & MCP server
│   ├── pkg/store/        # BBolt storage + Bleve search
│   ├── pkg/code/         # Tree-sitter code indexing
│   └── pkg/server/       # HTTP API for MCP
├── src/hooks/            # TypeScript event handlers
├── skills/               # Built-in workflow skills
└── .aide/                # Project-local storage
    ├── memory/memory.db  # Memories, decisions, state (BBolt)
    ├── memory/search.bleve/  # Full-text search index
    ├── memory/code/      # Code symbol index
    └── memory/findings/  # Static analysis findings
```

## Component Responsibilities

### Go Binary (`aide/`)

**Storage** (`pkg/store/`):

- Memory CRUD with categories and tags
- Task claiming with atomic locks
- Decisions with append-only history
- State key-value per session/agent
- Inter-agent messaging

**Code Index** (`pkg/code/`):

- Tree-sitter parsing (TS, JS, Go, Python, Rust, etc.)
- Symbol extraction (functions, classes, types)
- File watching for auto-reindex
- Bleve full-text search

**CLI & MCP** (`cmd/aide/`):

- `aide memory|decision|state|task|message|code` commands
- `aide mcp` - Starts MCP server (tools for Claude)
- `aide daemon` - Background HTTP server

### TypeScript Hooks (`src/hooks/`)

| Hook File              | Event                       | Purpose                                          |
| ---------------------- | --------------------------- | ------------------------------------------------ |
| `session-start.ts`     | SessionStart                | Initialize state, inject memories                |
| `skill-injector.ts`    | UserPromptSubmit            | Fuzzy-match skills, inject content               |
| `tool-tracker.ts`      | PreToolUse                  | Track current tool per agent for HUD display     |
| `pre-tool-enforcer.ts` | PreToolUse                  | Enforce tool access rules, inject mode reminders |
| `hud-updater.ts`       | PostToolUse                 | Update status line                               |
| `session-summary.ts`   | Stop                        | Capture session summary (files, tools, commits)  |
| `subagent-tracker.ts`  | SubagentStart, SubagentStop | Track active agents, inject context              |
| `persistence.ts`       | Stop                        | Prevent stop with incomplete tasks               |
| `agent-cleanup.ts`     | Stop                        | Clean up agent-specific state                    |
| `session-end.ts`       | SessionEnd                  | Session end cleanup and metrics                  |
| `pre-compact.ts`       | PreCompact                  | Preserve context before compaction               |

**Additional hooks (available but not registered in plugin.json):**

- `task-completed.ts` - SDLC stage validation on task completion (opt-in)

### Skills (`skills/`)

Markdown files with YAML frontmatter defining triggers:

```yaml
---
name: design
triggers:
  - design this
  - design the
  - architect this
---
# Design Mode
...workflow instructions...
```

Fuzzy matching tolerates typos (e.g., "desgin" matches "design").

**Built-in skills:** `swarm`, `design`, `test`, `implement`, `verify`, `docs`, `decide`, `ralph`, `build-fix`, `debug`, `perf`, `review`, `code-search`, `memorise`, `recall`, `git`, `worktree-resolve`

## Data Flow

```
User prompt → Hooks detect keywords → Inject skill context
           → Claude calls MCP tools → aide binary queries/updates storage
           → Hook captures memories → Storage updated
```

## MCP Tools

All tools are read-only from Claude's perspective (writes happen via hooks):

| Tool               | Purpose                              |
| ------------------ | ------------------------------------ |
| `memory_search`    | Full-text search with fuzzy matching |
| `memory_list`      | List by category/tags                |
| `code_search`      | Search code symbols                  |
| `code_symbols`     | List symbols in file                 |
| `code_stats`       | Index statistics                     |
| `code_references`  | Find call sites for a symbol         |
| `decision_get`     | Get decision for topic               |
| `decision_list`    | List all decisions                   |
| `decision_history` | Full history for a topic             |
| `state_get`        | Get session/agent state              |
| `state_list`       | List all state values                |
| `message_list`     | Inter-agent messages                 |
| `usage`            | Claude Code token usage statistics   |

## Storage

All data in `.aide/` (per-project, git-root aware):

| Path                   | Format        | Contents                                    |
| ---------------------- | ------------- | ------------------------------------------- |
| `memory/memory.db`     | BBolt         | Memories, decisions, state, tasks, messages |
| `memory/search.bleve/` | Bleve         | Full-text search index                      |
| `memory/code/`         | BBolt + Bleve | Symbol index with search                    |
| `memory/findings/`     | BBolt + Bleve | Static analysis findings with search        |

## Swarm Coordination

For parallel agents, each gets:

- Isolated git worktree (`.aide/worktrees/<task>/`)
- Own branch (`feat/<task>-<agent>`)
- Shared memory for task claiming, decisions, messages

Agents claim tasks atomically to prevent conflicts.
