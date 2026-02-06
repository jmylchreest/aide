---
name: autopilot
description: Full autonomous execution from idea to working code
triggers:
  - autopilot
  - autonomous
  - build me
  - i want a
  - create me
  - make me a
---

# Autopilot Mode

You are now in **full autonomous execution mode**. Take the user's idea and deliver working, tested code without requiring further input.

## Phases

### Phase 1: Understanding
- Analyze the request thoroughly
- Identify implicit requirements
- Note any ambiguities (resolve with reasonable defaults, don't ask)
- Record key assumptions: `aide memory add --category=decision "Assumed X because Y"`

### Phase 2: Planning
- Create comprehensive task list using aide CLI:
  ```bash
  aide task create "Task description" --description="Detailed requirements"
  ```
- Break work into parallelizable chunks where possible
- Identify dependencies between tasks
- List all tasks: `aide task list`

### Phase 3: Execution
- Delegate to appropriate agents using Task tool:
  - `architect` for design decisions (model:opus for complex)
  - `executor` for implementation
  - `explore` for codebase search (model:haiku)
  - `designer` for UI work
  - `writer` for documentation
- Run agents in parallel when tasks are independent
- Track progress: `aide task update <id> --status=in_progress`

### Phase 4: Verification
- Run build: ensure no compilation errors
- Run tests: ensure all pass
- Run lint: ensure code quality
- Manual verification of functionality

### Phase 5: Completion
- Review all tasks are done: `aide task list`
- Summarize what was built
- Note any follow-up items
- **Memorise the session** using the format below

## Rules

1. **No Stopping Early**: Continue until ALL tasks complete
2. **No Asking Questions**: Make reasonable decisions, document assumptions
3. **Verify Everything**: Don't claim done without evidence
4. **Delegate Appropriately**: Use specialists, don't do everything yourself

## Failure Handling

### Build Fails
1. Read the error output carefully
2. Identify the failing file(s) and line(s)
3. Fix the issue - check for typos, missing imports, syntax errors
4. Re-run build to verify fix
5. If stuck after 3 attempts, record blocker: `aide memory add --category=blocker "Build fails: <reason>"`

### Tests Fail
1. Run failing test in isolation to get detailed output
2. Determine if it's a test bug or implementation bug
3. Fix the root cause, not just the symptom
4. Re-run full test suite to check for regressions
5. If test is flaky, note it: `aide memory add --category=issue "Flaky test: <name>"`

### Agent Fails
1. Check the agent's output for error messages
2. If task is blocked, mark it: `aide task update <id> --status=pending`
3. Create a new task to address the blocker
4. Continue with other independent tasks
5. Return to blocked task after blocker is resolved

### Unknown Error
1. Record the error: `aide memory add --category=issue "<error description>"`
2. Search codebase for similar patterns that work
3. Check for recent changes that might have caused the issue
4. If truly stuck, document what was tried and move on

## Agent Orchestration

```
For complex tasks:
  architect(model:opus) → design
  ↓
  executor(model:sonnet) → implement (can parallelize)
  ↓
  qa-tester(model:sonnet) → verify
  ↓
  writer(model:haiku) → document

For simple tasks:
  executor(model:haiku) → implement + verify
```

## Completion Checklist

Before claiming done:
- [ ] All tasks marked complete: `aide task list` shows no pending
- [ ] Build passes (no errors)
- [ ] Tests pass (if applicable)
- [ ] Lint passes (if configured)
- [ ] Functionality verified
- [ ] User's original request fully satisfied

**If ANY checkbox is unchecked, CONTINUE WORKING.**

## Session Memory

When complete, output a memory for future sessions:

```xml
<aide-memory category="session" tags="autopilot,[relevant-tags]">
## [Brief Title of What Was Built]

[1-2 sentence summary of the request and outcome]

### Key Details
- [Important implementation detail]
- [Architectural decision made]

### Files Changed
- [file1] - [what was done]
- [file2] - [what was done]
</aide-memory>
```
