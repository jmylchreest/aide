---
name: swarm
description: Parallel agents with SDLC pipeline per story
triggers:
  - swarm
  - parallel agents
  - spawn agents
  - multi-agent
---

# Swarm Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Launch parallel agents, each working on a story through SDLC stages.

## NON-NEGOTIABLE REQUIREMENTS

1. **SDLC Pipeline is MANDATORY** - Every story MUST go through all 5 stages: DESIGN → TEST → DEV → VERIFY → DOCS
2. **Git Worktrees are MANDATORY** - Each story agent MUST work in an isolated worktree
3. **VERIFY failures trigger BUILD-FIX loop** - If VERIFY fails, invoke `/aide:build-fix` and re-verify until passing
4. **Swarm MUST conclude with `/aide:worktree-resolve`** - All story branches must be merged before completion

## Activation

```
swarm 3                              → 3 story agents (SDLC mode)
swarm stories "Auth" "Payments"      → Named stories
swarm 2 --flat                       → Flat task mode (legacy)
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│  ORCHESTRATOR (you)                                                      │
│  1. Decompose work into stories                                          │
│  2. Create worktree per story                                           │
│  3. Spawn story agent per worktree (subagent_type: general-purpose)     │
│  4. Monitor progress via TaskList (DO NOT create tasks yourself!)       │
│  5. Call /aide:worktree-resolve when all stories complete               │
└─────────────────────────────────────────────────────────────────────────┘
                              │
       ┌──────────────────────┼──────────────────────┐
       ▼                      ▼                      ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│ Story A Agent   │   │ Story B Agent   │   │ Story C Agent   │
│ (worktree-a)    │   │ (worktree-b)    │   │ (worktree-c)    │
├─────────────────┤   ├─────────────────┤   ├─────────────────┤
│ SDLC Pipeline:  │   │ SDLC Pipeline:  │   │ SDLC Pipeline:  │
│ [DESIGN]        │   │ [DESIGN]        │   │ [DESIGN]        │
│ [TEST]          │   │ [TEST]          │   │ [TEST]          │
│ [DEV]           │   │ [DEV]           │   │ [DEV]           │
│ [VERIFY]        │   │ [VERIFY]        │   │ [VERIFY]        │
│ [DOCS]          │   │ [DOCS]          │   │ [DOCS]          │
└─────────────────┘   └─────────────────┘   └─────────────────┘
        │                     │                     │
        └─────────────────────┼─────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  SHARED NATIVE TASKS (Claude Code TaskList)                             │
│  All agents can see, create, and update tasks                           │
│  Dependencies via blockedBy auto-manage stage ordering                  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Workflow

### 1. Story Decomposition

Break the work into independent stories/features:

```markdown
## Stories

