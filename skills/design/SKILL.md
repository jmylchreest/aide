---
name: design
description: Technical design and architecture for implementation
triggers:
  - design this
  - design the
  - architect this
  - architect the
  - spec this
  - spec the
  - plan this
  - plan the
---

# Design Mode

Output a technical design specification that downstream SDLC stages can consume.

## Purpose

Create a structured design document that defines:
1. **What** to build (interfaces, types, components)
2. **How** it fits together (data flow, interactions)
3. **Why** key decisions were made (rationale)
4. **Success criteria** (acceptance criteria for TEST stage)

## Workflow

### Step 1: Understand the Request

Read the request and identify:
- Core functionality required
- Constraints (tech stack, patterns, performance)
- Integration points with existing code

### Step 2: Explore the Codebase

Use search tools to understand existing patterns:

- Use `Grep` for similar patterns, interfaces, types
- Use `Glob` for relevant files
- Use `mcp__plugin_aide_aide__decision_list` to review all decisions
- Use `mcp__plugin_aide_aide__decision_get` with the relevant topic to check specific decisions

### Step 3: Define Interfaces

Define the public API/interfaces first:

```typescript
// Example TypeScript interface
interface UserService {
  createUser(data: CreateUserInput): Promise<User>;
  getUser(id: string): Promise<User | null>;
  updateUser(id: string, data: UpdateUserInput): Promise<User>;
}

interface CreateUserInput {
  email: string;
  name: string;
}
```

```go
// Example Go interface
type UserService interface {
    CreateUser(ctx context.Context, input CreateUserInput) (*User, error)
    GetUser(ctx context.Context, id string) (*User, error)
    UpdateUser(ctx context.Context, id string, input UpdateUserInput) (*User, error)
}
```

### Step 4: Document Data Flow

Describe how components interact:

```
Request → Controller → Service → Repository → Database
                ↓
           Validator
                ↓
          Error Handler → Response
```

### Step 5: Record Key Decisions

Store architectural decisions for future reference:

```bash
aide decision set "<feature>-storage" "PostgreSQL with JSONB for metadata" \
  --rationale="Need flexible schema for user preferences"

aide decision set "<feature>-auth" "JWT with refresh tokens" \
  --rationale="Stateless auth, mobile client support"
```

### Step 6: Define Acceptance Criteria

List specific, testable criteria for the TEST stage:

```markdown
## Acceptance Criteria

- [ ] User can be created with email and name
- [ ] Duplicate emails are rejected with 409 status
- [ ] Created user has UUID identifier
- [ ] User can be retrieved by ID
- [ ] Non-existent user returns null (not error)
- [ ] User email can be updated
- [ ] User name can be updated
```

## Required Output Format

```markdown
# Design: [Feature Name]

## Overview
[1-2 sentence summary of what this feature does]

## Interfaces

### [Interface/Type Name]
\`\`\`typescript
// Interface definition with comments
\`\`\`

## Data Flow
[Diagram or description of component interactions]

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Storage | PostgreSQL | [why] |
| Auth | JWT | [why] |

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Files to Create/Modify
- `path/to/file.ts` - [purpose]
- `path/to/test.ts` - [test scope]

## Dependencies
- [External packages needed]
- [Internal modules to import]

## Out of Scope
- [Explicitly excluded items]
```

## Failure Handling

### Unclear Requirements
1. List specific ambiguities
2. State assumptions you're making
3. Record assumptions: `aide memory add --category=decision "Assumed X because Y"`
4. Proceed with reasonable defaults

### Conflicting Patterns Found
1. Document the conflicting patterns
2. Choose one and record the decision:
   ```bash
   aide decision set "<topic>" "<choice>" --rationale="<why this over alternatives>"
   ```

### Too Large / Too Vague
1. Break into smaller, focused designs
2. Design the core/MVP first
3. Note future phases in "Out of Scope"

## Verification Checklist

Before completing design:
- [ ] Interfaces are defined with types
- [ ] Data flow is documented
- [ ] Key decisions are recorded in aide
- [ ] Acceptance criteria are testable
- [ ] Files to modify are listed
- [ ] Dependencies are identified

## Completion

When design is complete:
1. Output the full design document
2. Confirm: "Design complete. Ready for TEST stage."

The design document feeds directly into the TEST stage, where acceptance criteria become test cases.
