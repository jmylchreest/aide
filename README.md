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
You: swarm 3 implement the dashboard
→ Spawns 3 parallel agents, each with SDLC pipeline (DESIGN→TEST→DEV→VERIFY→DOCS)

You: ralph fix all the failing tests
→ Won't stop until all tests pass (Ralph Wiggum methodology)

You: design the auth system
→ Technical spec with interfaces, decisions, and acceptance criteria
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
| `SessionEnd` | Session closes cleanly | Cleanup session state, record metrics |
| `UserPromptSubmit` | User sends message | Inject matching skills (fuzzy trigger matching) |
| `PreToolUse` | Before tool execution | Track current tool, enforce read-only agent restrictions |
| `PostToolUse` | After any tool | Update HUD |
| `SubagentStart/Stop` | Agent lifecycle | Track active agents, inject memories |
| `Stop` | Conversation ends | Persist state, capture session summary, agent cleanup |
| `PreCompact` | Before context compaction | Preserve state snapshot before summarization |
| `TaskCompleted` | Task marked complete | Validate SDLC stage completion (tests pass, lint clean) |

### MCP Tools

| Tool | Purpose |
|------|---------|
| `memory_search` | Full-text search memories (Bleve) |
| `memory_list` | List memories by category/tags |
| `code_search` | Search code symbols |
| `code_symbols` | List symbols in a file |
| `code_references` | Find call sites for a symbol |
| `code_stats` | Index statistics |
| `state_get` | Get session/agent state |
| `state_list` | List all state values |
| `decision_get` | Get latest decision for topic |
| `decision_list` | List all decisions |
| `decision_history` | Full history for a topic |
| `message_list` | Inter-agent messages |
| `usage` | Show Claude Code token usage statistics |

## Memory System

Memories are organized by **category** (`learning`, `decision`, `session`, `pattern`, `gotcha`, `discovery`, `blocker`, `issue`) and **tags**.

**Key tags:**
- `scope:global` - Inject in every session, across all projects
- `project:<name>` - Project-specific memories
- `session:<id>` - Tied to specific session

**Storing:** Use natural language ("remember this...") or `/aide:memorise`

**Recalling:** Ask questions ("what did we decide...") or `/aide:recall`

**Search:** Full-text with fuzzy matching, typo tolerance, and prefix search.

### Automatic Injection

Memories are automatically injected into context at specific lifecycle events:

| Event | Hook | What Gets Injected |
|-------|------|-------------------|
| Session Start | `SessionStart` | Global memories, project memories, project decisions, recent session summaries |
| Subagent Spawn | `SubagentStart` | Global memories, project memories, project decisions |

**Session Start** injects comprehensive context to ground the conversation:
- All memories tagged `scope:global` (preferences, patterns)
- All memories tagged `project:<current-project>` (project-specific context)
- All project decisions (architectural choices)
- Recent session summaries (continuity)

**Subagent Start** injects context to keep subagents aligned:
- Global memories (`scope:global`)
- Project memories (`project:<current-project>`)
- Project decisions (so subagents respect architectural choices)

### Automatic Capture

Session summaries are automatically captured by the Stop hook when a session ends with meaningful activity (files modified, tools used, or git commits made).

### Storing Memories

Use the `/aide:memorise` skill or the CLI directly:

```bash
aide memory add --category=learning --tags=testing,vitest "User prefers vitest over jest for testing"
```

**Valid categories:** `learning`, `decision`, `session`, `pattern`, `gotcha`

### Decision System

Decisions are a specialized memory type for architectural choices that need to be enforced.

**Storage:**
```bash
aide decision set "auth-strategy" "JWT with refresh tokens" --rationale="Stateless, mobile-friendly"
```

**Retrieval:**
```bash
aide decision get "auth-strategy"    # Latest decision
aide decision list                   # All decisions
aide decision history "auth-strategy" # Full history
```

**Superseding:** When a new decision is set for an existing topic, it supersedes the old one. The history is preserved, but only the latest decision is injected into context.

**Injection:** All current project decisions are injected at session start and subagent spawn, ensuring all agents respect architectural choices.

## Code Indexing

