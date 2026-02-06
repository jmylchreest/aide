# aide

Unified CLI for AI agent orchestration. Provides persistent storage for learnings, decisions, swarm coordination, and inter-agent messaging.

## Features

- **Memory Storage**: Learnings, decisions, issues, discoveries with tags and priority
- **Swarm Coordination**: Atomic task claiming, inter-agent messaging
- **Decision Tracking**: Prevent conflicting architectural choices
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

## Server Modes

### Daemon (gRPC)

```bash
aide daemon --socket=/path/to/aide.sock
```

### MCP Server (Claude Code plugin)

```bash
aide mcp
```

## Storage

Data is stored in `.aide/memory/store.db` using [bbolt](https://github.com/etcd-io/bbolt), a pure Go embedded key-value database.

```
.aide/memory/
├── store.db              # Primary database
├── search.bleve/         # Full-text search index
└── exports/              # Human-readable exports
    ├── learnings.md
    ├── decisions.md
    └── issues.md
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AIDE_MEMORY_DB` | `.aide/memory/store.db` | Database file path |

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
├── cmd/aide/         # CLI entry point
└── pkg/
    ├── memory/       # Core types (Memory, Task, Decision, Message, State)
    ├── store/        # Storage backends (bbolt + bleve)
    ├── grpcapi/      # gRPC daemon server/client
    └── server/       # HTTP API server
```

## License

MIT
