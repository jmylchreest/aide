---
sidebar_position: 5
---

# Codebase Survey

Survey discovers **what a codebase is** -- its modules, tech stack, entry points, and change hotspots. It produces structural facts, distinct from [findings](./static-analysis.md) which detect code problems.

Survey data is stored in a separate BoltDB + Bleve database at `.aide/survey/` and cached until you re-run analyzers.

## Analyzers

Survey has 3 analyzers, each using a different data source:

| Analyzer      | Discovers                                         | Data Source                  |
| ------------- | ------------------------------------------------- | ---------------------------- |
| `topology`    | Modules, workspaces, build systems, tech stack    | Filesystem (project markers) |
| `entrypoints` | main() functions, HTTP handlers, CLI roots        | Code index + file scanning   |
| `churn`       | High-change files ranked by weighted commit score | Git history (go-git)         |

### Topology

Scans the filesystem for project markers defined in language packs and the per-topic partials under `packs/index.d/` (`languages.json`, `build-systems.json`, `ci-cd.json`, `containers.json`, `iac.json`, `monorepo.json`, `dev-tooling.json`, `docs.json`). See the [grammar](./grammar.md#project-marker-index) docs for the index layout. Detects:

- **Modules**: Go modules, npm packages, Cargo crates, Python packages, etc.
- **Workspaces**: Monorepo layouts (Nx, Lerna, Turborepo, pnpm workspaces)
- **Tech stack**: Build systems (Make, CMake, Bazel, Task), CI/CD (GitHub Actions, GitLab CI, Jenkins), IaC (Terraform, Pulumi, Ansible), container tools (Docker, Kubernetes/Helm), documentation frameworks, and more

### Entrypoints

Dual-mode detection:

1. **Code index mode** (preferred): Uses tree-sitter symbol data from language pack entrypoint definitions to find `main()` functions, HTTP handler mounts, gRPC services, CLI roots (Cobra, urfave/cli), etc.
2. **File scan fallback**: When the code index isn't available, falls back to regex-based content matching

### Churn

Uses go-git (no `git` binary required) to analyze commit history. Produces a ranked list of high-churn files by a weighted score: `commits * (1 + linesChanged/100)`. Also detects git submodules.

## Running Survey

```bash
aide survey run                          # Run all 3 analyzers
aide survey run --analyzer=topology      # Run specific analyzer
aide survey run --analyzer=churn         # Just git history analysis
```

Results are cached -- re-run to refresh after significant changes.

## Querying Results

```bash
aide survey stats                        # Overview: counts by analyzer and kind
aide survey list --kind=module           # All detected modules
aide survey list --kind=tech_stack       # Detected technologies
aide survey list --kind=entrypoint       # Where execution starts
aide survey list --kind=churn            # High-change files
aide survey search "auth"               # Full-text search across entries
aide survey clear                        # Clear all survey data
aide survey clear --analyzer=churn       # Clear specific analyzer data
```

### Entry Kinds

| Kind           | Description                                |
| -------------- | ------------------------------------------ |
| `module`       | Go module, npm package, Cargo crate, etc.  |
| `entrypoint`   | main() function, HTTP handler, CLI root    |
| `dependency`   | External dependency                        |
| `tech_stack`   | Detected technology, framework, or tool    |
| `churn`        | High-change file ranked by commit activity |
| `submodule`    | Git submodule                              |
| `workspace`    | Monorepo workspace root                    |
| `arch_pattern` | Architectural pattern                      |

## Call Graph

Survey includes a call graph feature that traces function relationships using the code index:

```bash
aide survey graph getUserById                  # Show callers and callees
aide survey graph --symbol=handleRequest \
    --direction=callers                        # Who calls this function?
aide survey graph --symbol=handleRequest \
    --direction=callees                        # What does this function call?
aide survey graph --symbol=main \
    --max-depth=3 --max-nodes=100              # Deeper traversal
```

The call graph is computed on demand via BFS over the code index (not stored). Requires `aide code index` to be run first.

## MCP Tools

5 survey MCP tools are available to the AI:

| Tool            | Purpose                                              |
| --------------- | ---------------------------------------------------- |
| `survey_search` | Full-text search across survey entries               |
| `survey_list`   | Browse entries filtered by analyzer, kind, or file   |
| `survey_stats`  | Aggregate counts by analyzer and kind                |
| `survey_run`    | Execute analyzers to populate survey data            |
| `survey_graph`  | Build call graph for a symbol (callers/callees/both) |

## Survey vs Findings vs Code Search

| Tool            | Purpose              | Example                                  |
| --------------- | -------------------- | ---------------------------------------- |
| **Survey**      | What the codebase IS | Modules, tech stack, entry points, churn |
| **Findings**    | Code problems        | Complexity, security issues, duplication |
| **Code search** | Symbol definitions   | Find function signatures, call sites     |

## Skill

Use `/aide:survey` to explore codebase structure interactively. The skill runs analyzers, queries results, and builds call graphs as needed.

## Prerequisites

- **Code index** (for entrypoints + call graph): Run `aide code index` first
- **Git history** (for churn): Must be a git repository. Uses go-git directly -- no `git` binary needed
