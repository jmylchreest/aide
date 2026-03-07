---
sidebar_label: Storage
sidebar_position: 5
title: Storage Layout
---

# Storage Layout

All AIDE data is stored in `.aide/` at the project root. A `.aide/.gitignore` is automatically created on first session to separate machine-local data from shareable content.

## Directory Structure

```
.aide/
├── memory/
│   ├── memory.db              # Primary database (BBolt)
│   ├── search.bleve/          # Full-text search index
│   ├── code/
│   │   ├── index.db           # Code symbol database
│   │   └── search.bleve/      # Code symbol search index
│   └── findings/
│       ├── findings.db        # Static analysis findings
│       └── search.bleve/      # Findings search index
├── state/                     # Runtime state (HUD, session info, worktrees)
├── bin/                       # Downloaded aide binary
├── worktrees/                 # Git worktree directories (swarm mode)
├── _logs/                     # Debug logs (when AIDE_DEBUG=1)
├── config/
│   ├── aide.json              # Project-level configuration
│   ├── mcp.json               # Canonical MCP server config
│   └── mcp-sync.journal.json  # MCP sync deletion journal
├── shared/                    # Exported decisions and memories
│   ├── decisions/             # One markdown file per topic
│   └── memories/              # One markdown file per category
├── skills/                    # Project-specific custom skills
└── .gitignore                 # Auto-generated
```

## Git Tracking Rules

### Gitignored (machine-local runtime data)

These are binary databases, runtime state, and machine-specific files that shouldn't be committed:

| Path                           | Purpose                                                     |
| ------------------------------ | ----------------------------------------------------------- |
| `memory/`                      | BBolt database + Bleve search index (binary, non-mergeable) |
| `state/`                       | Runtime state (HUD output, session info, worktree tracking) |
| `bin/`                         | Downloaded aide binary                                      |
| `worktrees/`                   | Git worktree directories for swarm mode                     |
| `_logs/`                       | Debug logs (when `AIDE_DEBUG=1`)                            |
| `config/mcp.json`              | Canonical MCP server config (machine-specific sync state)   |
| `config/mcp-sync.journal.json` | Tracks intentional MCP server removals                      |

### Shared via git

The `shared/` directory is explicitly un-ignored (`!shared/` in `.gitignore`):

| Path                | Purpose                                               |
| ------------------- | ----------------------------------------------------- |
| `shared/decisions/` | Exported decisions as markdown (one file per topic)   |
| `shared/memories/`  | Exported memories as markdown (one file per category) |

Files use YAML frontmatter + markdown body, making them useful as LLM context even without AIDE installed.

### Tracked by default (committing optional)

| Path               | Purpose                          |
| ------------------ | -------------------------------- |
| `config/aide.json` | Project-level AIDE configuration |
| `skills/`          | Project-specific custom skills   |

## Sharing Knowledge via Git

The `share` commands let you export and import AIDE knowledge through version control:

```bash
aide share export                    # Export decisions + memories to .aide/shared/
aide share export --decisions        # Decisions only
aide share import                    # Import from .aide/shared/
aide share import --dry-run          # Preview what would be imported
```

### Automatic Import

Set `AIDE_SHARE_AUTO_IMPORT=1` to automatically import from `.aide/shared/` at session start. This means:

1. Team member A makes a decision and exports it
2. They commit `.aide/shared/decisions/auth-strategy.md`
3. Team member B pulls and starts a session
4. The decision is automatically imported into their local database

### Export Format

Exported files are plain markdown with YAML frontmatter:

```markdown
---
topic: auth-strategy
decision: JWT with refresh tokens
rationale: Stateless, mobile-friendly
timestamp: 2025-01-15T10:30:00Z
---

## Details

Using JWT tokens with refresh token rotation.
Access tokens expire after 15 minutes.
```

This format works as context for any LLM, even without AIDE installed.

## Database Details

### BBolt (memory.db)

AIDE uses [BBolt](https://github.com/etcd-io/bbolt) — an embedded key-value store. A single `memory.db` file holds:

- Memories (with categories and tags)
- Decisions (append-only per topic)
- State (global and per-agent)
- Tasks (for swarm coordination)
- Messages (inter-agent communication)

### Bleve (search.bleve/)

[Bleve](https://github.com/blevesearch/bleve) provides full-text search with:

- Standard word matching
- Fuzzy matching (Levenshtein distance 1)
- Edge n-grams for prefix matching
- N-grams for substring matching

Separate Bleve indexes exist for memories, code symbols, and findings.

### Rebuilding Indexes

If search results seem stale:

```bash
aide memory reindex     # Rebuild memory search index
aide code clear         # Clear and rebuild code index
aide findings clear     # Clear and re-run findings
```
