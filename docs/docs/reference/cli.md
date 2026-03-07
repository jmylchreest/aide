---
sidebar_label: CLI Reference
sidebar_position: 4
title: CLI Reference
---

# CLI Reference

Full command reference for the `aide` binary.

## Memory

```bash
aide memory add --category=learning --tags=testing "Prefers vitest"
aide memory search "authentication"
aide memory list --category=learning
aide memory delete <id>
aide memory reindex                      # Rebuild search index
aide memory export --format=markdown     # Export to markdown
```

| Command          | Description                               |
| ---------------- | ----------------------------------------- |
| `memory add`     | Store a new memory with category and tags |
| `memory search`  | Full-text search across memories          |
| `memory list`    | List memories, optionally by category     |
| `memory delete`  | Delete a memory by ID                     |
| `memory reindex` | Rebuild the Bleve search index            |
| `memory export`  | Export memories to markdown               |

## Decisions

```bash
aide decision set auth-strategy "JWT with refresh tokens" --rationale="Stateless"
aide decision get auth-strategy
aide decision list
aide decision history auth-strategy
aide decision delete auth-strategy
```

| Command            | Description                                        |
| ------------------ | -------------------------------------------------- |
| `decision set`     | Record a decision for a topic (appends to history) |
| `decision get`     | Get the current (latest) decision                  |
| `decision list`    | List all decision topics                           |
| `decision history` | Show full history for a topic                      |
| `decision delete`  | Delete a decision topic                            |

## Tasks

```bash
aide task create "Implement user model" --description="Create User struct"
aide task claim <id> --agent=executor-1
aide task complete <id> --result="Done"
aide task list --status=pending
aide task delete <id>
```

| Command         | Description                           |
| --------------- | ------------------------------------- |
| `task create`   | Create a new task (starts as pending) |
| `task claim`    | Atomically claim a task for an agent  |
| `task complete` | Mark a task as done with a result     |
| `task list`     | List tasks, optionally by status      |
| `task delete`   | Delete a task                         |

## Messages

```bash
aide message send "User model ready" --from=executor-1
aide message send "Can you review?" --from=executor-2 --to=executor-1
aide message list --agent=executor-1
aide message ack <id> --agent=executor-1
```

| Command        | Description                            |
| -------------- | -------------------------------------- |
| `message send` | Send a message (broadcast or directed) |
| `message list` | List messages for an agent             |
| `message ack`  | Acknowledge a message as read          |

## State

```bash
aide state set mode ralph
aide state set mode eco --agent=worker-1
aide state get mode --agent=worker-1
aide state list
aide state clear --agent=worker-1
```

| Command       | Description                             |
| ------------- | --------------------------------------- |
| `state set`   | Set a state value (global or per-agent) |
| `state get`   | Get a state value                       |
| `state list`  | List all state entries                  |
| `state clear` | Clear state for an agent                |

## Code

```bash
aide code index                          # Index codebase (incremental)
aide code search "getUser"               # Search symbols
aide code symbols src/auth.ts            # List file symbols
aide code references getUserById         # Find call sites
aide code stats                          # Index statistics
aide code clear                          # Clear index
```

| Command           | Description                                        |
| ----------------- | -------------------------------------------------- |
| `code index`      | Index the codebase using tree-sitter (incremental) |
| `code search`     | Search symbol definitions                          |
| `code symbols`    | List all symbols in a specific file                |
| `code references` | Find all call sites of a symbol                    |
| `code stats`      | Show index statistics                              |
| `code clear`      | Clear the code index                               |

## Findings

```bash
aide findings run                         # Run all analysers
aide findings run --analyser=complexity   # Run specific analyser
aide findings search "high complexity"    # Search findings
aide findings list --severity=critical    # List by severity
aide findings list --file=src/auth        # List by file
aide findings stats                       # Health overview
aide findings accept <id1> <id2>          # Accept specific findings
aide findings accept --analyzer=clones    # Accept all clone findings
aide findings accept --all                # Accept all findings
aide findings clear                       # Clear all findings
```

| Command           | Description                                  |
| ----------------- | -------------------------------------------- |
| `findings run`    | Run analysers (all or specific)              |
| `findings search` | Full-text search across findings             |
| `findings list`   | List findings by severity, file, or analyser |
| `findings stats`  | Codebase health overview                     |
| `findings accept` | Accept (dismiss) findings by ID or filter    |
| `findings clear`  | Clear all findings                           |

:::note
Both `--analyser=` and `--analyzer=` spellings are accepted on all findings commands.
:::

## Share

```bash
aide share export                        # Export decisions + memories to .aide/shared/
aide share export --decisions            # Decisions only
aide share import                        # Import from .aide/shared/
aide share import --dry-run              # Preview import
```

| Command        | Description                                      |
| -------------- | ------------------------------------------------ |
| `share export` | Export decisions and memories to `.aide/shared/` |
| `share import` | Import from `.aide/shared/`                      |

## Status

```bash
aide status                              # Full dashboard
aide status --json                       # JSON output
```

Shows version, server status, file watcher, code index, findings analysers, MCP tools, stores, and environment variables.

## Other Commands

```bash
aide session init                        # Initialize session
aide upgrade                             # Self-upgrade binary
aide daemon --socket=/path/to/aide.sock  # Start gRPC daemon
aide mcp                                 # Start MCP server
aide version                             # Show version
```

| Command        | Description                  |
| -------------- | ---------------------------- |
| `session init` | Initialize a new session     |
| `upgrade`      | Self-upgrade the aide binary |
| `daemon`       | Start the gRPC daemon        |
| `mcp`          | Start the MCP server (stdio) |
| `version`      | Show the installed version   |
