---
name: swarm
description: Parallel agents with shared memory
triggers:
  - swarm
  - parallel agents
  - spawn agents
  - multi-agent
---

# Swarm Mode

Launch N parallel agents with shared memory coordination.

## Activation

```
swarm 3                    → 3 agents
swarm 5 executors          → 5 executor agents
swarm mixed                → auto-select agent types
```

## Architecture

```
         ┌─────────────────────────────┐
         │      SHARED MEMORY          │
         │  (aide database)            │
         │  • Tasks • Decisions        │
         │  • Discoveries • Messages   │
         └─────────────┬───────────────┘
                       │
       ┌───────────────┼───────────────┐
       │               │               │
  ┌────┴────┐    ┌────┴────┐    ┌────┴────┐
  │ Agent 1 │    │ Agent 2 │    │ Agent 3 │
  │ wt-1    │    │ wt-2    │    │ wt-3    │
  └─────────┘    └─────────┘    └─────────┘
```

## Workflow

### 1. Task Decomposition
Break the work into parallelizable subtasks:
```bash
aide task create "Implement user model" --description="Create User struct with fields: id, email, password_hash"
aide task create "Implement auth routes" --description="POST /login, POST /register, GET /me"
aide task create "Write tests" --description="Unit tests for user model and auth routes"
```

Verify tasks created:
```bash
aide task list
```

### 2. Create Git Worktrees
Each agent gets isolated workspace with a feature branch:
```bash
git worktree add .aide/worktrees/task1-agent1 -b feat/task1-agent1
git worktree add .aide/worktrees/task2-agent2 -b feat/task2-agent2
git worktree add .aide/worktrees/task3-agent3 -b feat/task3-agent3
```

**If worktree creation fails:**
1. Check if branch already exists: `git branch -a | grep feat/task1`
2. Remove stale worktree: `git worktree remove .aide/worktrees/task1-agent1 --force`
3. Prune worktree refs: `git worktree prune`
4. Retry creation

### 3. Spawn Agents
Launch agents with Task tool, each assigned to a worktree.

Each agent MUST be given:
- Their worktree path
- Their agent ID
- Their assigned task ID
- Instructions to use aide CLI for coordination

### 4. Coordination via aide

**Claim tasks atomically:**
```bash
aide task claim <task-id> --agent=agent-1
```

If claim fails (task already claimed), agent should:
1. Check task list for unclaimed tasks: `aide task list`
2. Claim a different available task
3. If no tasks available, report idle and wait

**Share discoveries:**
```bash
aide memory add --category=discovery "User model needs email field"
```

**Make decisions:**
```bash
aide decision set password-hashing "bcrypt with cost 12"
```

**Check existing decisions before making new ones:**
```bash
aide decision get password-hashing
```

**Send messages:**
```bash
aide message send "User model ready" --from=agent-1
```

### 5. Review & Merge Results
When all tasks complete, merge each branch sequentially:

```bash
# List all active worktrees
git worktree list

# Review what each branch changed
git log main..feat/task1-agent1 --oneline
git diff main...feat/task1-agent1 --stat

# Merge each branch
git checkout main
git merge feat/task1-agent1 --no-edit
```

**If conflicts occur:** Act as an expert code reviewer:
1. Read the conflicted files to understand the conflict markers
2. Analyze what both code paths were trying to achieve
3. Edit the file to combine both sets of logic correctly
4. `git add <file>` and `git commit`
5. Run tests to verify the resolution

**If resolution fails** (tests fail, logic contradictory):
1. `git merge --abort` to restore clean state
2. Record failure: `aide message send "CONFLICT: Cannot merge feat/<name> - <reason>" --to=orchestrator`
3. Skip this branch and continue with others
4. Report unmerged branches in final summary

See `/aide:worktree-resolve` for detailed conflict resolution workflow.

```bash
# After all branches merged, cleanup
git worktree remove .aide/worktrees/task1-agent1
git branch -d feat/task1-agent1
git worktree prune
```

## Agent Instructions

When spawning swarm agents, include these instructions:

```
You are swarm agent-N working in an isolated worktree.

**Worktree:** <absolute-path-to-worktree>
**Task ID:** <task-id>
**Agent ID:** agent-N

## Your Workflow

1. Claim your task immediately:
   ```bash
   aide task claim <task-id> --agent=agent-N
   ```
   If claim fails, report to orchestrator and wait for reassignment.

2. Check for existing decisions that affect your work:
   ```bash
   aide decision list
   aide decision get <relevant-topic>
   ```

3. Update task status when you start:
   ```bash
   aide task update <task-id> --status=in_progress
   ```

4. Do your work in the worktree. Commit frequently with descriptive messages.

5. Share discoveries that other agents might need:
   ```bash
   aide memory add --category=discovery "<what you found>"
   ```

6. When done, verify your work:
   - Run build in your worktree
   - Run relevant tests
   - Commit all changes

7. Mark task complete:
   ```bash
   aide task complete <task-id>
   ```

## Failure Handling

If you encounter a blocker:
1. Record it: `aide memory add --category=blocker "<description>"`
2. Send message: `aide message send "BLOCKED: <reason>" --from=agent-N`
3. Check if you can work on a different task
4. Do NOT mark task complete if it's not fully working
```

## Completion

Swarm is complete when:
- All tasks marked done: `aide task list` shows all complete
- All worktrees have committed changes
- No unresolved blockers: `aide memory list --category=blocker`

**IMPORTANT:** When all swarm agents have finished, **automatically invoke `/aide:worktree-resolve`** to merge the worktrees. Do not wait for user to request this - it is part of the swarm workflow.

## Verification Before Merge

Before merging worktrees, the orchestrator must verify:
1. Check all tasks complete: `aide task list`
2. Check for blockers: `aide memory list --category=blocker`
3. If blockers exist, resolve them before merging
4. If tasks are incomplete, investigate why agents stopped

### Main Agent Memorise

The **main/orchestrating agent** should memorise the overall outcome:

```xml
<aide-memory category="session" tags="swarm,[relevant-tags]">
## [Brief Title]

Swarm task with N agents.

### Agents & Work
- Agent 1: [what they did]
- Agent 2: [what they did]
- Agent 3: [what they did]

### Outcome
[Overall result, any issues encountered]

### Files Changed
- [file] - [change]
</aide-memory>
```

### Subagent Memorise (Optional)

Subagents should only memorise **significant learnings**:

```xml
<aide-memory category="learning" tags="swarm,agent:[id]">
## [Specific Discovery/Learning]

[What was learned that would help future work]
</aide-memory>
```
