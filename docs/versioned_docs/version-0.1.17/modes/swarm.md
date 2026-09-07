---
sidebar_label: Swarm Mode
sidebar_position: 2
title: Swarm Mode
---

# Swarm Mode

Swarm mode spawns parallel agents, each with its own git worktree and SDLC pipeline. It's designed for decomposing large features into independent stories that can be implemented concurrently.

## Architecture

```
+--------------------------------------------------------------------------+
|  ORCHESTRATOR                                                            |
|  1. Decompose work into stories                                          |
|  2. Create worktree per story                                            |
|  3. Spawn story agent per worktree                                       |
|  4. Monitor progress                                                     |
+--------------------------------------------------------------------------+
                              |
       +----------------------+----------------------+
       v                      v                      v
+-----------------+   +-----------------+   +-----------------+
| Story A Agent   |   | Story B Agent   |   | Story C Agent   |
| (worktree-a)    |   | (worktree-b)    |   | (worktree-c)    |
+-----------------+   +-----------------+   +-----------------+
| SDLC Pipeline:  |   | SDLC Pipeline:  |   | SDLC Pipeline:  |
| [DESIGN]        |   | [DESIGN]        |   | [DESIGN]        |
| [TEST]          |   | [TEST]          |   | [TEST]          |
| [DEV]           |   | [DEV]           |   | [DEV]           |
| [VERIFY]        |   | [VERIFY]        |   | [VERIFY]        |
| [DOCS]          |   | [DOCS]          |   | [DOCS]          |
+-----------------+   +-----------------+   +-----------------+
```

Each story agent works in isolation on its own branch, executing a full software development lifecycle.

## Activation

```
swarm 3                              # 3 story agents (SDLC mode)
swarm stories "Auth" "Payments"      # Named stories
swarm 2 --flat                       # Flat task mode (legacy)
```

## SDLC Pipeline

Each story agent executes these stages in order:

| Stage      | Skill             | Purpose                                         |
| ---------- | ----------------- | ----------------------------------------------- |
| **DESIGN** | `/aide:design`    | Technical spec, interfaces, acceptance criteria |
| **TEST**   | `/aide:test`      | Write failing tests (TDD)                       |
| **DEV**    | `/aide:implement` | Make tests pass                                 |
| **VERIFY** | `/aide:verify`    | Full QA validation                              |
| **DOCS**   | `/aide:docs`      | Update documentation                            |

Agents send `status` messages at each stage transition, making progress visible to the orchestrator and other agents.

## Workflow

### 1. Plan (Optional)

Use the `plan-swarm` skill to decompose work through a Socratic interview:

```
plan the dashboard feature
```

This produces validated, independent stories with clear boundaries.

### 2. Decompose

The orchestrator breaks work into independent stories. Each story should be:

- **Self-contained** — Can be implemented without depending on other stories
- **Testable** — Has clear acceptance criteria
- **Bounded** — Small enough for a single agent to complete

### 3. Create Worktrees

Each story gets an isolated git worktree:

```
.aide/worktrees/story-auth/       → branch: feat/story-auth
.aide/worktrees/story-payments/   → branch: feat/story-payments
.aide/worktrees/story-dashboard/  → branch: feat/story-dashboard
```

### 4. Execute

Each agent runs its SDLC pipeline independently. Agents can coordinate through:

- **Decisions** — `aide decision set/get` for architectural choices visible to all
- **Memory** — `aide memory add` for discoveries and learnings
- **Messages** — `aide message send` for direct inter-agent communication
- **Tasks** — `aide task create/claim/complete` for work tracking

### 5. Monitor

Track progress through:

```bash
aide task list                    # All tasks with status
aide message list --agent=orch   # Messages to orchestrator
aide state list                  # All agent states
```

### 6. Merge

When all stories complete, use the `worktree-resolve` skill to merge:

```
merge the swarm branches
```

Or manually:

```bash
git worktree list
git merge feat/story-auth
git merge feat/story-payments
git worktree remove .aide/worktrees/story-auth
git worktree remove .aide/worktrees/story-payments
```

## Agent Coordination

### Shared State

All agents share the same AIDE data store, enabling real-time coordination:

| Mechanism     | Use Case              | Example                                       |
| ------------- | --------------------- | --------------------------------------------- |
| **Decisions** | Architectural choices | "Use JWT for auth" — all agents see this      |
| **Messages**  | Direct communication  | "User model is ready" from agent A to agent B |
| **State**     | Status tracking       | Per-agent mode, stage, progress               |
| **Tasks**     | Work tracking         | Create, claim, complete tasks                 |

### Message Protocol

Agents follow a messaging protocol:

| Type         | When                                       |
| ------------ | ------------------------------------------ |
| `status`     | Stage transitions (DESIGN → TEST → DEV...) |
| `request`    | Asking another agent for information       |
| `response`   | Replying to a request                      |
| `blocker`    | Reporting something blocking progress      |
| `completion` | Signaling work is done                     |

### Tool Enforcement

In swarm mode, agent roles determine tool access:

- **Architect/Explorer/Researcher** agents are restricted to read-only tools
- **Executor** agents have full tool access
- This prevents conflicts when multiple agents work on the same codebase

## Platform Differences

| Feature               | Claude Code             | OpenCode               |
| --------------------- | ----------------------- | ---------------------- |
| Agent spawning        | Native (subagent hooks) | External orchestration |
| Worktree management   | Automatic               | Automatic              |
| SDLC pipeline         | Full                    | Full                   |
| Inter-agent messaging | Full                    | Full                   |
| Progress monitoring   | HUD + tasks             | Tasks only             |
