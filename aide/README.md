# aide

Unified CLI for AI agent orchestration. Provides persistent storage for learnings, decisions, swarm coordination, inter-agent messaging, code indexing, and static analysis.

## Features

- **Memory Storage**: Learnings, decisions, issues, discoveries with tags and priority
- **Swarm Coordination**: Atomic task claiming, inter-agent messaging
- **Decision Tracking**: Prevent conflicting architectural choices
- **Code Indexing**: Tree-sitter symbol search, file outline, reference finding
- **Static Analysis**: Complexity, coupling, secrets, and clone detection
- **File Watcher**: Auto-reindex and re-analyze on file changes
- **Export**: Markdown and JSON export for human review

## Installation

### From Source

```bash
cd aide
go build -o aide ./cmd/aide
```

### Using Make

```bash
make build          # Build for current platform
make build-all      # Build for all platforms
make install        # Install to PATH
```

## Usage

### Memory Operations

```bash
# Add memories
aide memory add --category=learning "Found auth middleware at src/auth.ts"
aide memory add --category=decision --tags=auth,security "Using JWT for authentication"

# Search and list
aide memory search "authentication"
aide memory list --category=learning
aide memory list --plan=auth-refactor
```

### Swarm Task Management

```bash
# Create tasks
aide task create "Implement user model" --description="Create User struct"

# Claim tasks (atomic - prevents race conditions)
aide task claim task-123 --agent=executor-1

# Complete tasks
aide task complete task-123 --result="Created src/models/user.go"

# List tasks
aide task list
aide task list --status=pending
```

### Decisions

```bash
# Set a decision (append-only, latest wins)
aide decision set auth-strategy "JWT with refresh tokens" --rationale="Stateless, scalable"

# Get a decision
aide decision get auth-strategy

# View decision history
aide decision history auth-strategy
```

### Inter-Agent Messaging

```bash
# Broadcast to all agents
aide message send "User model ready" --from=executor-1

# Send to specific agent
aide message send "Can you review?" --from=executor-2 --to=executor-1

# List messages for an agent
aide message list --agent=executor-1
```

### State Management

```bash
# Global state
aide state set mode ralph

# Per-agent state
aide state set mode eco --agent=worker-1
aide state get mode --agent=worker-1
aide state clear --agent=worker-1
```

### Export

```bash
# Export to markdown
aide memory export --format=markdown --output=.aide/memory/exports/

# Export to JSON
aide memory export --format=json --output=.aide/memory/exports/
```

### Code Indexing

```bash
# Index the codebase (incremental)
aide code index

# Search for symbols
aide code search "getUser"

# List symbols in a file
aide code symbols src/auth.ts

# Find call sites for a symbol
aide code references getUserById

# Index statistics
aide code stats

# Clear the index
aide code clear
```

### Static Analysis (Findings)

```bash
# Run all analysers
aide findings run

# Run a specific analyser
aide findings run --analyser=complexity

# Scope analysis to a directory
aide findings run --path=src/

# View findings
aide findings stats                      # Health overview
aide findings list --severity=critical   # Critical findings
aide findings list --file=src/auth       # Findings in specific files
aide findings search "AWS"               # Search by keyword

# Clear findings
aide findings clear
```

Both `--analyser=` and `--analyzer=` are accepted on all commands.

#### Configuring Thresholds

Add a `findings` section to `.aide/config/aide.json`:

```json
{
  "findings": {
    "complexity": { "threshold": 15 },
    "coupling": { "fanOut": 20, "fanIn": 25 },
    "clones": { "windowSize": 50, "minLines": 20 }
  }
}
```

CLI flags override config file values. See [docs/reference.md](../docs/reference.md#configuration) for full details.

### Status Dashboard

```bash
aide status          # Full dashboard: server, watcher, index, findings, stores, env
aide status --json   # Machine-readable JSON output
```

## Server Modes

### Daemon (gRPC)

```bash
aide daemon --socket=/path/to/aide.sock
```

### MCP Server (Claude Code / OpenCode plugin)

```bash
aide mcp                     # Start MCP server
aide mcp --code-watch        # Start with file watcher for auto-reindex
```

The MCP server exposes 18 tools across 6 groups: memory, decisions, state, messaging, code, and findings.

## Storage

Data is stored in `.aide/memory/memory.db` using [bbolt](https://github.com/etcd-io/bbolt), a pure Go embedded key-value database.

```
.aide/memory/
├── memory.db             # Primary database
├── search.bleve/         # Full-text search index
├── code/
│   ├── index.db          # Code symbol database
│   └── search.bleve/     # Code full-text search index
├── findings/
│   ├── findings.db       # Findings database
│   └── search.bleve/     # Findings full-text search index
└── exports/              # Human-readable exports
    ├── learnings.md
    ├── decisions.md
    └── issues.md
```

## Environment Variables

The database path is automatically derived from the project root (`.aide/memory/memory.db`).

| Variable                  | Default | Description                                        |
| ------------------------- | ------- | -------------------------------------------------- |
| `AIDE_DEBUG`              | unset   | Set to `1` for debug logging (to `.aide/_logs/`)   |
| `AIDE_FORCE_INIT`         | unset   | Set to `1` to initialize in non-git directories    |
| `AIDE_CODE_WATCH`         | unset   | Set to `1` to enable file watcher for auto-reindex |
| `AIDE_CODE_WATCH_DELAY`   | `30s`   | Debounce delay before re-indexing after changes    |
| `AIDE_CODE_STORE_DISABLE` | unset   | Set to `1` to disable code store                   |
| `AIDE_CODE_STORE_SYNC`    | unset   | Set to `1` for synchronous code store init         |

## Development

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint
make lint

# Format
make fmt
```

## Architecture

```
aide/
├── cmd/aide/         # CLI entry point + all subcommands
└── pkg/
    ├── memory/       # Core types (Memory, Task, Decision, Message, State)
    ├── store/        # Storage backends (bbolt + bleve)
    ├── findings/     # Static analysers (complexity, coupling, secrets, clones)
    ├── watcher/      # File watcher for auto-reindex and auto-analysis
    ├── aideignore/   # .aideignore file parser (gitignore syntax)
    ├── grpcapi/      # gRPC daemon server/client
    └── server/       # HTTP API server
```

## License

MIT
