# AIDE - AI Development Environment

Persistent memory, code intelligence, and multi-agent orchestration for AI coding assistants. Works with **Claude Code** and **OpenCode**.

## Install

**Claude Code:**

```bash
claude plugin marketplace add jmylchreest/aide
claude plugin install aide@aide
```

**OpenCode:**

```bash
bunx @jmylchreest/aide-plugin install
```

The Go binary downloads automatically. Skills become available immediately.

## What You Get

| Capability          | What it does                                                                     |
| ------------------- | -------------------------------------------------------------------------------- |
| **Memory**          | Remembers preferences and context across sessions                                |
| **Decisions**       | Records architectural choices, enforces them in every session                    |
| **Code Index**      | Fast symbol search, call graphs, and references via tree-sitter                  |
| **Static Analysis** | Detects complexity, coupling, secrets, and code duplication                      |
| **Survey**          | Maps codebase structure: modules, entry points, tech stack, churn hotspots       |
| **Skills**          | 23 built-in workflows triggered by natural language                              |
| **Swarm**           | Parallel agents with full SDLC pipelines (design, test, implement, verify, docs) |
| **32 MCP Tools**    | Full programmatic access to all capabilities above                               |

## Get Started

### Existing project — understand the codebase:

```
Survey this codebase and help me understand its structure.
```

AIDE indexes symbols, discovers modules, tech stack, entry points, and git churn hotspots, then presents the big picture. [Full guide](docs/docs/getting-started/existing-project.md)

### New project — set up guardrails:

```
Help me decide on the coding standards, error handling strategy, testing approach,
and architecture patterns for this project. I want to enforce SOLID, DRY, Clean Code,
and idiomatic language best practices.
```

The decide skill works through each topic in turn, recording separate decisions that persist across every future session. [Full guide](docs/docs/getting-started/new-project.md)

## Skills

Skills are markdown workflows triggered by keywords. Type naturally — trigger matching is fuzzy.

| Skill                | Example Prompt                          | What Happens                                           |
| -------------------- | --------------------------------------- | ------------------------------------------------------ |
| **swarm**            | `swarm 3 implement dashboard`           | Parallel agents with SDLC pipeline per story           |
| **plan-swarm**       | `plan swarm for the dashboard`          | Decomposes work into stories for swarm execution       |
| **decide**           | `help me decide on auth strategy`       | Structured decision interview, records choices         |
| **design**           | `design the auth system`                | Technical spec with interfaces and acceptance criteria |
| **survey**           | `survey this codebase`                  | Maps modules, tech stack, entry points, and churn      |
| **test**             | `write tests for auth`                  | Test suite with coverage verification                  |
| **implement**        | `implement the feature`                 | TDD — make failing tests pass                          |
| **verify**           | `verify the implementation`             | Full QA: tests, lint, types, build, debug artifacts    |
| **docs**             | `update the documentation`              | Updates docs to match implementation                   |
| **autopilot**        | `autopilot fix all failing tests`       | Persistent — won't stop until verified complete        |
| **build-fix**        | `fix the build errors`                  | Iteratively fixes build/lint/type errors               |
| **debug**            | `debug why login fails`                 | Systematic debugging with hypothesis testing           |
| **perf**             | `optimize the API`                      | Performance profiling and optimization                 |
| **review**           | `review this PR`                        | Security-focused code review                           |
| **patterns**         | `check code health`                     | Surface code quality issues via static analysis        |
| **assess-findings**  | `assess findings`                       | Triage: read code, accept noise, keep real issues      |
| **code-search**      | `find all auth functions`               | Search symbols, find call sites                        |
| **memorise**         | `remember I prefer vitest`              | Stores info for future sessions                        |
| **recall**           | `do you remember the testing decision?` | Searches memories and decisions                        |
| **forget**           | `forget the old auth decision`          | Soft-delete or hard-delete memories                    |
| **git**              | `create a worktree for this feature`    | Git operations and worktree management                 |
| **worktree-resolve** | `merge worktrees`                       | Merges worktree branches with conflict resolution      |
| **context-usage**    | `how much context am I using?`          | Analyze session context and token usage                |

**Custom skills:** Create `.aide/skills/my-skill.md` with YAML frontmatter (`name`, `triggers`) and markdown body. Auto-discovered from `.aide/skills/` > `skills/` > plugin-bundled > `~/.aide/skills/`.

## Configuration

| Variable                         | Description                                      |
| -------------------------------- | ------------------------------------------------ |
| `AIDE_DEBUG=1`                   | Enable debug logging (logs to `.aide/_logs/`)    |
| `AIDE_FORCE_INIT=1`              | Force initialization in non-git directories      |
| `AIDE_CODE_WATCH=1`              | Enable file watching for auto-reindex            |
| `AIDE_CODE_WATCH_DELAY=30s`      | Delay before re-indexing after file changes      |
| `AIDE_MEMORY_INJECT=0`           | Disable memory injection                         |
| `AIDE_MEMORY_SCORING_DISABLED=1` | Disable memory scoring (use chronological order) |
| `AIDE_MEMORY_DECAY_DISABLED=1`   | Disable recency decay in memory scoring          |
| `AIDE_SHARE_AUTO_IMPORT=1`       | Auto-import shared decisions/memories on start   |

## CLI Reference

```bash
aide status                               # System dashboard
aide code index                           # Index codebase symbols
aide code search "getUser"                # Search symbols
aide survey run                           # Map codebase structure
aide findings run all                     # Run all static analysers
aide findings stats                       # Health overview
aide findings list --severity=critical    # View critical findings
aide version                              # Check binary version
```

## Documentation

Full documentation: **[jmylchreest.github.io/aide](https://jmylchreest.github.io/aide/)**

- [Architecture](https://jmylchreest.github.io/aide/docs/reference/architecture) — Layered design, hooks, MCP read/write separation
- [MCP Tools](https://jmylchreest.github.io/aide/docs/reference/mcp-tools) — All 32 tools: memory, decisions, code, findings, survey, tasks
- [CLI Reference](https://jmylchreest.github.io/aide/docs/reference/cli) — Full command reference
- [Swarm Mode](https://jmylchreest.github.io/aide/docs/modes/swarm) — SDLC pipeline, worktrees, agent coordination
- [Skills](https://jmylchreest.github.io/aide/docs/skills) — Built-in and custom skill reference
- [Storage](https://jmylchreest.github.io/aide/docs/reference/storage) — File layout, sharing via git

## Advanced Installation

### Claude Code — Marketplace for Teams

Register the marketplace in `~/.claude/settings.json` (or `.claude/settings.json` for project-level) so team members are prompted to install:

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

### Claude Code — Permissions

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

### From Source

```bash
git clone https://github.com/jmylchreest/aide && cd aide

# Build (requires Go 1.25+)
cd aide && go build -o ../bin/aide ./cmd/aide && cd ..
npm install && npm run build

# Claude Code
claude --plugin-dir /path/to/aide

# OpenCode
bunx @jmylchreest/aide-plugin install --project
```

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
