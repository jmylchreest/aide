# AIDE - AI Development Environment

Multi-agent orchestration, persistent memory, and intelligent workflows for AI coding assistants.

Supports **Claude Code** and **OpenCode** through a shared core with platform-specific adapters.

## Why AIDE?

| Without AIDE                    | With AIDE                          |
| ------------------------------- | ---------------------------------- |
| Context lost between sessions   | Memories persist and auto-inject   |
| Manual task coordination        | Swarm mode with parallel agents    |
| Repeated setup instructions     | Skills activate by keyword         |
| No code search                  | Fast symbol search across codebase |
| Decisions forgotten             | Decisions recorded and enforced    |
| MCP servers configured per-tool | MCP configs sync across assistants |

## Quick Start

### Claude Code

```bash
# Add the marketplace
claude plugin marketplace add jmylchreest/aide

# Install the plugin
claude plugin install aide@aide
```

### OpenCode

```bash
# Install the plugin + MCP server
bunx @jmylchreest/aide-plugin install
```

This registers the aide plugin and MCP server in your OpenCode config. Skills become available as `/aide:*` slash commands.

## Installation

### Claude Code - From Marketplace (Recommended)

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

### OpenCode - From npm

```bash
bunx @jmylchreest/aide-plugin install
```

This modifies your `opencode.json` (project-level or `~/.config/opencode/opencode.json`) to register:

- The aide plugin (hooks, skill injection, memory)
- The aide MCP server (code search, state, decisions, messages)

**Check status:**

```bash
bunx @jmylchreest/aide-plugin status
```

**Uninstall:**

```bash
bunx @jmylchreest/aide-plugin uninstall
```

### From Source (Development)

```bash
git clone https://github.com/jmylchreest/aide
cd aide

# Build the aide binary (requires Go 1.21+)
cd aide && go build -o ../bin/aide ./cmd/aide && cd ..

# Build TypeScript hooks
npm install && npm run build

# Claude Code: use as plugin
claude --plugin-dir /path/to/aide

# OpenCode: point plugin at local source
bunx @jmylchreest/aide-plugin install --plugin-path /path/to/aide
```

**Binary included:** The `aide` Go binary is automatically downloaded when the plugin is installed (or upgraded). No separate binary installation needed.

## Permissions

### Claude Code

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

### OpenCode

OpenCode uses its own permission model. Tool enforcement in swarm mode is handled by the aide plugin's `permission.ask` hook, which restricts read-only agents from write tools.

## Quick Examples

### Memory

```
You: remember that I prefer vitest for testing
Claude: Stored preference

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
-> Spawns 3 parallel agents, each with SDLC pipeline (DESIGN->TEST->DEV->VERIFY->DOCS)

You: ralph fix all the failing tests
-> Won't stop until all tests pass

You: design the auth system
-> Technical spec with interfaces, decisions, and acceptance criteria
```

## Architecture

### Shared Core with Platform Adapters

AIDE uses a layered architecture where platform-agnostic logic lives in a shared core, and thin adapters map each AI assistant's lifecycle events to core functions.

```
+------------------------------------------------------------------+
|  AI Assistant (Claude Code / OpenCode / ...)                     |
+------------------------------------------------------------------+
          |                              |
  +----------------+            +----------------+
  | Claude Code    |            | OpenCode       |
  | Hooks          |            | Plugin         |
  | (src/hooks/)   |            | (src/opencode/)|
  +----------------+            +----------------+
          |                              |
  +--------------------------------------------------+
  |  Shared Core (src/core/)                         |
  |  session-init, skill-matcher, tool-tracking,     |
  |  tool-enforcement, persistence-logic,            |
  |  session-summary, mcp-sync, write-guard,         |
  |  comment-checker, ...                            |
  +--------------------------------------------------+
          |
  +--------------------------------------------------+
  |  aide binary (Go)                                |
  |  MCP server, BBolt storage, Bleve search,        |
  |  tree-sitter code indexing, gRPC multiplexing    |
  +--------------------------------------------------+
```

### MCP Reads, Hooks Write

AIDE follows a clear separation of concerns:

| Component     | Purpose                                     | Examples                                                    |
| ------------- | ------------------------------------------- | ----------------------------------------------------------- |
| **MCP Tools** | Read state, search, inter-agent messaging   | `memory_search`, `code_search`, `state_get`, `message_send` |
| **Hooks**     | Write state, detect keywords, enforce rules | Session start, keyword detection, tool enforcement          |

This prevents conflicts - the LLM reads via MCP, hooks update state in response to lifecycle events.

### Cross-Assistant MCP Sync

