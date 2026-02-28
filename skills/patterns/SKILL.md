---
name: patterns
description: Analyze codebase patterns, detect anti-patterns, and surface static analysis findings
triggers:
  - find patterns
  - anti-patterns
  - code smells
  - complexity
  - duplicated code
  - clones
  - secrets
  - coupling
  - static analysis
  - code health
---

# Pattern Analysis

**Recommended model tier:** balanced (sonnet) - this skill combines search with structured analysis

Analyze codebase patterns using static analysis findings. Surface complexity hotspots, code
duplication, coupling issues, and potential secrets. Use this skill to understand code health
and identify areas that need attention.

## Prerequisites

Findings must be generated first by running analyzers via the CLI:

```bash
# Run all analyzers
./.aide/bin/aide findings run --path .

# Run specific analyzers
./.aide/bin/aide findings run --path . --analyzer complexity
./.aide/bin/aide findings run --path . --analyzer coupling
./.aide/bin/aide findings run --path . --analyzer secrets
./.aide/bin/aide findings run --path . --analyzer clones
```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

## Available Tools

### 1. Search Findings (`mcp__plugin_aide_aide__findings_search`)

Full-text search across all findings. Supports Bleve query syntax for advanced searches.

**Parameters:**

- `query` (required) — Search term or Bleve query
- `analyzer` (optional) — Filter to one analyzer: `complexity`, `coupling`, `secrets`, `clones`
- `severity` (optional) — Filter by severity: `info`, `warning`, `critical`
- `file` (optional) — Filter by file path substring
- `limit` (optional) — Max results (default 20)

**Example usage:**

```
Search for: "high complexity"
-> findings_search query="complexity" severity="warning"
-> Returns: functions with high cyclomatic complexity, with file:line
```

### 2. List Findings (`mcp__plugin_aide_aide__findings_list`)

List findings with filters. Use when you want to browse rather than search.

**Parameters:**

- `analyzer` (optional) — Filter to one analyzer
- `severity` (optional) — Filter by severity
- `file` (optional) — Filter by file path substring
- `limit` (optional) — Max results (default 20)
- `offset` (optional) — Pagination offset

**Example usage:**

```
List all critical findings
-> findings_list severity="critical"
-> Returns: all critical-severity findings across all analyzers
```

### 3. Findings Statistics (`mcp__plugin_aide_aide__findings_stats`)

Get aggregate counts by analyzer and severity. Use as a starting point to understand overall
code health before drilling into specifics.

**Example usage:**

```
How healthy is the codebase?
-> findings_stats
-> Returns: counts per analyzer, counts per severity, total findings
```

## Workflow

### Quick Health Check

1. **Get overview** — Use `findings_stats` to see counts by analyzer and severity
2. **Triage critical** — Use `findings_list severity="critical"` to review highest-priority items
3. **Drill into areas** — Use `findings_search` with file or query filters for specific concerns

### Complexity Analysis

1. Run `findings_search analyzer="complexity" severity="critical"` to find the most complex functions
2. Use `code_outline` on flagged files to understand structure
3. Use `Read` with offset/limit to examine the specific functions
4. Recommend decomposition strategies

### Duplication Analysis

1. Run `findings_list analyzer="clones"` to see detected code clones
2. Each finding includes the clone pair — both file locations and line ranges
3. Use `Read` to compare the duplicated sections
4. Recommend extraction into shared functions or modules

### Coupling Analysis

1. Run `findings_search analyzer="coupling"` to see import fan-out/fan-in issues
2. High fan-out means a file imports too many things (potential god module)
3. High fan-in means many files depend on one (fragile dependency)
4. Cycle findings indicate circular dependency chains

### Secret Detection

1. Run `findings_list analyzer="secrets" severity="critical"` for confirmed secrets
2. Run `findings_list analyzer="secrets"` for all potential secrets (including unverified)
3. Each finding includes the secret category (e.g., AWS, GitHub, generic API key)
4. Snippets are redacted for safety — use `Read` to examine context around the finding

## Anti-Pattern Identification

Beyond the automated analyzers, look for these patterns using findings as starting points:

| Finding         | Likely Anti-Pattern    | Action                           |
| --------------- | ---------------------- | -------------------------------- |
| Complexity > 20 | God function           | Decompose into smaller functions |
| Fan-out > 15    | Kitchen sink module    | Split responsibilities           |
| Fan-in > 20     | Fragile dependency     | Consider interface/abstraction   |
| Multiple clones | Copy-paste programming | Extract shared utility           |
| Import cycle    | Circular dependency    | Restructure module boundaries    |

## Output Format

```markdown
## Code Health Report

### Overview

- Total findings: X (Y critical, Z warnings)
- Top concern: [area/file with most issues]

### Hotspots

1. **`file:line`** - [description] (severity)
   - Impact: [why this matters]
   - Recommendation: [what to do]

### Patterns Detected

- [List of anti-patterns found with evidence]

### Recommendations

1. [Prioritized action items]
```

## Failure Handling

1. **No findings data** — Tell user to run analyzers first: `./.aide/bin/aide findings run --path .`
2. **Stale findings** — Findings reflect the state at last analyzer run; recommend re-running if code changed significantly
3. **False positives** — Secret detection may flag test fixtures or example configs; note when findings appear to be in test/example code

## Verification Criteria

- [ ] Checked `findings_stats` for overall picture
- [ ] Reviewed critical findings
- [ ] Cross-referenced findings with actual code (used `Read` or `code_outline`)
- [ ] Provided actionable recommendations with file:line references
- [ ] Noted any false positives or findings that need manual verification
