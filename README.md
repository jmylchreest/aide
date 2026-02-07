# AIDE - AI Development Environment

A Claude Code plugin for multi-agent orchestration, persistent memory, and intelligent workflows.

## Why AIDE?

| Without AIDE | With AIDE |
|--------------|-----------|
| Context lost between sessions | Memories persist and auto-inject |
| Manual task coordination | Swarm mode with parallel agents |
| Repeated setup instructions | Skills activate by keyword |
| No code search | Fast symbol search across codebase |
| Decisions forgotten | Decisions recorded and enforced |

**Quick start:**
```bash
# Add the marketplace
claude plugin marketplace add jmylchreest/aide

# Install the plugin
claude plugin install aide@aide
```

## Installation

### From GitHub Marketplace (Recommended)

```bash
# Step 1: Add the marketplace source
claude plugin marketplace add jmylchreest/aide

# Step 2: Install the plugin
claude plugin install aide@aide
```

Or register the marketplace in `~/.claude/settings.json` (or `.claude/settings.json` for project-level) so team members are prompted to install:
```json
{
  "extraKnownMarketplaces": {
    "aide": {
      "source": {
        "source": "github",
        "repo": "jmylchreest/aide"
      }
    }
  }
}
```

**Binary included:** The `aide` binary is automatically downloaded when the plugin is installed (or upgraded) and bundled in the plugin's `bin/` directory. No separate binary installation needed.

### From Local Clone (Development)

```bash
git clone https://github.com/jmylchreest/aide
cd aide

# Build the aide binary (requires Go 1.21+)
cd aide && go build -o ../bin/aide ./cmd/aide && cd ..

# Build TypeScript hooks
npm install && npm run build

# Use as plugin
claude --plugin-dir /path/to/aide
```

## Permissions

Add to `~/.claude/settings.json` for smooth operation:

```json
{
  "permissions": {
    "allow": [
      "Bash(aide *)",
      "Bash(**/aide *)",
      "Bash(git worktree *)",
      "mcp__plugin_aide_aide__*"
    ]
  }
}
```

This pre-approves:
- All `aide` CLI commands
- Git worktree operations (for swarm mode)
- All AIDE MCP tools (memory, code search, state, etc.)

## Quick Examples

### Memory

```
You: remember that I prefer vitest for testing
Claude: ✓ Stored preference

# Next session...
You: what testing framework should I use?
Claude: Based on your preferences, you prefer vitest for testing.
```

### Code Search

```
You: find all functions related to authentication
Claude: [Uses code_search tool]

# Code Search Results

## `authenticateUser` [function]
**File:** `src/auth/middleware.ts:24`
**Signature:** `async function authenticateUser(req: Request): Promise<User>`

## `validateToken` [function]
**File:** `src/auth/jwt.ts:45`
**Signature:** `function validateToken(token: string): TokenPayload`
```

### Modes

```
You: autopilot build a REST API for task management
→ Plans, implements, tests, and commits autonomously

You: ralph fix all the failing tests
→ Won't stop until all tests pass (Ralph Wiggum methodology)

You: swarm 3 implement the dashboard
→ Spawns 3 parallel agents with isolated git worktrees
```

## Architecture: MCP Reads, Hooks Write

AIDE follows a clear separation:

| Component | Purpose | Examples |
|-----------|---------|----------|
| **MCP Tools** | Read state, search | `memory_search`, `code_search`, `state_get` |
| **Hooks** | Write state, detect keywords | Session start, keyword detection, HUD updates |

This prevents conflicts - Claude reads via MCP, hooks update state in response to events.

### Hooks

| Hook | Trigger | Purpose |
|------|---------|---------|
| `SessionStart` | New conversation | Initialize state, inject memories |
| `UserPromptSubmit` | User sends message | Inject matching skills (fuzzy trigger matching) |
| `PostToolUse` | After any tool | Update HUD, capture `<aide-memory>` tags |
| `SubagentStart/Stop` | Agent lifecycle | Track active agents |
| `Stop` | Conversation ends | Persistence check |

### MCP Tools

| Tool | Purpose |
|------|---------|
| `memory_search` | Full-text search memories (Bleve) |
| `memory_list` | List memories by category/tags |
| `code_search` | Search code symbols |
| `code_symbols` | List symbols in a file |
| `code_stats` | Index statistics |
| `state_get` | Get session/agent state |
| `state_list` | List all state values |
| `decision_get` | Get latest decision for topic |
| `decision_list` | List all decisions |
| `decision_history` | Full history for a topic |
| `message_list` | Inter-agent messages |

## Memory System

Memories are organized by **category** (`learning`, `decision`, `session`, `pattern`, `gotcha`) and **tags**.

**Key tags:**
- `scope:global` - Inject in every session
- `project:<name>` - Project-specific
- `session:<id>` - Tied to specific session

**On session start**, AIDE auto-injects global preferences and project decisions.

**Storing:** Use natural language ("remember this...") or `/aide:memorise`

