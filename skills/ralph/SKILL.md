---
name: ralph
description: Ralph Wiggum methodology - iterative implementation with test-driven backpressure
triggers:
  - ralph
  - persist
  - persistence
  - don't stop
  - dont stop
  - until done
  - must complete
  - relentless
  - ralph wiggum
---

# Ralph Mode (Ralph Wiggum Methodology)

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

You are now in **Ralph Wiggum mode** - an iterative development methodology that uses test-driven backpressure and aide-based state persistence.

## Core Principles

1. **Planning vs Building**: Separate phases with distinct behaviors
2. **Backpressure via Tests**: Cannot proceed until tests pass
3. **Task Atomicity**: One task per iteration
4. **Don't Assume**: Verify gaps exist before implementing
5. **aide-Based Persistence**: Tasks, state, and decisions stored in aide (not files)
6. **Swarm Compatible**: Multiple agents can work in parallel

---

## State Management

All state is managed through aide. Use MCP tools for reads, CLI for writes:

### Task System Roles

| System                      | Role                                                    | How                                                                   |
| --------------------------- | ------------------------------------------------------- | --------------------------------------------------------------------- |
| **aide tasks** (MCP or CLI) | Durable task backlog, claiming, persistence enforcement | `task_create`/`task_claim`/`task_complete` (MCP) or `aide task` (CLI) |
| **Native todowrite**        | Personal progress tracking within current iteration     | `todowrite` tool — tracks sub-steps of current task                   |

aide tasks are the source of truth for ralph — they survive session restarts and are checked by persistence hooks to block premature stopping. Use native `todowrite` for your own step-by-step checklist within each task iteration.

### Reads (MCP Tools)

| Tool                                   | Purpose              |
| -------------------------------------- | -------------------- |
| `mcp__plugin_aide_aide__state_get`     | Get phase, objective |
| `mcp__plugin_aide_aide__state_list`    | List all state       |
| `mcp__plugin_aide_aide__decision_get`  | Get decisions        |
| `mcp__plugin_aide_aide__decision_list` | List all decisions   |
| `mcp__plugin_aide_aide__memory_search` | Search discoveries   |
| `task_list`                            | List aide tasks      |
| `task_get`                             | Get task by ID       |

### Writes (CLI via Bash or MCP)

```bash
# Phase tracking
./.aide/bin/aide state set ralph:phase planning   # or "building"

# Task management — use aide tasks (persistent, claimable)
# Via MCP: task_create, task_claim, task_complete
# Via CLI: ./.aide/bin/aide task create/claim/complete

# Decisions
./.aide/bin/aide decision set <topic> "<decision>" --rationale="<why>"

# Gap analysis / discoveries
./.aide/bin/aide memory add --category=discovery --tags=ralph "Gap found: <description>"
```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

---

## Phase 1: Planning Mode

When starting a new task or when `./.aide/bin/aide state get ralph:phase` is empty/planning:

### Step 1: Set Phase

```bash
./.aide/bin/aide state set ralph:phase planning
./.aide/bin/aide state set ralph:objective "<what we're building>"
```

### Step 2: Gap Analysis (Don't Assume!)

**CRITICAL**: Before assuming anything needs implementation, SEARCH THE CODE:

```bash
# Search for existing implementations
rg "functionName\|ClassName\|feature" --type ts

# Check existing tests
rg "describe.*feature\|it.*should" --type ts
```

Record findings:

```bash
./.aide/bin/aide memory add --category=discovery --tags=ralph,gap-analysis "Searched for X: <results>"
```

Only after confirming gaps exist, proceed to task creation.

### Step 3: Create Tasks

Create atomic, testable tasks:

```bash
./.aide/bin/aide task create "Implement user model" --tags=ralph,task-1
./.aide/bin/aide task create "Add validation to user model" --tags=ralph,task-2
./.aide/bin/aide task create "Write user model tests" --tags=ralph,task-3
```

Each task should be:

- Small enough to complete in one iteration
- Independently testable
- Clearly defined acceptance criteria

### Step 4: Record Key Decisions

```bash
./.aide/bin/aide decision set ralph:test-framework "vitest" --rationale="Already configured in project"
./.aide/bin/aide decision set ralph:approach "<approach>" --rationale="<why>"
```

### Step 5: Exit Planning

```bash
./.aide/bin/aide state set ralph:phase building
```

