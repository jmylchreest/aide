# Getting Started on an Existing Project

When you open an unfamiliar codebase for the first time — even a very large one like the Linux kernel — AIDE can map it for you in minutes.

## First Prompt

```
Survey this codebase and help me understand its structure.
```

This triggers the survey skill, which will:

1. **Run survey analyzers** — Discover modules, tech stack, entry points, and git churn hotspots
2. **Index symbols** (if not already indexed) — Parse functions, types, and methods using tree-sitter for call graph navigation
3. **Present the big picture** — Modules, languages, build systems, where execution starts, and what files change most

## Follow-Up Questions

Once the survey is populated, drill into specifics:

```
What modules are in this project?
Where are the entry points?
What files change most often?
Who calls handleRequest?
```

### What Each Question Gives You

| Question     | What you learn                                                           |
| ------------ | ------------------------------------------------------------------------ |
| Modules      | Major packages, workspaces, submodules — the high-level decomposition    |
| Entry points | `main()` functions, HTTP handlers, CLI roots — where execution starts    |
| Churn        | Files ranked by commit frequency — where active development is happening |
| Call graph   | Who calls a function, what it depends on — how pieces connect            |

## Why Churn Matters

In a codebase with thousands of files, most of them barely change. Churn tells you which files actually matter right now:

- **High churn + high complexity** = top refactoring targets
- **High churn + low test coverage** = risk areas
- **High churn files** = where you should focus when onboarding

## CLI Alternative

You can also run the tools directly from the command line:

```bash
aide code index          # Index symbols (once, cached)
aide survey run          # Run all survey analyzers (once, cached)
aide findings run all    # Run all static analysers (optional, for code health)
aide status              # See what's populated
```

Results are cached in `.aide/` — subsequent sessions start instantly.

## Combining Survey with Findings

After survey tells you _what_ the codebase is, use findings to find _problems_:

```
Run a code health check and assess the findings.
```

This runs static analysis (complexity, coupling, duplication, secrets), then triages findings — accepting noise and flagging genuine issues. The overlap between high-churn survey entries and high-complexity findings is where you'll find the most impactful improvements.
