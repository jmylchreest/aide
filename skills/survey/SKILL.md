---
name: survey
description: Explore codebase structure, entry points, tech stack, hotspots, and call graphs
triggers:
  - survey
  - codebase structure
  - what is this codebase
  - tech stack
  - entry points
  - entrypoints
  - what modules
  - what packages
  - code churn
  - hotspots
  - call graph
  - who calls
  - what calls
  - orient me
  - onboard
  - codebase overview
---

# Codebase Survey

**Recommended model tier:** balanced (sonnet) - this skill performs structured queries

Understand the structure, technology, entry points, and change hotspots of a codebase.
Survey describes WHAT the codebase IS — not code problems (use `findings` for that).

## Available Tools

### 1. Survey Stats (`mcp__plugin_aide_aide__survey_stats`)

**Start here.** Get an overview of what has been surveyed: total entries, breakdown by analyzer and kind.

```
Is the codebase surveyed?
→ Uses survey_stats
→ Returns: counts by analyzer (topology, entrypoints, churn) and kind
```

### 2. Survey Run (`mcp__plugin_aide_aide__survey_run`)

Run analyzers to populate survey data. Three analyzers available:

- **topology** — Modules, packages, workspaces, build systems, tech stack detection
- **entrypoints** — main() functions, HTTP handlers, gRPC services, CLI roots (cobra/urfave). Uses code index when available; falls back to file scanning
- **churn** — Git history hotspots (files/dirs that change most often)

```
Survey this codebase
→ Uses survey_run (no analyzer param = run all)
→ Returns: entry counts per analyzer
```

### 3. Survey List (`mcp__plugin_aide_aide__survey_list`)

Browse entries filtered by analyzer, kind, or file path. No search query needed.

**Kinds:** module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern

```
What modules are in this codebase?
→ Uses survey_list with kind=module
→ Returns: all module entries

What technologies does this use?
→ Uses survey_list with kind=tech_stack
→ Returns: detected frameworks, languages, build systems

What files change most?
→ Uses survey_list with kind=churn
→ Returns: high-churn files ranked by commit count
```

### 4. Survey Search (`mcp__plugin_aide_aide__survey_search`)

Full-text search across entry names, titles, and details. Use when looking for specific modules or technologies.

```
Find anything related to "auth"
→ Uses survey_search with query="auth"
→ Returns: modules, entrypoints, churn entries matching "auth"
```

### 5. Call Graph (`mcp__plugin_aide_aide__survey_graph`)

Build a call graph for a symbol showing callers, callees, or both. BFS traversal over the code index.

```
Who calls BuildCallGraph?
→ Uses survey_graph with symbol="BuildCallGraph" direction="callers"
→ Returns: graph of calling symbols with file:line locations

What does handleSurveyRun call?
→ Uses survey_graph with symbol="handleSurveyRun" direction="callees"
→ Returns: graph of called symbols

Show call neighborhood of RunTopology
→ Uses survey_graph with symbol="RunTopology" direction="both"
→ Returns: both callers and callees
```

**Parameters:**

- `symbol` (required): Function/method name
- `direction`: "both" (default), "callers", "callees"
- `max_depth`: BFS hops (default 2)
- `max_nodes`: Max nodes (default 50)

**Requires:** Code index must be populated (`aide code index`).

## Workflow

### Orienting in an unfamiliar codebase

1. **Check survey status:**
   - Use `survey_stats` to see if data exists
   - If empty, run `survey_run` to populate

2. **Understand the structure:**
   - `survey_list kind=module` — What are the major modules?
   - `survey_list kind=tech_stack` — What technologies are used?
   - `survey_list kind=workspace` — Is this a monorepo?

3. **Find entry points:**
   - `survey_list kind=entrypoint` — Where does execution start?
   - Identifies main() functions, HTTP handlers, CLI roots

4. **Identify hotspots:**
   - `survey_list kind=churn` — What files change most? (complexity/bug magnets)

5. **Trace call relationships:**
   - `survey_graph symbol="handleRequest"` — Map the call neighborhood
   - Use `direction=callers` to find who invokes a function
   - Use `direction=callees` to understand what a function depends on

### Answering specific questions

| Question                      | Tool            | Parameters                  |
| ----------------------------- | --------------- | --------------------------- |
| "What is this codebase?"      | `survey_list`   | kind=module                 |
| "What tech stack?"            | `survey_list`   | kind=tech_stack             |
| "Where are the entry points?" | `survey_list`   | kind=entrypoint             |
| "What changes most?"          | `survey_list`   | kind=churn                  |
| "Is there an auth module?"    | `survey_search` | query="auth"                |
| "Who calls this function?"    | `survey_graph`  | symbol=X, direction=callers |
| "What does this call?"        | `survey_graph`  | symbol=X, direction=callees |

## Survey vs Findings vs Code Search

| Tool            | Purpose              | Example                                  |
| --------------- | -------------------- | ---------------------------------------- |
| **Survey**      | WHAT the codebase IS | Modules, tech stack, entry points, churn |
| **Findings**    | Code PROBLEMS        | Complexity, security issues, duplication |
| **Code Search** | Symbol DEFINITIONS   | Find function signatures, call sites     |

Survey gives you the big picture. Code search gives you specific symbols. Findings gives you problems to fix.

## Prerequisites

- **Survey data:** Run `aide survey run` or use `survey_run` tool
- **Code index (for entrypoints + graph):** Run `aide code index`
- **Git history (for churn):** Must be a git repository (uses go-git, no git binary needed)

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

## Notes

- Survey results are cached in BoltDB — re-run analyzers to refresh after significant changes
- Topology analyzer inspects the filesystem (build files, directory structure)
- Entrypoints analyzer uses the code index when available; falls back to file scanning
- Churn analyzer uses go-git to read git history directly (no git binary required)
- Call graph is computed on demand from the code index (not stored)