Report the plan:

- List tasks: `./.aide/bin/aide task list`
- List decisions: `./.aide/bin/aide decision list`

**DO NOT implement during planning phase.**

---

## Phase 2: Building Mode

When `./.aide/bin/aide state get ralph:phase` returns "building":

### Iteration Loop

Each iteration follows this exact sequence:

#### 1. Load Context

```bash
# Check current phase and objective
./.aide/bin/aide state get ralph:phase
./.aide/bin/aide state get ralph:objective

# List tasks to find next one
./.aide/bin/aide task list

# Check existing decisions
./.aide/bin/aide decision list
```

#### 2. Select Next Task

Find the first pending task:

```bash
./.aide/bin/aide task list  # Look for [pending] status
```

Claim it:

```bash
./.aide/bin/aide task claim <task-id> --agent=ralph
```

#### 3. Verify Gap Still Exists (Don't Assume!)

Before implementing, RE-VERIFY:

```bash
# Search again - someone may have implemented it
rg "featureName" --type ts
```

If gap no longer exists:

```bash
./.aide/bin/aide task complete <task-id>
# Proceed to next task
```

#### 4. Write Tests First

Create or update test file with failing tests:

```bash
# Run tests - they MUST fail initially
npm test -- path/to/test.test.ts
```

If tests pass without implementation, the gap analysis was wrong - complete the task and move on.

#### 5. Implement Solution

Write minimal code to make tests pass.

#### 6. Backpressure Checkpoint (REQUIRED)

**You CANNOT proceed until this passes:**

```bash
npm test -- path/to/test.test.ts
```

**BLOCKING RULE**: If tests fail, you MUST:

1. Analyze the failure
2. Fix the issue
3. Re-run tests
4. Repeat until passing

**DO NOT skip failing tests. DO NOT proceed with failing tests.**

#### 7. Complete Task

```bash
./.aide/bin/aide task complete <task-id>
```

#### 8. Atomic Commit

```bash
git add -A
git commit -m "feat: <task description> - tests passing"
```

#### 9. Check Completion

```bash
./.aide/bin/aide task list
```

If more pending tasks: continue to next iteration (step 2)
If all complete: run full verification

---

## Failure Handling

### Test Failures

When tests fail during backpressure checkpoint:

1. **DO NOT** proceed to next task
2. **DO NOT** skip the failing test
3. **DO** analyze the error message
4. **DO** fix and re-run until passing

Record blockers:

```bash
./.aide/bin/aide memory add --category=blocker --tags=ralph "Test failure: <description>"
```

### Stuck Conditions

If blocked for more than 3 attempts:

```bash
./.aide/bin/aide memory add --category=blocker --tags=ralph,needs-help "Stuck on: <description>"
```

Then ask user for guidance. **DO NOT** proceed without resolution.

---

## Full Verification Protocol

Before claiming completion, ALL must pass:

```bash
# 1. All tasks complete
./.aide/bin/aide task list  # Should show all [done]

# 2. All tests
npm test

# 3. Build
npm run build

# 4. Lint
npm run lint
```

Only proceed to completion when ALL verification passes.

---

## Completion

When all tasks complete and verification passes:

### Update State

```bash
./.aide/bin/aide state set ralph:phase complete
./.aide/bin/aide state set ralph:result "success"
```

### Record Session

```bash
./.aide/bin/aide memory add --category=session --tags=ralph,implementation "
## <Feature Name> Complete

Implemented using Ralph Wiggum methodology.

### Tasks Completed
- Task 1: <description>
- Task 2: <description>

### Verification
- Tests: passing
- Build: passing
- Lint: clean

### Key Decisions
- <decision>: <rationale>
"
```

---

## Anti-Patterns (AVOID)

- "I've made good progress, let me summarize..." (KEEP WORKING)
- "The main work is done, you can finish..." (VERIFY FIRST)
- "I'll skip this failing test for now..." (FIX IT NOW)
- "I assume this needs to be implemented..." (SEARCH FIRST)
- "I'll implement everything then test..." (TEST EACH TASK)
- Proceeding with red tests (NEVER)
- Implementing during planning phase (SEPARATE PHASES)
- Large commits with multiple tasks (ONE TASK PER COMMIT)

---

## Commands

