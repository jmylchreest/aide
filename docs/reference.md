# AIDE Reference

Detailed documentation for AIDE's subsystems. For a quick overview, see the [main README](../README.md).

## Table of Contents

- [Architecture](#architecture)
- [MCP Tools](#mcp-tools)
- [CLI Reference](#cli-reference)
- [Memory System](#memory-system)
- [Code Indexing](#code-indexing)
- [Static Analysis (Findings)](#static-analysis-findings)
- [Swarm Mode](#swarm-mode)
- [Skills](#skills)
- [Storage](#storage)
- [Status Dashboard](#status-dashboard)
- [Platform Comparison](#platform-comparison)
- [Status Line (Claude Code)](#status-line-claude-code)
- [Quality Guards](#quality-guards)
- [Hooks](#hooks)
- [Adding Support for New Assistants](#adding-support-for-new-assistants)

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
  |  tree-sitter code indexing, static analysis,     |
  |  file watcher, gRPC multiplexing                 |
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

## MCP Tools

AIDE exposes 25 MCP tools organized into 7 groups. All tools are prefixed `aide__` (e.g., `aide__memory_search`).

### Memory Tools

| Tool            | Purpose                                        |
| --------------- | ---------------------------------------------- |
| `memory_search` | Full-text fuzzy search across memories         |
| `memory_list`   | List memories, optionally filtered by category |

### Decision Tools

| Tool               | Purpose                                |
| ------------------ | -------------------------------------- |
| `decision_get`     | Get the current decision for a topic   |
| `decision_list`    | List all recorded decisions            |
| `decision_history` | Full chronological history for a topic |

### State Tools

| Tool         | Purpose                                      |
| ------------ | -------------------------------------------- |
| `state_get`  | Get a state value (global or per-agent)      |
| `state_list` | List all state values (global and per-agent) |

### Message Tools

| Tool           | Purpose                                          |
| -------------- | ------------------------------------------------ |
| `message_send` | Send a message to another agent or broadcast     |
| `message_list` | List messages for an agent (auto-prunes expired) |
| `message_ack`  | Acknowledge a message as read                    |

### Code Tools

| Tool              | Purpose                                                       |
| ----------------- | ------------------------------------------------------------- |
| `code_search`     | Search indexed symbol definitions (functions, classes, types) |
| `code_symbols`    | List all symbols defined in a specific file                   |
| `code_references` | Find all call sites and usages of a symbol                    |
| `code_stats`      | Get index statistics (files, symbols, references)             |
| `code_outline`    | Get collapsed file outline with signatures and line numbers   |

### Findings Tools

| Tool              | Purpose                                                         |
| ----------------- | --------------------------------------------------------------- |
| `findings_search` | Full-text search across static analysis findings                |
| `findings_list`   | List findings filtered by analyser, severity, file, or category |
| `findings_stats`  | Codebase health overview with counts by analyser and severity   |
| `findings_accept` | Accept (dismiss) findings by ID or filter — hides from output   |

### Task Tools

| Tool            | Purpose                                        |
| --------------- | ---------------------------------------------- |
| `task_create`   | Create a new swarm task (starts as pending)    |
| `task_get`      | Get full task details by ID                    |
| `task_list`     | List tasks, optionally filtered by status      |
| `task_claim`    | Atomically claim a pending task for your agent |
| `task_complete` | Mark a task as done with a result summary      |
| `task_delete`   | Delete a task by ID                            |

## CLI Reference

### Memory

```bash
aide memory add --category=learning --tags=testing "Prefers vitest"
aide memory search "authentication"
aide memory list --category=learning
aide memory delete <id>
aide memory reindex                      # Rebuild search index
aide memory export --format=markdown     # Export to markdown
```

### Decisions

```bash
aide decision set auth-strategy "JWT with refresh tokens" --rationale="Stateless"
aide decision get auth-strategy
aide decision list
aide decision history auth-strategy
aide decision delete auth-strategy
```

### Tasks

```bash
aide task create "Implement user model" --description="Create User struct"
aide task claim <id> --agent=executor-1
aide task complete <id> --result="Done"
aide task list --status=pending
```

### Messages

```bash
aide message send "User model ready" --from=executor-1
aide message send "Can you review?" --from=executor-2 --to=executor-1
aide message list --agent=executor-1
aide message ack <id> --agent=executor-1
```

### State

```bash
aide state set mode ralph
aide state set mode eco --agent=worker-1
aide state get mode --agent=worker-1
aide state list
aide state clear --agent=worker-1
```

### Code

```bash
aide code index                          # Index codebase (incremental)
aide code search "getUser"               # Search symbols
aide code symbols src/auth.ts            # List file symbols
aide code references getUserById         # Find call sites
aide code stats                          # Index statistics
aide code clear                          # Clear index
```

### Findings

```bash
aide findings run                         # Run all analysers
aide findings run --analyser=complexity   # Run specific analyser
aide findings search "high complexity"    # Search findings
aide findings list --severity=critical    # List by severity
aide findings list --file=src/auth        # List by file
aide findings stats                       # Health overview
aide findings accept <id1> <id2>          # Accept specific findings by ID
aide findings accept --analyzer=clones    # Accept all clone findings
aide findings accept --all                # Accept all findings
aide findings clear                       # Clear all findings
```

### Share

```bash
aide share export                        # Export decisions + memories to .aide/shared/
aide share export --decisions            # Decisions only
aide share import                        # Import from .aide/shared/
aide share import --dry-run              # Preview import
```

### Status

```bash
aide status                              # Full dashboard
aide status --json                       # JSON output
```

### Other

```bash
aide session init                        # Initialize session
aide upgrade                             # Self-upgrade binary
aide daemon --socket=/path/to/aide.sock  # Start gRPC daemon
aide mcp                                 # Start MCP server
```

## Memory System

### Categories and Tags

Memories are organized by **category** (`learning`, `decision`, `session`, `pattern`, `gotcha`, `discovery`, `blocker`, `issue`) and **tags**.

### Scoping Tags

Tags control when and where memories are injected:

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

### Automatic Injection

| Event              | What Gets Injected                                                             |
| ------------------ | ------------------------------------------------------------------------------ |
| Session Start      | Global memories, project memories, project decisions, recent session summaries |
| Subagent Spawn     | Global memories, project memories, project decisions                           |
| Context Compaction | State snapshot to preserve across summarization                                |

### Automatic Capture

Session summaries are automatically captured when a session ends with meaningful activity (files modified, tools used, or git commits made).

### Decision System

Decisions are a specialized memory type for architectural choices that need to be enforced.

```bash
aide decision set "auth-strategy" "JWT with refresh tokens" --rationale="Stateless, mobile-friendly"
aide decision get "auth-strategy"     # Latest decision
aide decision list                    # All decisions
aide decision history "auth-strategy" # Full history
```

When a new decision is set for an existing topic, it supersedes the old one. The history is preserved, but only the latest decision is injected into context. All current project decisions are injected at session start and subagent spawn.

## Code Indexing

Fast symbol search using [tree-sitter](https://tree-sitter.github.io/). Supports TypeScript, JavaScript, Go, Python, Rust, and many more.

```bash
aide code index              # Index codebase
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
aide code references getUser   # Find call sites
```

### File Watching

When the MCP server is running, a file watcher automatically re-indexes changed files. Controlled by:

- `--code-watch` flag on `aide mcp`
- `AIDE_CODE_WATCH=1` environment variable
- `AIDE_CODE_WATCH_DELAY=30s` debounce delay (default 30s)

The watcher also triggers findings analysers on changed files.

### .aideignore

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax:

```gitignore
# Exclude generated code
*.pb.go
*_generated.go

# Exclude vendor
vendor/

# But include a specific vendor file
!vendor/important.go
```

Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Static Analysis (Findings)

AIDE includes 4 built-in static analysers that detect code quality issues without external tools.

### Analysers

| Analyser     | Detects                                       | Severities        |
| ------------ | --------------------------------------------- | ----------------- |
| `complexity` | High cyclomatic complexity functions          | warning, critical |
| `coupling`   | High fan-in/fan-out, import cycles            | warning, critical |
| `secrets`    | Hardcoded API keys, tokens, passwords         | critical, warning |
| `clones`     | Duplicated code blocks (copy-paste detection) | warning, info     |

### Running Analysers

```bash
aide findings run                         # Run all analysers
aide findings run --analyser=complexity   # Run specific analyser
aide findings run --path=src/             # Scope to directory
```

Both `--analyser=` and `--analyzer=` are accepted on all commands.

### Querying Results

```bash
aide findings stats                      # Overview: counts by analyser and severity
aide findings list --severity=critical   # All critical findings
aide findings list --file=src/auth       # Findings in specific files
aide findings search "AWS"               # Search findings by keyword
```

### Accepting (Dismissing) Findings

Findings that are noise or irrelevant can be accepted (dismissed). Accepted findings are hidden from `list`, `search`, and `stats` output by default.

```bash
aide findings accept <id1> <id2>              # Accept specific findings by ID
aide findings accept --analyzer=clones        # Accept all clone findings
aide findings accept --severity=info          # Accept all info-severity findings
aide findings accept --file=cmd/              # Accept findings in a path
aide findings accept --all                    # Accept everything

aide findings list --include-accepted         # Show accepted findings too
aide findings stats --include-accepted        # Include accepted in counts
```

Use the `patterns` skill to analyse code health, then `assess-findings` to triage — the AI reads code for each finding and accepts noise automatically.

### Configuration

Analyser thresholds can be configured in `.aide/config/aide.json`:

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

Values in `aide.json` serve as project-level defaults. CLI flags (e.g. `--threshold=15`) override config file values. If neither is set, the built-in defaults above apply.

### MCP Integration

The findings are searchable via 3 MCP tools (`findings_search`, `findings_list`, `findings_stats`), making them available to the AI during code review, debugging, and refactoring. The `/aide:patterns` skill activates findings-based analysis.

### Auto-Analysis

When the file watcher is running (via `aide mcp`), findings are automatically re-run on changed files alongside code re-indexing. The watcher also reads thresholds from `aide.json`.

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

Use `/aide:worktree-resolve` to merge branches back to main, or manually:

```bash
git worktree list
git merge feat/story-auth
git worktree remove .aide/worktrees/story-auth
```

## Skills

Skills are markdown files that inject context when triggered by keywords. Trigger matching uses fuzzy matching with Levenshtein distance for typo tolerance.

### Built-in Skills

| Skill                | Triggers                          | Purpose                                                         |
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
| **patterns**         | `find patterns`, `code health`    | Analyze codebase patterns and surface static analysis findings  |
| **code-search**      | `find all auth functions`         | Search code symbols and find call sites                         |
| **memorise**         | `remember I prefer vitest`        | Stores info for future sessions                                 |
| **recall**           | `what testing framework?`         | Searches memories and decisions                                 |
| **forget**           | `forget the old auth decision`    | Soft-delete or hard-delete outdated memories                    |
| **git**              | `help with git rebase`            | Expert git operations and worktree management                   |
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

## Storage

All data stored in `.aide/` (per-project). A `.aide/.gitignore` is automatically created on first session.

### Gitignored (machine-local runtime data)

| Path                           | Purpose                                                     |
| ------------------------------ | ----------------------------------------------------------- |
| `memory/`                      | BBolt database + Bleve search index (binary, non-mergeable) |
| `state/`                       | Runtime state (HUD output, session info, worktree tracking) |
| `bin/`                         | Downloaded aide binary                                      |
| `worktrees/`                   | Git worktree directories for swarm mode                     |
| `_logs/`                       | Debug logs (when `AIDE_DEBUG=1`)                            |
| `config/mcp.json`              | Canonical MCP server config (machine-specific sync state)   |
| `config/mcp-sync.journal.json` | Tracks intentional MCP server removals for sync             |

### Shared via git (`!shared/` in `.gitignore`)

| Path                | Purpose                                               |
| ------------------- | ----------------------------------------------------- |
| `shared/decisions/` | Exported decisions as markdown (one file per topic)   |
| `shared/memories/`  | Exported memories as markdown (one file per category) |

The `shared/` directory is created when you run `aide share export`. Files use YAML frontmatter + markdown body, so they work as LLM context even without aide installed. This is the recommended way to share architectural decisions and project learnings via git.

### Tracked by default (committing optional)

| Path               | Purpose                        |
| ------------------ | ------------------------------ |
| `config/aide.json` | Project-level aide config      |
| `skills/`          | Project-specific custom skills |

### Store Files

```
.aide/memory/
├── memory.db             # Primary database (memories, decisions, state, tasks, messages)
├── search.bleve/         # Full-text search index for memories
├── code/
│   ├── index.db          # Code symbol database
│   └── search.bleve/     # Code symbol search index
└── findings/
    ├── findings.db       # Static analysis findings
    └── search.bleve/     # Findings search index
```

### Sharing Knowledge via Git

```bash
aide share export                    # Export decisions + memories to .aide/shared/
aide share export --decisions        # Decisions only
aide share import                    # Import from .aide/shared/
aide share import --dry-run          # Preview what would be imported
```

Set `AIDE_SHARE_AUTO_IMPORT=1` for automatic import on session start.

## Status Dashboard

`aide status` shows a comprehensive view of aide's internal state:

```bash
aide status          # Full dashboard
aide status --json   # Machine-readable JSON
```

Output includes:

- **Version and project info** - aide version, project root, current mode
- **Server** - whether MCP/gRPC server is running, uptime
- **File watcher** - watched paths, directory count, debounce delay, pending files
- **Code index** - files, symbols, and references indexed
- **Findings analysers** - per-analyser status, last run time, finding counts by severity
- **MCP tools** - all 18 tools with per-tool execution counts
- **Stores** - paths and sizes of all data files
- **Environment** - active `AIDE_*` environment variables

## Platform Comparison

| Feature             | Claude Code             | OpenCode                                          |
| ------------------- | ----------------------- | ------------------------------------------------- |
| Memory & decisions  | Full                    | Full                                              |
| Code indexing       | Full                    | Full                                              |
| Static analysis     | Full                    | Full                                              |
| Skill injection     | Via hooks               | Via system prompt transform + slash commands      |
| Swarm mode          | Native (subagent hooks) | Passive swarm-aware state; external orchestration |
| HUD / status line   | Native                  | Not supported (OpenCode TUI has no status line)   |
| Persistence (ralph) | Stop-blocking           | Re-prompting via `session.prompt()` on idle       |
| Subagent lifecycle  | Full hooks              | Session-based tracking (observational, no spawn)  |
| Write guard         | Full                    | Full                                              |
| MCP sync            | Full                    | Full                                              |

See the [OpenCode adapter docs](../adapters/opencode/README.md) for detailed integration notes and multi-instance orchestration patterns.

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
[aide(0.0.40)] mode:idle | 12m | tasks:done(6) wip(0) todo(0) | 5h:115K ~<1m
```

Shows: version, current mode, session duration, task status, 5-hour token usage, and estimated time to rate limit.

## Quality Guards

AIDE includes automatic quality controls that run via hooks:

- **Write Guard** - Prevents the `Write` tool from overwriting existing files, forcing `Edit` instead to reduce accidental file clobbers
- **Comment Checker** - Detects excessive AI-generated comments in code output and flags them
- **Tool Enforcement** - In swarm mode, restricts read-only agents (architect, explorer, researcher) from using write tools

## Hooks

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

## Adding Support for New Assistants

AIDE's adapter architecture makes it straightforward to add support for new AI coding tools. See the [adapters documentation](../adapters/README.md) for the template:

1. Create `adapters/<tool>/generate.ts`
2. Map the tool's lifecycle events to aide core functions
3. Generate config files for the tool's plugin format
4. Skills and MCP tools work out of the box