**Recalling:** Ask questions ("what did we decide...") or `/aide:recall`

**Search:** Full-text with fuzzy matching, typo tolerance, and prefix search.

## Code Indexing

Fast symbol search using [tree-sitter](https://tree-sitter.github.io/). Supports TypeScript, JavaScript, Go, Python, Rust, and many more.

```bash
aide code index              # Index codebase
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
```

**File watching:** Set `AIDE_CODE_WATCH=1` for auto-reindexing on file changes.

## Swarm Mode

Parallel agents with shared memory and git worktree isolation.

### How It Works

```
         ┌─────────────────────────────┐
         │      SHARED MEMORY          │
         │  (aide database)            │
         │  • Tasks • Decisions        │
         │  • Messages • Learnings     │
         └─────────────┬───────────────┘
                       │
       ┌───────────────┼───────────────┐
       │               │               │
  ┌────┴────┐    ┌────┴────┐    ┌────┴────┐
  │ Agent 1 │    │ Agent 2 │    │ Agent 3 │
  │ feat/a  │    │ feat/b  │    │ feat/c  │
  └─────────┘    └─────────┘    └─────────┘
```

### Activation

```
swarm 3 implement the dashboard components
```

### Workflow

1. **Decompose** - Break work into parallelizable tasks
2. **Create worktrees** - Each agent gets `feat/<feature>` branch
3. **Coordinate** - Agents use shared memory for:
   - Task claiming (atomic)
   - Decision sharing
   - Discovery broadcasting
   - Messaging
4. **Merge** - Combine all branches when complete

### Git Worktree Management

```bash
# Worktrees created automatically as:
.aide/worktrees/<task>-<agent>/
# On branch: feat/<task>-<agent>
```

### Resolving Worktrees

Use `/aide:worktree-resolve` to merge branches back to main, or manually:

```bash
git worktree list                    # List worktrees
git merge feat/task1-agent1          # Merge each branch
git worktree remove .aide/worktrees/task1-agent1  # Cleanup
```

## Skills

Skills are markdown files that inject context when triggered by keywords (with fuzzy matching for typos).

### Built-in Skills

| Skill | Example Prompt | What Happens |
|-------|----------------|--------------|
| **autopilot** | `autopilot build a REST API` | Full autonomous execution - plans, implements, tests, commits |
| **ralph** | `ralph fix all failing tests` | Won't stop until verified complete (Ralph Wiggum methodology) |
| **plan** | `plan the auth system` | Interactive planning interview before implementation |
| **swarm** | `swarm 3 implement dashboard` | Spawns N parallel agents with isolated git worktrees |
| **build-fix** | `fix the build errors` | Iteratively fixes build/lint/type errors until clean |
| **debug** | `debug why login fails` | Systematic debugging with hypothesis testing |
| **test** | `write tests for auth` | Creates test suite with coverage verification |
| **perf** | `optimize the API` | Performance profiling and optimization workflow |
| **review** | `review this PR` | Security-focused code review |
| **memorise** | `remember I prefer vitest` | Stores info for future sessions |
| **recall** | `what testing framework?` | Searches memories and decisions |
| **git** | `help with git rebase` | Expert git operations |
| **worktree-resolve** | `merge the swarm branches` | Intelligently merges worktrees with conflict resolution |

### Custom Skills

Create `.aide/skills/my-skill.md`:

```markdown
---
name: deploy
triggers:
  - deploy
  - ship it
---

# Deploy Workflow

1. Run tests: `npm test`
2. Build: `npm run build`
3. Deploy: `./scripts/deploy.sh`
```

Skills in `.aide/skills/` are auto-discovered and hot-reloaded.

## CLI Reference

Run `aide --help` for full CLI documentation. Key commands:

```bash
aide memory add/search/list/delete   # Memory management
aide decision set/get/list/history   # Architectural decisions
aide state set/get/list              # Session/agent state
aide task create/claim/complete/list # Swarm task coordination
aide message send/list/ack           # Inter-agent messaging
aide code index/search/symbols       # Code indexing
```

## Configuration

Key environment variables:

| Variable | Description |
|----------|-------------|
| `AIDE_DEBUG=1` | Enable debug logging (logs to `.aide/_logs/`) |
| `AIDE_CODE_WATCH=1` | Enable file watching for auto-reindex |
| `AIDE_MEMORY_INJECT=0` | Disable memory injection |

## Storage

All data stored in `.aide/` (per-project):
- `memory/store.db` - Memories, tasks, decisions (BBolt)
- `memory/search.bleve/` - Full-text search index
- `code/` - Code symbol index
- `skills/` - Custom skills
- `_logs/` - Debug logs (gitignored)

The `aide` binary is bundled with the plugin in `<plugin>/bin/aide`.

## Troubleshooting

```bash
# Check if binary exists and works
aide version

# Reinstall plugin to re-download binary
claude plugin uninstall aide
claude plugin install aide@aide

# Enable debug logging
AIDE_DEBUG=1 claude

# Check memories
aide memory list
```

## License

MIT
