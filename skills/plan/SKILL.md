---
name: plan
description: Planning interview workflow
triggers:
  - plan this
  - plan the
  - planning mode
  - interview me
  - let's plan
---

# Planning Mode

You are now in **planning mode**. Interview the user to understand requirements before implementation.

## Interview Process

### Phase 1: Understanding
Ask clarifying questions directly to the user (they will respond in the conversation):
- What is the core goal?
- Who are the users/consumers?
- What are the constraints (time, tech stack, etc.)?

**Ask one focused question at a time. Wait for the user's response before proceeding.**

### Phase 2: Scope Definition
- What's in scope vs out of scope?
- What's the MVP vs nice-to-have?
- Are there existing patterns to follow?

### Phase 3: Technical Discovery
Use the Task tool to spawn an explorer agent:
```
Task: "Explore the codebase to understand: [specific question]"
Agent type: explore
Model: haiku (fast, cost-effective for exploration)
```

Discover:
- Current codebase structure
- Existing patterns and conventions
- Related implementations

If exploration fails or returns empty results:
1. Try alternative search terms
2. Check if the feature area exists at all
3. Note "greenfield" if no existing patterns found

### Phase 4: Plan Creation

Create a structured plan:

```markdown
## Goal
[One sentence summary]

## Requirements
- [ ] Requirement 1
- [ ] Requirement 2

## Technical Approach
[High-level architecture]

## Tasks
1. [ ] Task 1 (estimated complexity: low/medium/high)
2. [ ] Task 2
3. [ ] Task 3

## Risks & Mitigations
- Risk: [description] → Mitigation: [approach]

## Out of Scope
- [Items explicitly excluded]
```

### Phase 5: Approval

Present plan to user and ask:
- "Does this plan capture your requirements?"
- "Should I proceed with implementation?"

**Wait for explicit user approval before proceeding.**

## Guidelines

- **Don't assume** - ask when unclear
- **Be specific** - vague plans lead to vague results
- **Consider edge cases** - what could go wrong?
- **Size appropriately** - break large tasks into smaller ones

## Failure Handling

### User Provides Unclear Requirements
1. Summarize your understanding
2. List specific ambiguities
3. Offer options: "Did you mean A or B?"
4. Wait for clarification

### Technical Discovery Finds Conflicts
1. Document the conflicting patterns found
2. Present options to user with trade-offs
3. Record decision: `aide decision set "<topic>" "<choice> because <reason>"`

### Plan Is Too Large
1. If more than 10 tasks, suggest breaking into phases
2. Identify a meaningful MVP subset
3. Propose: "Phase 1: [MVP], Phase 2: [Enhancements]"

## Exiting Plan Mode

After user approves:
1. Create tasks using aide CLI:
   ```bash
   aide task create "Task 1" --description="Details from plan"
   aide task create "Task 2" --description="Details from plan"
   ```
2. Store the plan as a decision for reference:
   ```bash
   aide decision set "<feature>-plan" "Approved plan with N tasks"
   ```
3. Switch to autopilot or normal execution
4. Reference plan throughout implementation

## Completion Criteria

Plan mode is complete when:
- User has explicitly approved the plan
- All tasks are created in aide
- Ready statement given: "Plan approved. Ready to proceed with implementation."
