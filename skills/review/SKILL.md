---
name: review
description: Code review and security audit
triggers:
  - review this
  - review the
  - code review
  - security audit
  - audit this
---

# Code Review Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Comprehensive code review covering quality, security, and maintainability.

## Review Checklist

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

1. **Outline changed files** - Use `code_outline` on each changed file to understand structure.
   Identify areas of concern from signatures and line ranges.
2. **Read targeted sections** - Use `Read` with `offset`/`limit` to read only the specific
   functions/sections that need detailed review (use line numbers from the outline).
3. **Search for context** - Use `code_search`, `code_references`, and **Grep**:
   - `code_search` — Find related function/class/type _definitions_ by name
   - `code_references` — Find all callers/usages of a modified symbol (exact name match)
   - **Grep** — Find code _patterns_ in bodies (error handling, SQL queries, security-sensitive calls)
4. **Check integration** - How does it fit the larger system?
5. **Run static analysis** - Use lsp_diagnostics, ast_grep if available
6. **Document findings** - Use severity levels

## MCP Tools

Use these tools during review:

- `mcp__plugin_aide_aide__code_outline` - **Start here.** Get collapsed file skeleton with signatures and line ranges
- `mcp__plugin_aide_aide__code_search` - Find symbols related to changes (e.g., `code_search query="getUserById"`)
- `mcp__plugin_aide_aide__code_symbols` - List all symbols in a file being reviewed
- `mcp__plugin_aide_aide__code_references` - Find all callers/usages of a modified symbol
- `mcp__plugin_aide_aide__memory_search` - Check for related past decisions or issues
- `mcp__plugin_aide_aide__findings_search` - Search static analysis findings (complexity, secrets, clones) related to changed code
- `mcp__plugin_aide_aide__findings_list` - List findings filtered by file, severity, or analyzer
- `mcp__plugin_aide_aide__findings_stats` - Overview of finding counts by analyzer and severity

## Output Format

```markdown
## Code Review: [Feature/PR Name]

### Summary

[1-2 sentence overview]

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

| Level      | Criteria                                          |
| ---------- | ------------------------------------------------- |
| Critical   | Security vulnerability, data loss risk, crash     |
| Warning    | Bug potential, maintainability issue, performance |
| Suggestion | Style, minor improvement, optional                |

## Failure Handling

### If unable to complete review:

1. **Missing files** - Report which files could not be read
2. **Ambiguous scope** - Ask user to clarify what code to review
3. **Large changeset** - Break into smaller chunks, review systematically

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
