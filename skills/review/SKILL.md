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

## Review Process

1. **Read the diff/files** - Understand what changed
2. **Search for context** - Use `code_search` MCP tool to find:
   - Related symbols that might be affected
   - Other usages of modified functions/classes
   - Similar patterns in the codebase
3. **Check integration** - How does it fit the larger system?
4. **Run static analysis** - Use lsp_diagnostics, ast_grep if available
5. **Document findings** - Use severity levels

## MCP Tools

Use these tools during review:

- `mcp__plugin_aide_aide__code_search` - Find symbols related to changes (e.g., `code_search query="getUserById"`)
- `mcp__plugin_aide_aide__code_symbols` - List all symbols in a file being reviewed
- `mcp__plugin_aide_aide__memory_search` - Check for related past decisions or issues

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

| Level | Criteria |
|-------|----------|
| Critical | Security vulnerability, data loss risk, crash |
| Warning | Bug potential, maintainability issue, performance |
| Suggestion | Style, minor improvement, optional |

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

1. **Read all changed files** - Verify each file was actually read
2. **Check for related code** - Use code search to find callers/callees
3. **Verify test coverage** - Check if tests exist for critical paths
4. **Document all findings** - Even if no issues found, state that explicitly

### Checklist before submitting review:

- [ ] All files in diff/scope have been read
- [ ] Related symbols searched (callers, implementations)
- [ ] Security checklist evaluated
- [ ] Findings documented with file:line references
- [ ] Verdict provided with clear reasoning
