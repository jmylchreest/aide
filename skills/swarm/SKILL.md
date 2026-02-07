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

Launch parallel agents, each working on a story through SDLC stages.

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
│  3. Spawn story agent per worktree                                      │
│  4. Monitor progress via TaskList                                       │
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

Each story agent gets an isolated workspace:

```bash
git worktree add .aide/worktrees/story-auth -b feat/story-auth
git worktree add .aide/worktrees/story-payments -b feat/story-payments
git worktree add .aide/worktrees/story-dashboard -b feat/story-dashboard
```

**If worktree creation fails:**
1. Check if branch exists: `git branch -a | grep feat/story-auth`
2. Remove stale worktree: `git worktree remove .aide/worktrees/story-auth --force`
3. Prune refs: `git worktree prune`
4. Retry creation

### 3. Spawn Story Agents

Launch agents using the Task tool. Each agent manages its own SDLC pipeline.

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
- Share discoveries: aide memory add --category=discovery "..."
- Record decisions: aide decision set "<topic>" "<decision>"
- Check decisions: aide decision get "<topic>"

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

# Check for blockers
aide memory list --category=blocker

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

- Check existing decisions: aide decision get <topic>
- Record new decisions: aide decision set <topic> "<decision>"
- Share discoveries: aide memory add --category=discovery "<finding>"
- Report blockers: aide memory add --category=blocker "<issue>"

## Completion

All stages must complete. When done:
1. All 5 tasks show [completed]
2. All changes committed to your worktree branch
3. Report: "Story [STORY-ID] complete"
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
aide decision set "auth-strategy" "JWT with refresh tokens"
aide decision get "auth-strategy"
```

**Memory** (shared discoveries):
```bash
aide memory add --category=discovery "User model needs email validation"
```

**Messages** (direct communication):
```bash
aide message send "Auth module ready for integration" --from=agent-auth
```

## Completion

Swarm is complete when:
1. All story tasks show [completed] in TaskList
2. No unresolved blockers: `aide memory list --category=blocker`
3. All worktrees have committed changes

**IMPORTANT:** When complete, automatically invoke `/aide:worktree-resolve` to merge all story branches.

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