- `ralph` or `ralph plan` - Start planning phase
- `ralph build` - Start building phase (requires tasks exist)
- `ralph status` - Show current state via aide
- `cancel` or `stop ralph` - Exit ralph mode

---

## Quick Reference

```
PLANNING PHASE:
1. ./.aide/bin/aide state set ralph:phase planning
2. Search code (don't assume!)
3. ./.aide/bin/aide memory add findings
4. ./.aide/bin/aide task create (atomic tasks)
5. ./.aide/bin/aide decision set (key decisions)
6. ./.aide/bin/aide state set ralph:phase building

BUILDING PHASE (per task):
1. ./.aide/bin/aide task list (find next)
2. ./.aide/bin/aide task claim <id>
3. Re-verify gap exists
4. Write failing tests
5. Implement
6. BACKPRESSURE: Tests MUST pass
7. ./.aide/bin/aide task complete <id>
8. Atomic commit
9. Repeat or verify completion
```

---

## Swarm Compatibility

This skill is **swarm-compatible**. Multiple ralph agents can:

- Work on different tasks in parallel
- Share discoveries via `./.aide/bin/aide memory`
- Check decisions via `./.aide/bin/aide decision get`
- Claim tasks atomically via `./.aide/bin/aide task claim`

No file conflicts because all state is in aide's database.

---

## Phase 3: Final QA (Swarm Mode)

**MANDATORY** when ralph runs with swarm (multiple agents). After all tasks show `[done]`:

### Step 1: Spawn QA Agent

The orchestrator spawns a **single** QA subagent:

```
Spawn a final QA agent with instructions:
"You are the QA agent for a ralph swarm session. Your job is NOT to trust the task list.
Instead, independently verify the implementation against the original objective."
```

### Step 2: QA Agent Workflow

The QA agent must:

#### a) Load the Objective (not the task list)

```bash
./.aide/bin/aide state get ralph:objective
```

#### b) Independent Verification

**Ignore the task list.** Instead, verify from first principles:

1. **Read the code** - Does it implement the objective?
2. **Check for gaps** - Are there missing pieces the tasks didn't cover?
3. **Run full test suite** - Not just individual task tests
   ```bash
   npm test
   npm run build
   npm run lint
   ```
4. **Integration check** - Does it work as a whole?

#### c) Find & Fix Gaps

If gaps are found:

```bash
# Record the gap
./.aide/bin/aide memory add --category=discovery --tags=ralph,qa "QA found gap: <description>"

# Create fix task
./.aide/bin/aide task create "QA fix: <description>" --tags=ralph,qa-fix

# Implement the fix (follow standard backpressure rules)
# ...

# Mark complete
./.aide/bin/aide task complete <id>
```

#### d) Final Sign-off

Only when QA agent confirms:

- All tests passing
- Build clean
- Lint clean
- Objective fully met (not just tasks)

```bash
./.aide/bin/aide state set ralph:qa "passed"
./.aide/bin/aide state set ralph:phase complete
```

### Step 3: QA Failure Handling

If QA finds unfixable issues:

```bash
./.aide/bin/aide state set ralph:qa "failed"
./.aide/bin/aide memory add --category=blocker --tags=ralph,qa "QA failed: <reason>"
```

Report to user with specific failures. **DO NOT** mark complete.

---

## Swarm + Ralph Workflow Summary

```
ORCHESTRATOR                    SWARM AGENTS              QA AGENT
     │                               │                        │
     ├─► Planning phase              │                        │
     │   (create tasks)              │                        │
     │                               │                        │
     ├─► Spawn N agents ────────────►│                        │
     │                               ├─► Claim tasks          │
     │                               ├─► Implement            │
     │                               ├─► Backpressure tests   │
     │                               ├─► Complete & commit    │
     │                               │                        │
     │◄── All tasks [done] ──────────┤                        │
     │                               │                        │
     ├─► Merge worktrees             │                        │
     │   (worktree-resolve)          │                        │
     │                               │                        │
     ├─► Spawn QA agent ─────────────┼───────────────────────►│
     │                               │                        ├─► Ignore task list
     │                               │                        ├─► Verify objective
     │                               │                        ├─► Fix gaps
     │                               │                        ├─► Full test suite
     │                               │                        │
     │◄── QA passed ─────────────────┼────────────────────────┤
     │                               │                        │
     └─► Mark complete               │                        │
```

The QA phase ensures swarm work is **truly complete**, not just task-list complete.