Fast symbol search using [tree-sitter](https://tree-sitter.github.io/). Supports TypeScript, JavaScript, Go, Python, Rust, and many more.

```bash
aide code index              # Index codebase
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
```

**File watching:** Set `AIDE_CODE_WATCH=1` for auto-reindexing on file changes.

## Swarm Mode

Parallel agents with SDLC pipeline per story, shared memory, and git worktree isolation.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│  ORCHESTRATOR                                                            │
│  1. Decompose work into stories                                          │
│  2. Create worktree per story                                           │
│  3. Spawn story agent per worktree                                      │
│  4. Monitor progress via TaskList                                       │
└─────────────────────────────────────────────────────────────────────────┘
                              │
       ┌──────────────────────┼──────────────────────┐
       ▼                      ▼                      ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│ Story A Agent   │   │ Story B Agent   │   │ Story C Agent   │
│ (worktree-a)    │   │ (worktree-b)    │   │ (worktree-c)    │
├─────────────────┤   ├─────────────────┤   ├─────────────────┤
│ SDLC Pipeline:  │   │ SDLC Pipeline:  │   │ SDLC Pipeline:  │
│ [DESIGN]        │   │ [DESIGN]        │   │ [DESIGN]        │
│ [TEST]          │   │ [TEST]          │   │ [TEST]          │
│ [DEV]           │   │ [DEV]           │   │ [DEV]           │
│ [VERIFY]        │   │ [VERIFY]        │   │ [VERIFY]        │
│ [DOCS]          │   │ [DOCS]          │   │ [DOCS]          │
└─────────────────┘   └─────────────────┘   └─────────────────┘
        │                     │                     │
        └─────────────────────┼─────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  SHARED NATIVE TASKS (Claude Code TaskList)                             │
│  All agents can see, create, and update tasks                           │
│  Dependencies via blockedBy auto-manage stage ordering                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### SDLC Stages

Each story agent executes these stages in order:

| Stage | Skill | Purpose |
|-------|-------|---------|
| DESIGN | `/aide:design` | Technical spec, interfaces, acceptance criteria |
| TEST | `/aide:test` | Write failing tests (TDD) |
| DEV | `/aide:implement` | Make tests pass |
| VERIFY | `/aide:verify` | Full QA validation |
| DOCS | `/aide:docs` | Update documentation |

### Activation

```
swarm 3                              # 3 story agents (SDLC mode)
swarm stories "Auth" "Payments"      # Named stories
swarm 2 --flat                       # Flat task mode (legacy)
```

### Workflow

1. **Decompose** - Break work into independent stories
2. **Create worktrees** - Each story gets isolated `feat/<story>` branch
3. **Spawn agents** - Each agent manages its own SDLC pipeline
4. **Track progress** - Use `TaskList` to see all stage tasks across agents
5. **Merge** - Use `/aide:worktree-resolve` when all stories complete

### Coordination

Agents share state via:
- **Native Tasks** - Claude Code's TaskCreate/Update/List (visible to all agents)
- **Decisions** - `aide decision set/get` for architectural choices
- **Memory** - `aide memory add` for discoveries and learnings
- **Messages** - `aide message send` for direct communication

### Git Worktree Management

```bash
# Worktrees created automatically as:
.aide/worktrees/story-<name>/
# On branch: feat/story-<name>
```

### Resolving Worktrees

Use `/aide:worktree-resolve` to merge branches back to main, or manually:

```bash
git worktree list                    # List worktrees
git merge feat/story-auth            # Merge each branch
git worktree remove .aide/worktrees/story-auth  # Cleanup
```

## Skills

Skills are markdown files that inject context when triggered by keywords (with fuzzy matching for typos).

### Built-in Skills

| Skill | Example Prompt | What Happens |
|-------|----------------|--------------|
| **swarm** | `swarm 3 implement dashboard` | Spawns N parallel agents with SDLC pipeline per story |
| **decide** | `help me decide on auth strategy` | Formal decision-making interview, records architectural choices |
| **design** | `design the auth system` | Technical spec with interfaces, decisions, acceptance criteria |
| **test** | `write tests for auth` | Creates test suite with coverage verification |
| **implement** | `implement the feature` | TDD implementation - make failing tests pass |
| **verify** | `verify the implementation` | Full QA: tests, lint, types, debug artifact check |
| **docs** | `update the documentation` | Updates docs to match implementation |
| **ralph** | `ralph fix all failing tests` | Won't stop until verified complete (Ralph Wiggum methodology) |
| **build-fix** | `fix the build errors` | Iteratively fixes build/lint/type errors until clean |
| **debug** | `debug why login fails` | Systematic debugging with hypothesis testing |
| **perf** | `optimize the API` | Performance profiling and optimization workflow |
| **review** | `review this PR` | Security-focused code review |
| **code-search** | `find all auth functions` | Search code symbols and find call sites |
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

## Status Line

AIDE can display session info in Claude Code's status line, showing mode, duration, task counts, and token usage.

**Setup:** Add to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "bash ~/.claude/bin/aide-hud.sh"
  }
}
```

The wrapper script (`~/.claude/bin/aide-hud.sh`) is automatically installed on first session start after plugin installation.

**Example output:**
```
[aide(0.0.4)] mode:idle | 12m | tasks:✓(6) ●(0) ○(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status (completed/in-progress/pending), 5-hour token usage, and estimated time to rate limit.

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
