# aide TODO

## Active Work: SDLC-Enhanced Swarm

### Overview

Enhance `/aide:swarm` to use structured SDLC stages for code changes, leveraging Claude Code's native task system.

### Project Stack
- **TypeScript** (Node.js 20+): Hooks, plugins - vitest, eslint, prettier
- **Go**: aide CLI binary

---

## Completed

### 1. Skill Updates ✅

- [x] 1.1 Update `plan` → `design` skill (output-focused, not interview)
- [x] 1.2 Create `implement` skill (TDD implementation for DEV stage)
- [x] 1.3 Create `verify` skill (QA validation for VERIFY stage)
- [x] 1.4 Create `docs` skill (documentation updates for DOCS stage)
- [x] 1.5 Delete `autopilot` skill (replaced by `swarm 1` with SDLC)

### 2. Swarm Skill Enhancement ✅

- [x] 2.1 Migrate to native tasks (TaskCreate/Update/List instead of aide task)
- [x] 2.2 Add SDLC mode (story decomposition with stage dependencies)
- [x] 2.3 Update agent instructions (SDLC pipeline management)

### 4.4 Enhanced Subagent Context Injection ✅

- [x] Project memories (`project:<name>`) now injected to both session-start and subagent-start

---

## Pending Implementation

### 3. Quality Gate Hooks

#### 3.1 TeammateIdle hook
Fires when agent goes idle - validate work complete.

**File**: `src/hooks/teammate-idle.ts` (new)

**Checks**:
- All claimed tasks complete
- Tests pass in worktree
- No uncommitted changes
- No lint/type errors

**Note**: Requires `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled.

#### 3.2 TaskCompleted hook ⭐ (Ready to implement)
Fires when task marked complete - stage-specific validation.

**File**: `src/hooks/task-completed.ts` (new)

**Stage-specific checks**:
- DESIGN: Has design output
- TEST: Test files exist
- DEV: Tests pass
- VERIFY: Full suite green, lint clean
- DOCS: Docs updated

**Note**: Works WITHOUT `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`! Fires on any TaskUpdate completion.

---

### 4. Memory/Decision System Enhancements

#### 4.1 Auto Session Summary
Generate session summary automatically on session end.

**File**: `src/hooks/memory-capture.ts` (enhance Stop hook)

**Implementation**:
- On `Stop` hook, read transcript if available
- Extract: files modified, key actions, decisions made
- Store as memory with:
  - `category=session`
  - `tags=session-summary,session:<id>`
- Summary should be structured but concise

#### 4.2 Decision Capture with `<aide-decision>` Tag
Auto-capture decisions when Claude outputs structured decision.

**File**: `src/hooks/memory-capture.ts` (add decision parsing)

**Format**:
```xml
<aide-decision topic="auth-strategy">
## Decision
Use JWT with refresh tokens

## Rationale
Stateless auth, mobile client support

## Alternatives Considered
- Session cookies - too stateful
</aide-decision>
```

**Implementation**:
- Parse `<aide-decision topic="...">` in PostToolUse
- Extract topic, decision text, rationale
- Store via `aide decision set <topic> "<decision>" --rationale="<rationale>"`
- Newer decisions for same topic supersede older (append-only history)

#### 4.3 Create `/aide:decide` Skill
Formal decision-making interview for architectural choices.

**File**: `skills/decide/SKILL.md` (new)

**Flow**:
1. Interview: What decision needs to be made?
2. Explore: What are the options?
3. Analyze: Pros/cons of each
4. Recommend: Claude's recommendation
5. Confirm: User approval
6. Record: Output `<aide-decision>` for capture

#### 4.5 Session Memory Injection (Future)
Add current session memories to subagent context.

**File**: `src/hooks/subagent-tracker.ts`

**Add**:
- Current session memories (`session:<current-id>`)
- Task/story-specific memories (`task:<id>` or `story:<id>` tags)

---

## SDLC Stage → Skill Mapping

| Stage | Skill | Purpose |
|-------|-------|---------|
| DESIGN | `design` (updated from plan) | Output technical spec |
| TEST | `test` (existing) | Write failing tests |
| DEV | `implement` (new) | Make tests pass |
| VERIFY | `verify` (new) | Full QA validation |
| FIX | `build-fix` (existing) | Fix failures |
| DOCS | `docs` (new) | Update documentation |

---

## Reference

### Claude Code Agent Teams
See: https://code.claude.com/docs/en/agent-teams

Key integrations:
- Native `TaskCreate/TaskUpdate/TaskList` - shared across agents
- `TeammateIdle` hook - quality gate when agent finishes
- `TaskCompleted` hook - quality gate when task completes

### Architecture
```
ORCHESTRATOR (swarm)
    │
    ├── Story A Agent (worktree-a)
    │   └── [DESIGN] → [TEST] → [DEV] → [VERIFY] → [DOCS]
    │
    ├── Story B Agent (worktree-b)
    │   └── [DESIGN] → [TEST] → [DEV] → [VERIFY] → [DOCS]
    │
    └── SHARED NATIVE TASKS (Claude Code)
        All agents see all tasks, dependencies auto-managed
```
