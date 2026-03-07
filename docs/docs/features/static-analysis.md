---
sidebar_position: 4
---

# Static Analysis

AIDE includes 4 built-in static analysers that detect code quality issues without external tools.

## Analysers

| Analyser     | Detects                                       | Severities        |
| ------------ | --------------------------------------------- | ----------------- |
| `complexity` | High cyclomatic complexity functions          | warning, critical |
| `coupling`   | High fan-in/fan-out, import cycles            | warning, critical |
| `secrets`    | Hardcoded API keys, tokens, passwords         | critical, warning |
| `clones`     | Duplicated code blocks (copy-paste detection) | warning, info     |

## Running Analysis

```bash
aide findings run                         # Run all analysers
aide findings run --analyser=complexity   # Run specific analyser
aide findings run --path=src/             # Scope to directory
```

Both `--analyser=` and `--analyzer=` are accepted on all commands.

## Querying Findings

```bash
aide findings stats                      # Overview: counts by analyser and severity
aide findings list --severity=critical   # All critical findings
aide findings list --file=src/auth       # Findings in specific files
aide findings search "AWS"               # Search findings by keyword
```

## Accepting (Dismissing) Findings

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

## MCP Tools

3 findings-related MCP tools make analysis available to the AI during code review and debugging:

| Tool              | Purpose                                                         |
| ----------------- | --------------------------------------------------------------- |
| `findings_search` | Full-text search across findings                                |
| `findings_list`   | List findings filtered by analyser, severity, file, or category |
| `findings_stats`  | Codebase health overview with counts by analyser and severity   |
| `findings_accept` | Accept (dismiss) findings by ID or filter                       |

## Auto-Run

When the file watcher is running (via `aide mcp`), findings are automatically re-run on changed files alongside code re-indexing.

## Typical Workflow

1. Use `/aide:patterns` to surface code health issues
2. Use `/aide:assess-findings` to triage -- the AI reads actual code for each finding, accepts noise, and reports what remains actionable
