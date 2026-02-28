---
name: assess-findings
description: Triage static analysis findings, assess merit, and accept noise or irrelevant items
triggers:
  - assess findings
  - analyse findings
  - analyze findings
  - triage findings
  - review findings
  - accept findings
  - dismiss findings
  - clean up findings
---

# Assess Findings

**Recommended model tier:** balanced (sonnet) - this skill requires reading code and making judgement calls

Triage static analysis findings by reading the actual code, assessing whether each finding
is genuine or noise, and accepting (dismissing) irrelevant ones using `findings_accept`.
Accepted findings are hidden from future output by default.

## Prerequisites

- Findings must already exist. If `findings_stats` returns zero counts, tell the user to run:
  ```bash
  ./.aide/bin/aide findings run --path .
  ```
- The `findings_accept` tool must be available (provided by the aide MCP server).

## Available Tools

### Read-only (shared with `patterns` skill)

| Tool              | Purpose                                                 |
| ----------------- | ------------------------------------------------------- |
| `findings_stats`  | Counts by analyzer and severity — start here            |
| `findings_list`   | Browse findings with filters (analyzer, severity, file) |
| `findings_search` | Full-text search across finding titles and details      |

### Write (unique to this skill)

| Tool              | Purpose                                             |
| ----------------- | --------------------------------------------------- |
| `findings_accept` | Mark findings as accepted/dismissed by ID or filter |

### Code inspection

| Tool           | Purpose                                             |
| -------------- | --------------------------------------------------- |
| `code_outline` | Get collapsed file structure to understand context  |
| `Read`         | Read specific line ranges to evaluate finding merit |

## Workflow

### 1. Get the Landscape

Call `findings_stats` to understand the scope:

```
findings_stats
-> Returns: counts per analyzer (complexity, coupling, secrets, clones) and severity
```

If the user asked to focus on a specific analyzer or severity, note that and filter accordingly.
Otherwise, work through all findings systematically.

### 2. Prioritise Review Order

Work through findings in this order:

1. **Secrets** (critical first) — these need immediate attention; false positives are common in test fixtures
2. **Complexity** (critical, then warning) — assess whether high complexity is inherent or decomposable
3. **Clones** (all) — determine if duplication is extractable or structural boilerplate
4. **Coupling** (all) — assess whether high fan-in/fan-out is expected for the file's role

### 3. Assess Each Finding

For each finding or group of related findings:

1. **Read the finding details** — note the file, line range, and metric values
2. **Read the actual code** — use `code_outline` first, then `Read` with offset/limit on the flagged section
3. **Make a judgement call** using these criteria:

#### Accept (dismiss) when:

- **Complexity**: The function is inherently complex (CLI dispatch, protocol handling, state machines) and cannot be meaningfully decomposed without harming readability
- **Clones**: The duplication is structural boilerplate (e.g., CLI subcommand wiring, store method patterns) where extraction would require framework-level abstraction
- **Coupling**: High fan-in/fan-out is expected for the file's architectural role (e.g., a main entry point, a facade, a registry)
- **Secrets**: The flagged string is a test fixture, example config, documentation placeholder, or env var name (not an actual secret)

#### Keep (do NOT accept) when:

- The finding points to a genuine problem that should be fixed
- Complexity can be reduced by extracting helper functions
- Duplication can be resolved by creating a shared utility
- A coupling cycle exists that indicates poor module boundaries
- A string looks like it could be a real secret or credential

### 4. Accept Findings

Use `findings_accept` to dismiss noise. You can accept:

- **By IDs** — for individual findings after assessment:
  ```
  findings_accept ids=["finding-id-1", "finding-id-2"]
  ```
- **By filter** — for bulk dismissal of an entire category:
  ```
  findings_accept analyzer="clones" file="cmd/"
  ```

Always explain **why** each finding is being accepted before calling the tool.

### 5. Report Summary

After completing the triage, produce a summary:

```markdown
## Findings Triage Summary

### Before

- Total: X findings (Y critical, Z warnings, W info)

### Accepted (Dismissed)

- N findings accepted as noise/irrelevant
  - Complexity: X (inherent complexity in [files])
  - Clones: Y (structural boilerplate in [area])
  - Coupling: Z (expected for [role])
  - Secrets: W (test fixtures / placeholders)

### Remaining (Genuine)

- M findings require attention
  - [List each with file:line and brief description]

### Recommendations

1. [Prioritised action items for genuine findings]
```

## Decision Criteria Reference

| Analyzer   | Accept If                                                                                                                            | Keep If                                                         |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------- |
| complexity | Cyclomatic complexity is inherent to the problem domain; function handles unavoidable branching (CLI dispatch, protocol negotiation) | Function can be decomposed into smaller, testable units         |
| clones     | Duplication is cross-cutting boilerplate (CLI wiring, store CRUD patterns)                                                           | A shared utility or abstraction would reduce maintenance burden |
| coupling   | File is an intentional integration point (main, facade, registry)                                                                    | Circular dependencies or unexpected transitive coupling exists  |
| secrets    | Test fixture, documentation example, env var name, or placeholder                                                                    | Looks like a real credential, API key, or connection string     |

## Failure Handling

1. **No findings** — Tell user to run `./.aide/bin/aide findings run --path .` first
2. **`findings_accept` not available** — The aide MCP server may not expose this tool; tell the user to update aide
3. **Uncertain about a finding** — When in doubt, **keep it**. It's better to flag a false positive for human review than to dismiss a real issue
4. **Large number of findings** — Work in batches by analyzer. Accept obvious noise first, then do detailed code review for borderline cases

## Verification

- [ ] Called `findings_stats` for baseline counts
- [ ] Reviewed each finding category (secrets, complexity, clones, coupling)
- [ ] Read actual code for every finding before accepting
- [ ] Provided rationale for each acceptance
- [ ] Produced summary with before/after counts
- [ ] Remaining findings are genuinely actionable