MCP server configurations are automatically synchronized across assistants. When aide initializes on any platform, it reads MCP configs from Claude Code (`.mcp.json`), OpenCode (`opencode.json`), and the aide canonical format (`.aide/config/mcp.json`), merges them, and writes back. A journal tracks intentional deletions so removed servers aren't re-imported.

### Hooks

| Hook                 | Trigger                   | Purpose                                               |
| -------------------- | ------------------------- | ----------------------------------------------------- |
| `SessionStart`       | New conversation          | Initialize state, inject memories                     |
| `SessionEnd`         | Session closes cleanly    | Cleanup session state, record metrics                 |
| `UserPromptSubmit`   | User sends message        | Inject matching skills (fuzzy trigger matching)       |
| `PreToolUse`         | Before tool execution     | Track tools, enforce agent roles, write guard         |
| `PostToolUse`        | After any tool            | Update HUD, check for AI comment bloat                |
| `SubagentStart/Stop` | Agent lifecycle           | Track active agents, inject memories                  |
| `Stop`               | Conversation ends         | Persist state, capture session summary, agent cleanup |
| `PreCompact`         | Before context compaction | Preserve state snapshot before summarization          |

### MCP Tools

| Tool               | Purpose                            |
| ------------------ | ---------------------------------- |
| `memory_search`    | Full-text search memories (Bleve)  |
| `memory_list`      | List memories by category/tags     |
| `code_search`      | Search code symbols                |
| `code_symbols`     | List symbols in a file             |
| `code_references`  | Find call sites for a symbol       |
| `code_stats`       | Index statistics                   |
| `state_get`        | Get session/agent state            |
| `state_list`       | List all state values              |
| `decision_get`     | Get latest decision for topic      |
| `decision_list`    | List all decisions                 |
| `decision_history` | Full history for a topic           |
| `message_list`     | List inter-agent messages          |
| `message_send`     | Send message to agent or broadcast |
| `message_ack`      | Acknowledge a received message     |

### Quality Guards

AIDE includes automatic quality controls that run via hooks:

- **Write Guard** - Prevents the `Write` tool from overwriting existing files, forcing `Edit` instead to reduce accidental file clobbers
- **Comment Checker** - Detects excessive AI-generated comments in code output and flags them
- **Tool Enforcement** - In swarm mode, restricts read-only agents (architect, explorer, researcher) from using write tools

## Platform Comparison

| Feature             | Claude Code             | OpenCode                            |
| ------------------- | ----------------------- | ----------------------------------- |
| Memory & decisions  | Full                    | Full                                |
| Code indexing       | Full                    | Full                                |
| Skill injection     | Via hooks               | Via system prompt transform         |
| Swarm mode          | Native (subagent hooks) | Multi-instance workarounds          |
| HUD / status line   | Native                  | Fallback to `.aide/state/hud.txt`   |
| Persistence (ralph) | Stop-blocking           | Re-prompting via `session.prompt()` |
| Subagent lifecycle  | Full hooks              | Session-based tracking              |
| Write guard         | Full                    | Full                                |
| MCP sync            | Full                    | Full                                |

See the [OpenCode adapter docs](adapters/opencode/README.md) for detailed integration notes and multi-instance orchestration patterns.

## Memory System

Memories are organized by **category** (`learning`, `decision`, `session`, `pattern`, `gotcha`, `discovery`, `blocker`, `issue`) and **tags**.

### Tagging and Scoping

Tags control when and where memories are injected. The key scoping tags:

| Tag              | Scope                      | Behavior                                                                |
| ---------------- | -------------------------- | ----------------------------------------------------------------------- |
| `scope:global`   | All projects, all sessions | Injected at every session start and subagent spawn, across all projects |
| `project:<name>` | Single project             | Injected only when working in the named project                         |
| `session:<id>`   | Single session             | Groups memories from the same session for recent-session summaries      |

At session start, aide fetches:

1. All memories tagged `scope:global` (your cross-project preferences and patterns)
2. All memories tagged `project:<current-project>` (project-specific context)
3. Recent session groups (memories sharing a `session:*` tag within the current project)

Memories without scope tags are still searchable via `memory_search` but won't be auto-injected.

**Storing:** Use natural language ("remember this...") or `/aide:memorise`

**Recalling:** Ask questions ("what did we decide...") or `/aide:recall`

**Search:** Full-text with fuzzy matching, typo tolerance, and prefix search.

### Automatic Injection

Memories are automatically injected into context at specific lifecycle events:

| Event              | What Gets Injected                                                             |
| ------------------ | ------------------------------------------------------------------------------ |
| Session Start      | Global memories, project memories, project decisions, recent session summaries |
| Subagent Spawn     | Global memories, project memories, project decisions                           |
| Context Compaction | State snapshot to preserve across summarization                                |

### Automatic Capture

Session summaries are automatically captured when a session ends with meaningful activity (files modified, tools used, or git commits made).

