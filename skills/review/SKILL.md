---
name: review
description: Code review and security audit, including conformance against recorded decisions
triggers:
  - review this
  - review the
  - code review
  - security audit
  - audit this
  - check decisions
  - decision conformance
  - conformance check
---

# Code Review Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Comprehensive code review covering quality, security, and maintainability.

## Review Checklist

### Decision Conformance

Recorded decisions are this project's binding architectural commitments. Check them first —
a conformance violation outranks any style or quality issue below.

- [ ] Called `decision_list` (or noted the fallback) before reading any code
- [ ] Every changed file checked against every decision that could apply to it
- [ ] Every ✅ backed by a cited check, not by inspection alone
- [ ] Every decision on its own line, including the ones that do not apply — no bulk dismissals
- [ ] "Decisions loaded: N" counted, not estimated, and matching the number of lines
- [ ] No decision violated by the change under review
- [ ] Change does not extend a pre-existing violation
- [ ] Where a decision seems wrong rather than the code, that is stated explicitly

### Code Quality

- [ ] Clear naming (variables, functions, classes)
- [ ] Single responsibility (functions do one thing)
- [ ] DRY (no unnecessary duplication)
- [ ] Appropriate abstraction level
- [ ] Error handling coverage
- [ ] Edge cases considered

### Security (OWASP Top 10)

- [ ] Input validation (no injection vulnerabilities)
- [ ] Authentication checks (routes protected)
- [ ] Authorization (proper access control)
- [ ] Sensitive data handling (no secrets in code)
- [ ] SQL/NoSQL injection prevention
- [ ] XSS prevention (output encoding)
- [ ] CSRF protection
- [ ] Secure dependencies (no known vulnerabilities)

### Maintainability

- [ ] Code is readable without comments
- [ ] Comments explain "why" not "what"
- [ ] Consistent with codebase patterns
- [ ] Tests cover critical paths
- [ ] No dead code

### Performance

- [ ] No N+1 queries
- [ ] Appropriate caching
- [ ] No memory leaks
- [ ] Efficient algorithms

## Context-Efficient Reading

Prefer lightweight tools first, then read in detail where needed:

- **`code_outline`** -- Collapsed skeleton with signatures and line ranges. Great first step for unfamiliar files.
- **`code_symbols`** -- Quick symbol list when you only need names and kinds.
- **`code_search`** / **`code_references`** -- Find symbol definitions or callers across the codebase.
- **`Read` with offset/limit** -- Read specific functions using line numbers from the outline.
- **Grep** -- Find patterns in code content (loops, queries, string literals) that the index doesn't cover.

For reviews spanning many files, consider using **Task sub-agents** (`explore` type) which run in their
own context and return summaries.

## Review Process

1. **Load the decisions** - Call `decision_list`. It is authoritative: live, and complete
   with Details and References. The "Project Decisions" block in session context is a
   convenience copy — a snapshot taken at session start that omits Details and References
   and goes stale the moment a decision changes, so it is a fallback when the tool is
   unavailable, never the source you check against. Use `decision_get` for one decision's
   full text when the list entry is not enough to judge conformance.
   See "Decision Conformance Pass" below for how to check them.
2. **Outline changed files** - Use `code_outline` on each changed file to understand structure.
   Identify areas of concern from signatures and line ranges.
3. **Read targeted sections** - Use `Read` with `offset`/`limit` to read only the specific
   functions/sections that need detailed review (use line numbers from the outline).
4. **Search for context** - Use `code_search`, `code_references`, and **Grep**:
   - `code_search` — Find related function/class/type _definitions_ by name
   - `code_references` — Find all callers/usages of a modified symbol (exact name match)
   - **Grep** — Find code _patterns_ in bodies (error handling, SQL queries, security-sensitive calls)
5. **Check integration** - How does it fit the larger system?
6. **Run static analysis** - Use lsp_diagnostics, ast_grep if available
7. **Document findings** - Use severity levels

## Decision Conformance Pass

This is the part of the review that no static analyzer can do for you. `assess-findings`
grades findings the analyzers already produced; this pass reads the code and looks for
violations nothing flagged.

**Scope.** Default to the changed code — the diff, or the files the user named. Say which
scope you used. Only sweep the whole repo when the user asks for it; a full sweep is slow
and mostly re-reports known debt.

**Method.** For each decision, work out what would falsify it, then go looking for that:

| Decision shape                                                             | How to check it                                                                                                                                                                                                       |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bans a mechanism ("no bash in runtime paths")                              | **Grep** for the mechanism across the reviewed scope                                                                                                                                                                  |
| Requires a boundary ("plugin code must not import daemon internals")       | `code_references` on the target symbols, or Grep the import lines                                                                                                                                                     |
| Requires a property of all code ("must be reachable from real call paths") | `findings_list analyzer=deadcode include_accepted=true` for the scope, then confirm by reading. Accepted findings are hidden by default, and an acceptance predating the decision is exactly what you are looking for |
| Mandates a library or approach ("use go-git, not shelling out to git")     | Grep for the banned alternative and for the required one                                                                                                                                                              |
| Constrains a platform or runtime                                           | Read the changed code for platform-specific assumptions                                                                                                                                                               |

A decision whose text gives you nothing falsifiable is not checkable here. Say so plainly
rather than inventing a check — and mention it to the user, because a decision that cannot
be checked is a decision that cannot be enforced.

**Evidence is required, including for a pass.** Every ✅ must cite what produced it — the
Grep pattern you ran, the `code_references` call, the `findings_list` filter, or the specific
lines you read. "Looks fine" is not a check, and a conformance block full of uncited ticks is
worse than no block at all: it reports assurance you did not earn.

