# aide TODO

## Active Work: SDLC-Enhanced Swarm

### Overview

Enhance `/aide:swarm` to use structured SDLC stages for code changes, leveraging Claude Code's native task system.

### Project Stack
- **TypeScript** (Node.js 20+): Hooks, plugins - vitest, eslint, prettier
- **Go**: aide CLI binary

---

## Implementation Tasks

### 1. Skill Updates

#### 1.1 Update `plan` → `design` skill
Transform from interview-focused to design output.

**File**: `skills/plan/SKILL.md`

**Changes**:
- Rename triggers to include "design", "architect", "spec"
- Remove interview Q&A flow
- Add required outputs:
  - Interface/type definitions
  - Component interaction / data flow
  - Key decisions → `aide decision set`
  - Acceptance criteria (feeds TEST stage)
- Output format: structured design doc

#### 1.2 Create `implement` skill
Focused TDD implementation for DEV stage.

**File**: `skills/implement/SKILL.md` (new)

**Content**:
- Read failing tests from TEST stage
- Implement minimal code to pass tests
- Run tests - MUST pass before complete
- Commit atomically
- Extract from `ralph` building phase

#### 1.3 Create `verify` skill
Active QA verification for VERIFY stage.

**File**: `skills/verify/SKILL.md` (new)

**Content**:
- Run full test suite
- Run linter (`npm run lint` / `go vet`)
- Run type checker (`tsc --noEmit` / go build)
- Check for debug artifacts (console.log, TODO, debugger)
- Output: PASS/FAIL with specifics
- Different from `review` (which is read-only analysis)

#### 1.4 Create `docs` skill
Documentation updates for DOCS stage.

**File**: `skills/docs/SKILL.md` (new)

**Content**:
- Identify affected docs (README, API docs, inline)
- Read current docs + new code
- Update docs to match implementation
- Verify docs build if applicable (e.g., typedoc)

#### 1.5 Delete `autopilot` skill
Replaced by `swarm 1` with SDLC stages.

**File**: `skills/autopilot/SKILL.md` (delete)

---

### 2. Swarm Skill Enhancement

#### 2.1 Migrate to native tasks
Replace `aide task` CLI with Claude Code native tools.

**File**: `skills/swarm/SKILL.md`

**Changes**:
- Use `TaskCreate` instead of `aide task create`
- Use `TaskUpdate` for claiming (owner + status)
- Use `TaskList` for visibility
- Use `blockedBy` for SDLC stage dependencies
- Document task naming: `[Story-N][STAGE] Description`

#### 2.2 Add SDLC mode
Default mode for code changes.

**Changes**:
- Decompose work into stories (not flat tasks)
- Each story agent creates stage tasks with dependencies:
  - `[Story][DESIGN]` → `[Story][TEST]` → `[Story][DEV]` → `[Story][VERIFY]` → `[Story][DOCS]`
- Story agents can spawn subagents for specific stages
- Keep worktree isolation per story

#### 2.3 Update agent instructions
Teach story agents to manage SDLC pipeline.

**Changes**:
- Create all stage tasks upfront with `blockedBy`
- Execute stages sequentially
- Use appropriate skill for each stage
- Mark tasks complete as stages finish

---

### 3. Quality Gate Hooks (Future)

#### 3.1 TeammateIdle hook
Fires when agent goes idle - validate work complete.

**File**: `src/hooks/teammate-idle.ts` (new)

**Checks**:
- All claimed tasks complete
- Tests pass in worktree
- No uncommitted changes
- No lint/type errors

#### 3.2 TaskCompleted hook
Fires when task marked complete - stage-specific validation.

**File**: `src/hooks/task-completed.ts` (new)

**Stage-specific checks**:
- DESIGN: Has design output
- TEST: Test files exist
- DEV: Tests pass
- VERIFY: Full suite green, lint clean
- DOCS: Docs updated

**Note**: May require `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled.

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

#### 4.4 Enhanced Subagent Context Injection
Give subagents more relevant context.

**File**: `src/hooks/subagent-tracker.ts` (enhance fetchSubagentMemories)

**Current**:
- Global memories (`scope:global`)
- Project decisions

**Add**:
- Current session memories (`session:<current-id>`)
- Task/story-specific memories (`task:<id>` or `story:<id>` tags)
- Decision ordering: latest per topic wins (ensure superseding works)

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