1. **Auth Module** - User authentication with JWT
2. **Payment Processing** - Stripe integration for subscriptions
3. **User Dashboard** - Profile management UI
```

Each story should be:
- Independent (can be developed in parallel)
- Complete (has clear boundaries)
- Testable (has acceptance criteria)

### 2. Create Git Worktrees

Each story agent gets an isolated workspace. Create worktrees using git commands:

```bash
git worktree add .aide/worktrees/story-auth -b feat/story-auth
git worktree add .aide/worktrees/story-payments -b feat/story-payments
git worktree add .aide/worktrees/story-dashboard -b feat/story-dashboard
```

**Automatic Integration:**
- Worktrees in `.aide/worktrees/` are **auto-discovered** by AIDE hooks
- When agents spawn, their worktree path is **auto-injected** into context
- When agents complete, worktrees are marked as **"agent-complete"** (ready for merge)
- Worktree state is tracked in `.aide/state/worktrees.json`

**Naming Convention:**
- Use `story-<name>` as worktree directory name
- Use matching `agent-<name>` as agent_id when spawning
- Example: `story-auth` worktree → spawn with `agent_id` containing "auth"

**If worktree creation fails:**
1. Check if branch exists: `git branch -a | grep feat/story-auth`
2. Remove stale worktree: `git worktree remove .aide/worktrees/story-auth --force`
3. Prune refs: `git worktree prune`
4. Retry creation

### 3. Spawn Story Agents

Launch agents using the Task tool with `subagent_type: "general-purpose"` (required for Edit/Write access).

**IMPORTANT: Task Ownership**
- The ORCHESTRATOR does NOT create SDLC tasks
- Each SUBAGENT creates and manages its OWN tasks
- Orchestrator only monitors via `TaskList`

Each agent manages its own SDLC pipeline.

```typescript
Task({
  subagent_type: "general-purpose",
  prompt: `You are a story agent working on: Auth Module

Worktree: /path/to/.aide/worktrees/story-auth
Story ID: story-auth
Agent ID: agent-auth

## Your Mission
Implement the Auth Module through the full SDLC pipeline.

## SDLC Pipeline
You will create and execute these stages IN ORDER:

### Stage 1: DESIGN
Use the /aide:design skill (or follow its workflow).
Output: Technical design with interfaces, decisions, acceptance criteria.

### Stage 2: TEST
Use the /aide:test skill.
Write failing tests based on acceptance criteria from DESIGN.

### Stage 3: DEV
Use the /aide:implement skill.
Make all tests pass with minimal implementation.

### Stage 4: VERIFY
Use the /aide:verify skill.
Run full test suite, lint, type check. Must all pass.

**IF VERIFY FAILS:**
1. Invoke /aide:build-fix to address failures
2. Re-run /aide:verify
3. Repeat until VERIFY passes
4. Only then proceed to DOCS

### Stage 5: DOCS
Use the /aide:docs skill.
Update documentation to match implementation.

## Task Management
Use native Claude Code task tools to track your progress:

1. Create all stage tasks upfront:
   TaskCreate: "[story-auth][DESIGN] Design auth module"
   TaskCreate: "[story-auth][TEST] Write auth tests" (blockedBy: DESIGN task)
   TaskCreate: "[story-auth][DEV] Implement auth" (blockedBy: TEST task)
   TaskCreate: "[story-auth][VERIFY] Verify auth" (blockedBy: DEV task)
   TaskCreate: "[story-auth][DOCS] Document auth" (blockedBy: VERIFY task)

2. As you start each stage:
   TaskUpdate: taskId=X, status=in_progress, owner=agent-auth

3. As you complete each stage:
   TaskUpdate: taskId=X, status=completed

## Coordination
- Share discoveries: `aide memory add --category=discovery "..."` (CLI write)
- Record decisions: `aide decision set "<topic>" "<decision>"` (CLI write)
- Check decisions: `mcp__plugin_aide_aide__decision_get` (MCP read)

## Completion
When all 5 stages are complete:
1. Verify all tasks show completed
2. Ensure all changes are committed
3. Report: "Story complete: Auth Module"
`
});
```

### 4. Monitor Progress

Use TaskList to see all story progress:

```bash
TaskList
```

Example output:
```
#10 [completed] [story-auth][DESIGN] Design auth module (agent-auth)
#11 [completed] [story-auth][TEST] Write auth tests (agent-auth)
#12 [in_progress] [story-auth][DEV] Implement auth (agent-auth)
#13 [pending] [story-auth][VERIFY] Verify auth [blocked by #12]
#14 [pending] [story-auth][DOCS] Document auth [blocked by #13]
#20 [in_progress] [story-payments][DESIGN] Design payment module (agent-payments)
```

### 5. Merge Results

When all stories complete, use `/aide:worktree-resolve`:

```bash
# Verify all tasks complete
TaskList  # Should show all [completed]

# Check for blockers (use MCP tool)
mcp__plugin_aide_aide__memory_list with category=blocker

# Merge worktrees
/aide:worktree-resolve
```

## SDLC Stage Reference

| Stage | Skill | Creates | Depends On |
|-------|-------|---------|------------|
| DESIGN | `/aide:design` | Interfaces, decisions, acceptance criteria | - |
| TEST | `/aide:test` | Failing tests | DESIGN |
| DEV | `/aide:implement` | Passing implementation | TEST |
| VERIFY | `/aide:verify` | Quality validation | DEV |
| DOCS | `/aide:docs` | Updated documentation | VERIFY |

### VERIFY → BUILD-FIX Loop

```
                    ┌──────────────┐
                    │    VERIFY    │
                    └──────┬───────┘
                           │
              ┌────────────┴────────────┐
              │                         │
           PASS                       FAIL
              │                         │
              ▼                         ▼
         ┌────────┐              ┌──────────────┐
         │  DOCS  │              │  BUILD-FIX   │
         └────────┘              └──────┬───────┘
                                        │
                                        └──────► back to VERIFY
```

If VERIFY fails:
1. `/aide:build-fix` to fix issues
2. Re-run `/aide:verify`
3. Repeat until passing
4. Then proceed to DOCS

## Story Agent Instructions Template

When spawning story agents, include:

```markdown
You are story agent [AGENT-ID] working in worktree [PATH].

