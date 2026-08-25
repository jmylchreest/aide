---
name: assess-findings
description: Triage static analysis findings, grade them against recorded decisions, and accept noise or irrelevant items
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

### Decisions (conformance context)

| Tool            | Purpose                                                     |
| --------------- | ----------------------------------------------------------- |
| `decision_list` | List recorded decisions — load these **before** triaging    |
| `decision_get`  | Fetch one decision by topic when a finding may relate to it |

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

### 0. Load the Decisions

Call `decision_list` first — it is authoritative, where the "Project Decisions" block in
session context is a session-start snapshot that can predate a decision changed since. Fall
back to the block only if the tool is unavailable. Decisions are the recorded architectural
commitments for this project; a finding that evidences a decision violation is not noise,
whatever its analyzer or severity says.

Keep the list to hand for step 3. You are not searching the codebase for violations here —
that is the `review` skill's job. You are checking whether findings the analyzers **already
produced** happen to be evidence of one.

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
3. **Check it against the decisions** from step 0 — see "Decision conformance gate" below
4. **Make a judgement call** using these criteria:

#### Decision conformance gate

Before applying any accept criterion, ask: **does this finding evidence a violation of a
recorded decision?**

A finding is decision-linked when the code it flags contradicts a decision's stated
commitment. For example:

- a `deadcode` finding on an unreferenced symbol, against a decision requiring all code to
  be reachable from real call paths
- a `secrets` finding in a runtime path, against a decision on credential handling
- a `coupling` finding crossing a boundary a decision declared off-limits
- a `security` finding using a mechanism a decision ruled out

**A decision-linked finding cannot be accepted as noise.** There are exactly two valid
resolutions:

1. **Fix the code** so it conforms — the finding disappears on the next `findings run`.
2. **Amend the decision** — if the decision is genuinely wrong or has been superseded, say
   so explicitly and tell the user to run `/decide` on that topic. Do not accept the finding
   on the assumption that the decision will change.

If you are unsure whether a finding is decision-linked, treat it as linked and keep it.
Report it under "Decision conflicts" in the summary so a human decides.

Note the limits of this gate. It grades findings the analyzers already produced; it cannot
find a decision violation that no analyzer flagged. For a code-level sweep against the
decisions, use the `review` skill.

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
- **The finding is decision-linked** (see the conformance gate above) — this overrides every accept criterion

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

### Decision Conflicts (must not be accepted)

- K findings evidence a violation of a recorded decision
  - `<decision-topic>` — file:line — [what the code does vs what the decision requires]
  - (omit this section entirely when there are none)

### Remaining (Genuine)

- M findings require attention
  - [List each with file:line and brief description]

### Recommendations

1. [Prioritised action items for genuine findings]
```

## Decision Criteria Reference

| Analyzer   | Accept If                                                                                                                            | Keep If                                                                             |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| complexity | Cyclomatic complexity is inherent to the problem domain; function handles unavoidable branching (CLI dispatch, protocol negotiation) | Function can be decomposed into smaller, testable units                             |
| clones     | Duplication is cross-cutting boilerplate (CLI wiring, store CRUD patterns)                                                           | A shared utility or abstraction would reduce maintenance burden                     |
| coupling   | File is an intentional integration point (main, facade, registry)                                                                    | Circular dependencies or unexpected transitive coupling exists                      |
| secrets    | Test fixture, documentation example, env var name, or placeholder                                                                    | Looks like a real credential, API key, or connection string                         |
| **any**    | **Never** — a decision-linked finding is out of scope for acceptance                                                                 | **The finding evidences a recorded decision violation** (overrides every row above) |

## Failure Handling

1. **No findings** — Tell user to run `./.aide/bin/aide findings run --path .` first
2. **`findings_accept` not available** — The aide MCP server may not expose this tool; tell the user to update aide
3. **Uncertain about a finding** — When in doubt, **keep it**. It's better to flag a false positive for human review than to dismiss a real issue
4. **Large number of findings** — Work in batches by analyzer. Accept obvious noise first, then do detailed code review for borderline cases
5. **`decision_list` returns nothing** — The project has no recorded decisions; skip the conformance gate and say so in the summary. Do not invent decisions

## Verification

- [ ] Called `decision_list` (or noted the fallback) before triaging
- [ ] Called `findings_stats` for baseline counts
- [ ] Reviewed each finding category (secrets, complexity, clones, coupling)
- [ ] Read actual code for every finding before accepting
- [ ] Checked every finding against the recorded decisions before accepting it
- [ ] No decision-linked finding was accepted
- [ ] Provided rationale for each acceptance
- [ ] Produced summary with before/after counts
- [ ] Remaining findings are genuinely actionable