### Storing Memories

Use the `/aide:memorise` skill or the CLI directly:

```bash
aide memory add --category=learning --tags=testing,vitest "User prefers vitest over jest for testing"
```

**Valid categories:** `learning`, `decision`, `session`, `pattern`, `gotcha`, `discovery`, `blocker`, `issue`

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
+--------------------------------------------------------------------------+
|  ORCHESTRATOR                                                            |
|  1. Decompose work into stories                                          |
|  2. Create worktree per story                                            |
|  3. Spawn story agent per worktree                                       |
|  4. Monitor progress                                                     |
+--------------------------------------------------------------------------+
                              |
       +----------------------+----------------------+
       v                      v                      v
+-----------------+   +-----------------+   +-----------------+
| Story A Agent   |   | Story B Agent   |   | Story C Agent   |
| (worktree-a)    |   | (worktree-b)    |   | (worktree-c)    |
+-----------------+   +-----------------+   +-----------------+
| SDLC Pipeline:  |   | SDLC Pipeline:  |   | SDLC Pipeline:  |
| [DESIGN]        |   | [DESIGN]        |   | [DESIGN]        |
| [TEST]          |   | [TEST]          |   | [TEST]          |
| [DEV]           |   | [DEV]           |   | [DEV]           |
| [VERIFY]        |   | [VERIFY]        |   | [VERIFY]        |
| [DOCS]          |   | [DOCS]          |   | [DOCS]          |
+-----------------+   +-----------------+   +-----------------+
```

### SDLC Stages

Each story agent executes these stages in order:

| Stage  | Skill             | Purpose                                         |
| ------ | ----------------- | ----------------------------------------------- |
| DESIGN | `/aide:design`    | Technical spec, interfaces, acceptance criteria |
| TEST   | `/aide:test`      | Write failing tests (TDD)                       |
| DEV    | `/aide:implement` | Make tests pass                                 |
| VERIFY | `/aide:verify`    | Full QA validation                              |
| DOCS   | `/aide:docs`      | Update documentation                            |

### Activation

```
swarm 3                              # 3 story agents (SDLC mode)
swarm stories "Auth" "Payments"      # Named stories
swarm 2 --flat                       # Flat task mode (legacy)
```

### Workflow

1. **Plan** (optional) - Use `/aide:plan-swarm` to decompose work into validated stories
2. **Decompose** - Break work into independent stories
3. **Create worktrees** - Each story gets isolated `feat/<story>` branch
4. **Spawn agents** - Each agent manages its own SDLC pipeline
5. **Track progress** - Monitor via tasks and messages
6. **Merge** - Use `/aide:worktree-resolve` when all stories complete

### Coordination

Agents share state via:

- **Decisions** - `aide decision set/get` for architectural choices
- **Memory** - `aide memory add` for discoveries and learnings
- **Messages** - `aide message send` for direct inter-agent communication

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

Skills are markdown files that inject context when triggered by keywords. Trigger matching uses fuzzy matching with Levenshtein distance for typo tolerance.

### Built-in Skills

| Skill                | Example Prompt                    | What Happens                                                    |
| -------------------- | --------------------------------- | --------------------------------------------------------------- |
| **swarm**            | `swarm 3 implement dashboard`     | Spawns N parallel agents with SDLC pipeline per story           |
| **plan-swarm**       | `plan the dashboard work`         | Decomposes work into validated stories for swarm execution      |
| **decide**           | `help me decide on auth strategy` | Formal decision-making interview, records architectural choices |
| **design**           | `design the auth system`          | Technical spec with interfaces, decisions, acceptance criteria  |
| **test**             | `write tests for auth`            | Creates test suite with coverage verification                   |
| **implement**        | `implement the feature`           | TDD implementation - make failing tests pass                    |
| **verify**           | `verify the implementation`       | Full QA: tests, lint, types, debug artifact check               |
| **docs**             | `update the documentation`        | Updates docs to match implementation                            |
| **ralph**            | `ralph fix all failing tests`     | Won't stop until verified complete                              |
| **build-fix**        | `fix the build errors`            | Iteratively fixes build/lint/type errors until clean            |
| **debug**            | `debug why login fails`           | Systematic debugging with hypothesis testing                    |
| **perf**             | `optimize the API`                | Performance profiling and optimization workflow                 |
| **review**           | `review this PR`                  | Security-focused code review                                    |
| **code-search**      | `find all auth functions`         | Search code symbols and find call sites                         |
| **memorise**         | `remember I prefer vitest`        | Stores info for future sessions                                 |
| **recall**           | `what testing framework?`         | Searches memories and decisions                                 |
| **git**              | `help with git rebase`            | Expert git operations                                           |
| **worktree-resolve** | `merge the swarm branches`        | Intelligently merges worktrees with conflict resolution         |

### Skill Discovery

Skills are discovered from multiple locations with priority (highest first):

1. Project-local `.aide/skills/` - Project-specific overrides
2. Project-local `skills/` - Alternative project location
3. Plugin-bundled `skills/` - Ships with aide
4. Global `~/.aide/skills/` - User-wide custom skills

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

Skills are auto-discovered and hot-reloaded.

## CLI Reference

Run `aide --help` for full CLI documentation. Key commands:

```bash
aide memory add/search/list/delete   # Memory management
aide decision set/get/list/history   # Architectural decisions
aide state set/get/list              # Session/agent state
aide message send/list/ack           # Inter-agent messaging
aide code index/search/symbols       # Code indexing
aide session init/cleanup            # Session lifecycle
aide upgrade                         # Self-upgrade binary
```

## Configuration

Key environment variables:

| Variable                    | Description                                   |
| --------------------------- | --------------------------------------------- |
| `AIDE_DEBUG=1`              | Enable debug logging (logs to `.aide/_logs/`) |
| `AIDE_CODE_WATCH=1`         | Enable file watching for auto-reindex         |
| `AIDE_CODE_WATCH_DELAY=30s` | Delay before re-indexing after file changes   |
| `AIDE_MEMORY_INJECT=0`      | Disable memory injection                      |

## Status Line (Claude Code)

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
[aide(0.0.32)] mode:idle | 12m | tasks:done(6) wip(0) todo(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status, 5-hour token usage, and estimated time to rate limit.

## Storage

All data stored in `.aide/` (per-project). A `.aide/.gitignore` is automatically created on first session.

**Explicitly gitignored** (machine-local runtime data):

| Path                           | Purpose                                                     |
| ------------------------------ | ----------------------------------------------------------- |
| `memory/`                      | BBolt database + Bleve search index (binary, non-mergeable) |
| `state/`                       | Runtime state (HUD output, session info, worktree tracking) |
| `bin/`                         | Downloaded aide binary                                      |
| `code/`                        | Tree-sitter code symbol index                               |
| `worktrees/`                   | Git worktree directories for swarm mode                     |
| `_logs/`                       | Debug logs (when `AIDE_DEBUG=1`)                            |
| `config/mcp.json`              | Canonical MCP server config (machine-specific sync state)   |
| `config/mcp-sync.journal.json` | Tracks intentional MCP server removals for sync             |

**Explicitly un-ignored** (`!shared/` in `.gitignore`):

| Path                | Purpose                                               |
| ------------------- | ----------------------------------------------------- |
| `shared/decisions/` | Exported decisions as markdown (one file per topic)   |
| `shared/memories/`  | Exported memories as markdown (one file per category) |

The `shared/` directory is created when you run `aide share export`. Files use YAML frontmatter + markdown body, so they work as LLM context even without aide installed. This is the recommended way to share architectural decisions and project learnings via git.

**Not gitignored** (tracked by default, committing is optional):

| Path               | Purpose                                                     | Notes                                                                  |
| ------------------ | ----------------------------------------------------------- | ---------------------------------------------------------------------- |
| `config/aide.json` | Project-level aide config (e.g. auto-import settings)       | Only useful if you've customized settings                              |
| `skills/`          | Project-specific custom skills (highest discovery priority) | Commit if you add custom skills; they are auto-discovered and injected |

Use `aide share export` to populate `shared/` and `aide share import` to load it into the local store (or set `AIDE_SHARE_AUTO_IMPORT=1` for automatic import on session start).

```bash
aide share export                    # Export decisions + memories to .aide/shared/
aide share export --decisions        # Decisions only
aide share import                    # Import from .aide/shared/
aide share import --dry-run          # Preview what would be imported
```

The `aide` binary is bundled with the plugin and automatically downloaded on install/upgrade.

## Troubleshooting

```bash
# Check if binary exists and works
aide version

# Claude Code: reinstall plugin
claude plugin uninstall aide
claude plugin install aide@aide

# OpenCode: reinstall
bunx @jmylchreest/aide-plugin uninstall
bunx @jmylchreest/aide-plugin install

# Enable debug logging
AIDE_DEBUG=1 claude    # or AIDE_DEBUG=1 opencode

# Check memories
aide memory list

# Check MCP sync status
cat .aide/config/mcp.json
```

## Adding Support for New Assistants

AIDE's adapter architecture makes it straightforward to add support for new AI coding tools. See the [adapters documentation](adapters/README.md) for the template:

1. Create `adapters/<tool>/generate.ts`
2. Map the tool's lifecycle events to aide core functions
3. Generate config files for the tool's plugin format
4. Skills and MCP tools work out of the box

## License

MIT
