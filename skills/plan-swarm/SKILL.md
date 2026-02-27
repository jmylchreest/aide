---
name: plan-swarm
description: Socratic planning interview for swarm decomposition
triggers:
  - plan swarm
  - plan stories
  - decompose stories
  - scope swarm
  - socratic plan
---

# Plan Swarm

**Recommended model tier:** smart (opus) - this skill requires careful reasoning

Structured planning interview to decompose work into independent stories before running a swarm.

## Quick Reference

```
plan swarm                → Full interview workflow (recommended)
plan swarm --fast         → Skip interview, state assumptions, decompose directly
```

## Workflow

### Phase 1: Understand

Explore the codebase and existing context before asking questions.

1. **Read existing decisions** via `mcp__plugin_aide_aide__decision_list` and `mcp__plugin_aide_aide__decision_get`
2. **Search memories** via `mcp__plugin_aide_aide__memory_search` for relevant past context
3. **Explore the codebase** — read key files, understand architecture, identify boundaries
4. **Identify the scope** — what is the user asking for? What are the natural boundaries?

Do NOT ask questions yet. Build understanding first.

### Phase 2: Interview

Conduct 2-3 rounds of focused questions. Each round has 2-4 questions. Max 3 rounds total.

**Round 1: Scope & Boundaries**

- What is in scope vs out of scope?
- What are the success criteria?
- Are there any constraints (time, tech, compatibility)?

**Round 2: Dependencies & Risks**

- What shared state or files will multiple stories touch?
- What could go wrong? What are the risky parts?
- Are there external dependencies (APIs, services, data)?

**Round 3: Acceptance Criteria** (if needed)

- How will we know each story is done?
- What tests should exist?
- What does "good enough" look like?

Use the AskUserQuestion tool for each round. Summarize what you've learned before asking the next round.

**Fast mode** (`plan swarm --fast`): Skip this phase. State your assumptions explicitly, then proceed directly to Phase 3.

### Phase 3: Decompose

Output a structured story list. Each story must be:

- **Independent** — can be developed in parallel without conflicting file edits
- **Complete** — has clear boundaries and acceptance criteria
- **Testable** — has concrete verification steps

**Story format:**

```markdown
## Stories

### 1. [Story Name]

- **Description**: What this story implements
- **Files**: List of files this story will create or modify
- **Acceptance Criteria**:
  - [ ] Criterion 1
  - [ ] Criterion 2
- **Risk**: low | medium | high
- **Notes**: Any special considerations

### 2. [Story Name]

...
```

**Independence check**: Verify that no two stories modify the same file. If they do, either:

1. Merge the stories
2. Restructure so each story owns its files
3. Sequence them (one blocks the other)

### Phase 4: Validate & Store

1. **Present the plan** to the user for review
2. **Store as decision** once approved:
   ```bash
   ./.aide/bin/aide decision set "swarm-plan" "<N> stories: <story-1>, <story-2>, ..." \
     --details='<JSON object>' \
     --rationale="<brief description of scope and approach>"
   ```
3. **Record shared decisions** discovered during planning:
   ```bash
   ./.aide/bin/aide decision set "<topic>" "<decision>"
   ```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

4. **Instruct the user**: Run `/aide:swarm` to execute the plan

**Note on task materialization:** The plan is stored as a decision, not as tasks. The `/aide:swarm` skill reads the plan and materializes tasks at execution time:

- **Claude Code**: Each story agent creates native tasks (`TaskCreate`) with `blockedBy` dependency chaining for SDLC stages.
- **OpenCode**: The orchestrator creates aide tasks (`task_create` MCP tool) for all SDLC stages upfront, and story agents claim them.

## Output Format

The stored `swarm-plan` decision should be a JSON object:

```json
{
  "stories": [
    {
      "name": "story-name",
      "description": "What it does",
      "files": ["src/foo.ts", "src/bar.ts"],
      "acceptance": ["criterion 1", "criterion 2"],
      "risk": "low",
      "notes": ""
    }
  ],
  "shared_decisions": ["auth-strategy", "db-schema"],
  "assumptions": ["Node 20+", "existing test framework"],
  "created": "ISO timestamp"
}
```

## Tips

- Fewer stories is usually better. 2-4 stories is ideal for most tasks.
- Each story should take roughly similar effort.
- If a story is too large, split it. If it's too small, merge it.
- Stories that share database migrations or config files should be sequenced, not parallelized.
- The plan is a starting point. The swarm skill will adapt if needed.
