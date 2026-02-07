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
    ├── memory/store.db   # Memories, decisions, state (BBolt)
    ├── memory/search.bleve/  # Full-text search index
    └── code/             # Code symbol index
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

| Hook | Event | Purpose |
|------|-------|---------|
| `session-start.ts` | SessionStart | Initialize state, inject memories |
| `skill-injector.ts` | UserPromptSubmit | Fuzzy-match skills, inject content |
| `pre-tool-enforcer.ts` | PreToolUse | Block writes for read-only agents |
| `hud-updater.ts` | PostToolUse | Update status line |
| `memory-capture.ts` | PostToolUse | Capture `<aide-memory>` tags |
| `persistence.ts` | Stop | Prevent stop with incomplete tasks |

### Skills (`skills/`)

Markdown files with YAML frontmatter defining triggers:

```yaml
---
name: autopilot
triggers:
  - autopilot
  - build me
---
# Autopilot Mode
...workflow instructions...
```

Fuzzy matching tolerates typos (e.g., "atopilot" matches "autopilot").

## Data Flow

```
User prompt → Hooks detect keywords → Inject skill context
           → Claude calls MCP tools → aide binary queries/updates storage
           → Hook captures memories → Storage updated
```

## MCP Tools

All tools are read-only from Claude's perspective (writes happen via hooks):

| Tool | Purpose |
|------|---------|
| `memory_search` | Full-text search with fuzzy matching |
| `memory_list` | List by category/tags |
| `code_search` | Search code symbols |
| `code_symbols` | List symbols in file |
| `decision_get` | Get decision for topic |
| `state_get` | Get session/agent state |
| `message_list` | Inter-agent messages |

## Storage

All data in `.aide/` (per-project, git-root aware):

| Path | Format | Contents |
|------|--------|----------|
| `memory/store.db` | BBolt | Memories, decisions, state, tasks, messages |
| `memory/search.bleve/` | Bleve | Full-text search index |
| `code/` | BBolt + Bleve | Symbol index with search |

## Swarm Coordination

For parallel agents, each gets:
- Isolated git worktree (`.aide/worktrees/<task>/`)
- Own branch (`feat/<task>-<agent>`)
- Shared memory for task claiming, decisions, messages

Agents claim tasks atomically to prevent conflicts.
