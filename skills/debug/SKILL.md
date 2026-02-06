---
name: debug
description: Systematic debugging workflow for tracking down bugs and issues
triggers:
  - "debug this"
  - "debug mode"
  - "fix this bug"
  - "trace the bug"
  - "find the bug"
---

# Debug Mode

Systematic approach to identifying and fixing bugs.

## Prerequisites

Before starting:
- Get the exact error message or unexpected behavior description
- Identify the entry point or trigger for the bug
- Note any relevant environment details (Node version, OS, etc.)

## Workflow

### Step 1: Reproduce the Issue

**Goal:** Confirm the bug exists and understand its behavior.

```bash
# Run the failing code/test
npm test -- --grep "failing test"
# OR
node path/to/script.js
```

Document:
- Exact error message
- Steps to trigger
- Expected vs actual behavior
- Is it consistent or intermittent?

**If cannot reproduce:**
- Check environment differences
- Look for race conditions
- Check for cached state

### Step 2: Locate the Relevant Code

Use tools to find code related to the error:

```
# Search for function mentioned in stack trace
mcp__plugin_aide_aide__code_search query="functionName" kind="function"

# Get symbols in suspect file
mcp__plugin_aide_aide__code_symbols file="path/to/file.ts"

# Search for error message text in code
Grep for "error message text"
```

### Step 3: Trace the Execution Path

Follow the code flow from entry to error:

1. Start at the entry point (route handler, event listener, etc.)
2. Trace through each function call
3. Use `mcp__plugin_aide_aide__code_references` to find callers
4. Check type definitions with `mcp__plugin_aide_aide__code_search kind="interface"`

### Step 4: Form Hypotheses

Based on the error type, consider these causes:

| Symptom | Likely Causes |
|---------|---------------|
| "undefined is not a function" | Variable is null/undefined, wrong import |
| "Cannot read property of undefined" | Missing null check, async timing issue |
| "Type error" | Type mismatch, wrong function signature |
| "Maximum call stack" | Infinite recursion, circular reference |
| "Network error" | Bad URL, CORS, timeout, server down |
| "State not updating" | Mutation instead of new object, missing dependency |

### Step 5: Validate Hypotheses

Test each hypothesis systematically:

```bash
# Add temporary logging
console.log('DEBUG: variable =', JSON.stringify(variable));

# Check with debugger
node --inspect-brk script.js

# Run specific test
npm test -- --grep "test name"
```

**Validation checklist:**
- Check variable values at key points
- Verify assumptions about input data
- Test edge cases (null, empty, boundary values)
- Check async ordering

### Step 6: Apply the Fix

**Rules:**
- Change only what's necessary to fix the bug
- Don't refactor unrelated code
- Match existing code patterns

**Common fixes:**
- Add null/undefined check
- Fix type annotation
- Correct async/await usage
- Fix variable scope
- Add missing initialization

### Step 7: Verify the Fix

```bash
# Run the originally failing test/scenario
npm test -- --grep "failing test"

# Run related tests
npm test -- --grep "related feature"

# Run full test suite to check for regressions
npm test
```

**Verification criteria:**
- Original issue no longer occurs
- Related tests still pass
- No new errors introduced

## Failure Handling

| Situation | Action |
|-----------|--------|
| Cannot reproduce | Check environment, add logging to narrow down |
| Multiple bugs intertwined | Fix one at a time, verify after each |
| Fix causes new failures | Revert, analyze dependencies, try different approach |
| Root cause is in dependency | Check for updates, file issue, implement workaround |
| Bug is in async code | Add proper await, check Promise chains |

## MCP Tools

- `mcp__plugin_aide_aide__code_search` - Find functions, classes, types involved in the bug
- `mcp__plugin_aide_aide__code_symbols` - Understand file structure
- `mcp__plugin_aide_aide__code_references` - Find all callers of a function
- `mcp__plugin_aide_aide__memory_search` - Check for related past issues

## Output Format

```markdown
## Debug Report: [Issue Title]

### Problem
[What was happening vs what should happen]

### Reproduction
[Steps to reproduce the issue]

### Root Cause
[Identified cause with file:line reference]
[Why the bug occurred]

### Fix Applied
[What was changed and why]

### Verification
- Original issue: FIXED
- Related tests: PASS
- Full suite: PASS

### Prevention
[Optional: How to prevent similar bugs]
```

## Tips

- Always get the full error message and stack trace first
- Don't guess - trace the code path methodically
- One fix at a time - don't bundle unrelated changes
- Remove temporary logging before committing
- Consider if the bug could occur elsewhere
