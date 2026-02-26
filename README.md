# AIDE - AI Development Environment

Multi-agent orchestration, persistent memory, and intelligent workflows for AI coding assistants.

Supports **Claude Code** and **OpenCode** through a shared core with platform-specific adapters.

| Without AIDE                  | With AIDE                          |
| ----------------------------- | ---------------------------------- |
| Context lost between sessions | Memories persist and auto-inject   |
| Manual task coordination      | Swarm mode with parallel agents    |
| Repeated setup instructions   | Skills activate by keyword         |
| No code search                | Fast symbol search across codebase |
| No code quality analysis      | Static analysis with 4 analysers   |
| Decisions forgotten           | Decisions recorded and enforced    |

## Quick Start

### Claude Code

```bash
claude plugin marketplace add jmylchreest/aide
claude plugin install aide@aide
```

### OpenCode

```bash
bunx @jmylchreest/aide-plugin install
```

This registers the aide plugin and MCP server. Skills become available as `/aide:*` slash commands.

## Installation

### Claude Code - From Marketplace (Recommended)

```bash
claude plugin marketplace add jmylchreest/aide
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

This modifies your `opencode.json` to register the aide plugin and MCP server. Check status with `bunx @jmylchreest/aide-plugin status`, uninstall with `bunx @jmylchreest/aide-plugin uninstall`.

### From Source

```bash
git clone https://github.com/jmylchreest/aide && cd aide

# Build (requires Go 1.21+)
cd aide && go build -o ../bin/aide ./cmd/aide && cd ..
npm install && npm run build

# Claude Code
claude --plugin-dir /path/to/aide

# OpenCode
bunx @jmylchreest/aide-plugin install --plugin-path /path/to/aide
```

The `aide` Go binary is automatically downloaded when the plugin is installed. No separate binary installation needed.

### Permissions (Claude Code)

Add to `~/.claude/settings.json`:

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

## Features

### Memory & Decisions

Memories persist across sessions and auto-inject at session start. Decisions record architectural choices that are enforced across all agents.

```
You: remember that I prefer vitest for testing
# Next session...
You: what testing framework should I use?
AI: Based on your preferences, you prefer vitest for testing.
```

Use `/aide:memorise` to store, `/aide:recall` to search, `/aide:forget` to clean up, `/aide:decide` for formal architectural decisions. See [Memory System](docs/reference.md#memory-system) for tagging, scoping, and injection details.

### Code Indexing

Fast symbol search using tree-sitter. Supports TypeScript, JavaScript, Go, Python, Rust, and more.

```bash
aide code index              # Index codebase
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
```

Set `AIDE_CODE_WATCH=1` for automatic re-indexing on file changes. Use `.aideignore` (gitignore syntax) to exclude files from indexing and analysis.

### Static Analysis

4 built-in analysers detect code quality issues without external tools:

| Analyser     | Detects                              |
| ------------ | ------------------------------------ |
| `complexity` | High cyclomatic complexity functions |
| `coupling`   | High fan-in/fan-out, import cycles   |
| `secrets`    | Hardcoded API keys, tokens           |
| `clones`     | Duplicated code blocks               |

```bash
aide findings run                         # Run all analysers
aide findings stats                       # Health overview
aide findings list --severity=critical    # Critical findings
```

Findings are searchable via MCP tools during code review and debugging. The file watcher auto-runs analysers on changed files. Use `/aide:patterns` to activate findings-based analysis.

### Status Dashboard

```bash
aide status          # Server, watcher, index, findings, stores, env
aide status --json   # Machine-readable output
```

### Modes

```
swarm 3 implement the dashboard   # Spawns 3 parallel agents with SDLC pipelines
ralph fix all the failing tests   # Won't stop until all tests pass
design the auth system            # Technical spec with interfaces and decisions
```

See [Swarm Mode](docs/reference.md#swarm-mode) for SDLC pipeline details, worktree management, and agent coordination.

## Skills

Skills are markdown files that inject context when triggered by keywords. Trigger matching uses fuzzy matching for typo tolerance.

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
| **patterns**         | `find patterns`, `code health`    | Analyze codebase patterns using static analysis findings        |
| **code-search**      | `find all auth functions`         | Search code symbols and find call sites                         |
| **memorise**         | `remember I prefer vitest`        | Stores info for future sessions                                 |
| **recall**           | `what testing framework?`         | Searches memories and decisions                                 |
| **forget**           | `forget the old auth decision`    | Soft-delete or hard-delete outdated memories                    |
| **git**              | `help with git rebase`            | Expert git operations and worktree management                   |
| **worktree-resolve** | `merge the swarm branches`        | Intelligently merges worktrees with conflict resolution         |

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

Skills are auto-discovered from: `.aide/skills/` (project) > `skills/` (project) > plugin-bundled > `~/.aide/skills/` (global). Skills are hot-reloaded on changes.

## Configuration

| Variable                    | Description                                    |
| --------------------------- | ---------------------------------------------- |
| `AIDE_DEBUG=1`              | Enable debug logging (logs to `.aide/_logs/`)  |
| `AIDE_FORCE_INIT=1`         | Force initialization in non-git directories    |
| `AIDE_CODE_WATCH=1`         | Enable file watching for auto-reindex          |
| `AIDE_CODE_WATCH_DELAY=30s` | Delay before re-indexing after file changes    |
| `AIDE_MEMORY_INJECT=0`      | Disable memory injection                       |
| `AIDE_SHARE_AUTO_IMPORT=1`  | Auto-import shared decisions/memories on start |

## Reference Documentation

For detailed documentation on all subsystems, see **[docs/reference.md](docs/reference.md)**:

- [Architecture](docs/reference.md#architecture) - Layered design, hooks, MCP read/write separation
- [All 18 MCP Tools](docs/reference.md#mcp-tools) - Memory, decisions, state, messaging, code, findings
- [CLI Reference](docs/reference.md#cli-reference) - Full command reference
- [Memory System](docs/reference.md#memory-system) - Tagging, scoping, auto-injection, decisions
- [Code Indexing](docs/reference.md#code-indexing) - Tree-sitter, file watcher, .aideignore
- [Static Analysis](docs/reference.md#static-analysis-findings) - 4 analysers, MCP integration, auto-run
- [Swarm Mode](docs/reference.md#swarm-mode) - SDLC pipeline, worktrees, coordination
- [Storage](docs/reference.md#storage) - File layout, gitignore rules, sharing via git
- [Status Dashboard](docs/reference.md#status-dashboard) - `aide status` output details
- [Platform Comparison](docs/reference.md#platform-comparison) - Claude Code vs OpenCode capabilities
- [Quality Guards](docs/reference.md#quality-guards) - Write guard, comment checker, tool enforcement
- [Hooks](docs/reference.md#hooks) - Lifecycle events and their purposes

## Troubleshooting

```bash
aide version                              # Check binary
aide status                               # Full system dashboard
AIDE_DEBUG=1 claude                       # Debug logging (or AIDE_DEBUG=1 opencode)
```

**Reinstall:**

```bash
# Claude Code
claude plugin uninstall aide && claude plugin install aide@aide

# OpenCode
bunx @jmylchreest/aide-plugin uninstall && bunx @jmylchreest/aide-plugin install
```

## Adding Support for New Assistants

See the [adapters documentation](adapters/README.md). AIDE's adapter architecture maps any tool's lifecycle events to shared core functions — skills and MCP tools work out of the box.

## License

MIT
