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

### 3.2 TaskCompleted Hook ✅

- [x] SDLC stage validation on task completion
- [x] Parses `[story-id][STAGE]` pattern from task subject
- [x] Stage-specific validation (DEV: tests pass, VERIFY: full QA)
- [x] Works WITHOUT `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`

### 4.1 Auto Session Summary ✅

- [x] Captures session summary on Stop hook
- [x] Extracts: files modified, tools used, user tasks
- [x] Stores with `category=session`, `tags=session-summary,session:<id>`

### 4.2 Decision Capture ✅

- [x] `<aide-decision topic="...">` tag parsing in PostToolUse
- [x] Extracts topic, decision, rationale
- [x] Stores via `aide decision set`

### 4.3 `/aide:decide` Skill ✅

- [x] Formal decision-making interview workflow
- [x] Triggers: "decide", "help me decide", "help me choose", "how should we", etc.
- [x] 6-phase flow: IDENTIFY → EXPLORE → ANALYZE → RECOMMEND → CONFIRM → RECORD

---

## Pending Implementation

### 3.1 TeammateIdle Hook (Future)

Fires when agent goes idle - validate work complete.

**File**: `src/hooks/teammate-idle.ts` (new)

**Checks**:

- All claimed tasks complete
- Tests pass in worktree
- No uncommitted changes
- No lint/type errors

**Note**: Requires `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled.

### 4.5 Session Memory Injection (Future)

Add current session memories to subagent context.

**File**: `src/hooks/subagent-tracker.ts`

**Add**:

- Current session memories (`session:<current-id>`)
- Task/story-specific memories (`task:<id>` or `story:<id>` tags)

### 5. LLM-Based Classification of Unknown Files (Future)

Surface unclassified files (not matching any grammar pack) via MCP tools so the calling
LLM can classify them. Since aide is an MCP server (not an LLM client), classification
must be initiated by the LLM via survey skill tools.

**Approach**:

- The topology analyzer already groups files by extension during scanning
- `KindUnclassified` constant added to `pkg/survey/types.go`
- Add a `processUnclassified()` pass to topology that emits `KindUnclassified` entries
  grouped by extension (e.g., ".tf" -> 12 files, ".proto" -> 3 files)
- Expose via `survey_list` / `survey_search` MCP tools (already works by kind filter)
- Survey skill can prompt the LLM: "These file extensions are unclassified: .tf, .proto.
  What languages/technologies do they represent?"
- LLM responses can be stored as decisions or memories for future sessions

---

## SDLC Stage → Skill Mapping

| Stage  | Skill                        | Purpose               |
| ------ | ---------------------------- | --------------------- |
| DESIGN | `design` (updated from plan) | Output technical spec |
| TEST   | `test` (existing)            | Write failing tests   |
| DEV    | `implement` (new)            | Make tests pass       |
| VERIFY | `verify` (new)               | Full QA validation    |
| FIX    | `build-fix` (existing)       | Fix failures          |
| DOCS   | `docs` (new)                 | Update documentation  |

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