**Every decision gets its own line.** Report all of them, individually, with a reason —
including the ones that do not apply. Never collapse several into a category dismissal
("the remaining N are out of scope"): a bulk ➖ is the same evasion as an uncited ✅, and it
is how a decision that did apply gets swept up with the ones that did not. A ➖ still needs
its own evidence — the path check or grep that shows the diff does not touch what the
decision governs. Grouping several decisions under one shared piece of evidence is fine
when the evidence genuinely covers them all (one path check disposing of every `survey-*`
decision, say), as long as each still has its own line naming it.

**State the count and make it match.** "Decisions loaded: N" must equal the number of lines
below it. Count them; do not estimate.

**A hidden finding is not an absent one.** Any `findings_*` call made to discharge a
decision must pass `include_accepted=true`; the default hides accepted findings, so a
single earlier acceptance turns every later review into a cited, confident, wrong ✅ —
the exact failure the evidence rule above exists to prevent. When an accepted finding is
decision-linked, report the acceptance rather than passing over it: it may predate the
decision, and nothing re-examines it otherwise.

**Reporting.** A confirmed violation is 🔴 Critical regardless of how small the code change
is, and must name the decision topic. If the code looks right and the decision looks wrong
or stale, say that instead of forcing a violation — recommend `/decide` on that topic.

## MCP Tools

Use these tools during review:

- `mcp__plugin_aide_aide__code_outline` - **Start here.** Get collapsed file skeleton with signatures and line ranges
- `mcp__plugin_aide_aide__code_search` - Find symbols related to changes (e.g., `code_search query="getUserById"`)
- `mcp__plugin_aide_aide__code_symbols` - List all symbols in a file being reviewed
- `mcp__plugin_aide_aide__code_references` - Find all callers/usages of a modified symbol
- `mcp__plugin_aide_aide__decision_list` - **Load first.** The authoritative list of recorded
  decisions — current, and including Details and References
- `mcp__plugin_aide_aide__decision_get` - Full text of one decision by topic, including rationale
- `mcp__plugin_aide_aide__memory_search` - Check for related past learnings, gotchas, or issues
- `mcp__plugin_aide_aide__findings_search` - Search static analysis findings (complexity, secrets, clones) related to changed code
- `mcp__plugin_aide_aide__findings_list` - List findings filtered by file, severity, or analyzer
- `mcp__plugin_aide_aide__findings_stats` - Overview of finding counts by analyzer and severity

## Output Format

```markdown
## Code Review: [Feature/PR Name]

### Summary

[1-2 sentence overview]

### Decision Conformance

Scope checked: [diff / named files / whole repo]
Decisions loaded: N

- ✅ `<topic>` — conforms — checked by: [grep pattern / tool call / lines read]
- ❌ `<topic>` — violated at `file:line`: [what the code does vs what the decision requires]
- ➖ `<topic>` — n/a: [the path check or grep showing the diff does not touch what it governs]
- ➖ `<topic>` — not checkable: [nothing falsifiable in the decision text as written]

One line per decision, no exceptions — the list length must equal "Decisions loaded".
Decisions sharing one piece of evidence still get one line each.

(State "no decisions recorded" if `decision_list` returns none.)

### Findings

#### 🔴 Critical (must fix)

- **[Issue]** `file:line`
  - Problem: [description]
  - Fix: [recommendation]

#### 🟡 Warning (should fix)

- **[Issue]** `file:line`
  - Problem: [description]
  - Fix: [recommendation]

#### 🔵 Suggestion (consider)

- **[Issue]** `file:line`
  - Suggestion: [recommendation]

### Security Notes

- [Any security-specific observations]

### Verdict

[ ] ✅ Approve
[ ] ⚠️ Approve with comments
[ ] ❌ Request changes
```

## Severity Guide

| Level      | Criteria                                                                      |
| ---------- | ----------------------------------------------------------------------------- |
| Critical   | Security vulnerability, data loss risk, crash, **recorded decision violated** |
| Warning    | Bug potential, maintainability issue, performance                             |
| Suggestion | Style, minor improvement, optional                                            |

## Failure Handling

### If unable to complete review:

1. **Missing files** - Report which files could not be read
2. **Ambiguous scope** - Ask user to clarify what code to review
3. **Large changeset** - Break into smaller chunks, review systematically
4. **No decisions recorded** - `decision_list` returns none; skip the conformance pass, say so
   in the report, and continue with the rest of the review. Never invent decisions
5. **`decision_list` unavailable** - Fall back to the injected "Project Decisions" block and
   say in the report that you used it, since it omits Details and References and may predate
   a decision changed this session. If neither is available, report the conformance pass as
   not run rather than passing it

### Reporting blockers:

```markdown
## Review Status: Incomplete

### Blockers

- Could not access: `path/to/file.ts` (permission denied)
- Missing context: Need to understand `AuthService` implementation

### Partial Findings

[Include any findings from files that were reviewed]
```

## Verification Criteria

A complete code review must:

1. **Outline all changed files** - Use `code_outline` on every file in scope
2. **Read critical sections** - Use targeted `Read` with offset/limit on flagged areas
3. **Check for related code** - Use `code_search` and `code_references` to find callers/callees
4. **Verify test coverage** - Check if tests exist for critical paths
5. **Document all findings** - Even if no issues found, state that explicitly

### Checklist before submitting review:

- [ ] All files in diff/scope have been outlined
- [ ] Critical functions/sections read in detail (with offset/limit)
- [ ] Related symbols searched (callers, implementations)
- [ ] Security checklist evaluated
- [ ] Findings documented with file:line references
- [ ] Verdict provided with clear reasoning