## Story
[Story name and description]

## SDLC Pipeline

Execute these stages in order. For each stage:
1. Create task with TaskCreate (set blockedBy for dependencies)
2. Claim task with TaskUpdate (owner=your-id, status=in_progress)
3. Execute stage using appropriate skill
4. Mark complete with TaskUpdate (status=completed)

### Stage Tasks to Create

TaskCreate({
  subject: "[STORY-ID][DESIGN] Design [feature]",
  description: "Technical design with interfaces and acceptance criteria",
  activeForm: "Designing [feature]"
})

TaskCreate({
  subject: "[STORY-ID][TEST] Write tests for [feature]",
  description: "Failing tests based on acceptance criteria",
  activeForm: "Writing tests"
})
// ... set blockedBy to DESIGN task ID

TaskCreate({
  subject: "[STORY-ID][DEV] Implement [feature]",
  description: "Make tests pass with minimal code",
  activeForm: "Implementing [feature]"
})
// ... set blockedBy to TEST task ID

TaskCreate({
  subject: "[STORY-ID][VERIFY] Verify [feature]",
  description: "Full test suite, lint, type check",
  activeForm: "Verifying [feature]"
})
// ... set blockedBy to DEV task ID

TaskCreate({
  subject: "[STORY-ID][DOCS] Document [feature]",
  description: "Update documentation",
  activeForm: "Documenting [feature]"
})
// ... set blockedBy to VERIFY task ID

## Coordination

- Check existing decisions: `mcp__plugin_aide_aide__decision_get` (MCP read)
- Record new decisions: `aide decision set <topic> "<decision>"` (CLI write)
- Share discoveries: `aide memory add --category=discovery "<finding>"` (CLI write)
- Report blockers: `aide memory add --category=blocker "<issue>"` (CLI write)

## VERIFY Failure Handling

If VERIFY stage fails:
1. DO NOT proceed to DOCS
2. Invoke /aide:build-fix to address failures
3. Re-run /aide:verify
4. Repeat until VERIFY passes
5. Only then proceed to DOCS

## Completion

All stages must complete. When done:
1. All 5 tasks show [completed]
2. VERIFY must have passed (not skipped)
3. All changes committed to your worktree branch
4. Report: "Story [STORY-ID] complete - ready for merge"
```

## Flat Mode (Legacy)

For non-code tasks or simple work, use `--flat`:

```
swarm 3 --flat
```

This uses the original task-grabbing model without SDLC stages.

## Coordination via aide

**Decisions** (shared across agents):
```bash
# Write (CLI)
aide decision set "auth-strategy" "JWT with refresh tokens"

# Read (MCP) - use mcp__plugin_aide_aide__decision_get with topic="auth-strategy"
```

**Memory** (shared discoveries):
```bash
aide memory add --category=discovery "User model needs email validation"
```

**Messages** (direct communication):
```bash
aide message send "Auth module ready for integration" --from=agent-auth
```

## Completion (MANDATORY STEPS)

Swarm completion checklist - ALL REQUIRED:

### Step 1: Verify All Stories Complete
```
TaskList  # All story tasks must show [completed]
```
- Every story must have completed all 5 SDLC stages
- No tasks should be [pending] or [in_progress]

### Step 2: Check for Blockers
Use `mcp__plugin_aide_aide__memory_list` with category=blocker
- If blockers exist, resolve them before proceeding
- Use `/aide:build-fix` for any remaining build/test issues

### Step 3: Final Verification
Run verification on each worktree:
```bash
cd .aide/worktrees/story-X && npm test && npm run build
```
- If any fail, invoke `/aide:build-fix` and re-verify

### Step 4: Merge Worktrees (MANDATORY)
**YOU MUST invoke `/aide:worktree-resolve`** - this is not optional.
```
/aide:worktree-resolve
```
This skill will:
- Merge each story branch into main
- Handle any merge conflicts
- Clean up worktrees

### Step 5: Record Session
Only after successful merge, record the swarm session (see Orchestrator Memory below).

## Orchestrator Memory

After swarm completes, record the session:

```xml
<aide-memory category="session" tags="swarm,sdlc">
## Swarm: [Brief Description]

### Stories Completed
- Story A: [outcome]
- Story B: [outcome]
- Story C: [outcome]

### Key Decisions Made
- [decision]: [rationale]

### Files Changed
- [summary of changes per story]

### Merge Status
- [branches merged successfully / any conflicts]
</aide-memory>
```
