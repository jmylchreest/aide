---
name: implement
description: TDD implementation - make failing tests pass
triggers:
  - implement this
  - implement the
  - make tests pass
  - dev stage
  - development stage
---

# Implement Mode

Focused TDD implementation: make failing tests pass with minimal code.

## Purpose

This is the DEV stage of the SDLC pipeline. Tests already exist from the TEST stage. Your job is to write the minimal implementation that makes them pass.

## Prerequisites

Before starting:
- Tests exist and are failing (from TEST stage)
- Design/spec is available (from DESIGN stage)
- You know which files to create/modify

## Workflow

### Step 1: Verify Tests Exist and Fail

```bash
# Run the tests - they MUST fail initially
npm test -- path/to/feature.test.ts
# or
go test ./pkg/feature/...
```

**If tests pass**: The work is already done. Mark complete and move on.

**If tests don't exist**: This is wrong - go back to TEST stage.

### Step 2: Read the Tests

Understand what the tests expect:
- What functions/methods need to exist?
- What are the expected inputs and outputs?
- What edge cases are covered?

```bash
# Read the test file
Read path/to/feature.test.ts
```

### Step 3: Check Design Decisions

Use `mcp__plugin_aide_aide__decision_get` with the feature topic to review decisions from DESIGN stage.
Use `mcp__plugin_aide_aide__decision_list` to see all project decisions.

### Step 4: Implement Incrementally

Write code to make tests pass **one at a time**:

1. Pick the simplest failing test
2. Write minimal code to pass it
3. Run tests
4. If passing, move to next test
5. If failing, fix before proceeding

```bash
# Run specific test
npm test -- --grep "should create user"
# or
go test -run TestCreateUser ./pkg/...
```

### Step 5: Backpressure Checkpoint (REQUIRED)

**You CANNOT proceed until ALL tests pass.**

```bash
# Full test run
npm test -- path/to/feature.test.ts
# or
go test -v ./pkg/feature/...
```

**BLOCKING RULE**: If any test fails:
1. Analyze the failure
2. Fix the issue
3. Re-run tests
4. Repeat until ALL pass

**DO NOT skip failing tests. DO NOT proceed with red tests.**

### Step 6: Verify Build

```bash
# Ensure it compiles
npm run build
# or
go build ./...
```

### Step 7: Commit

```bash
git add -A
git commit -m "feat: implement <feature> - tests passing"
```

## Rules

1. **Tests First**: Never write code without a failing test
2. **Minimal Code**: Only write what's needed to pass tests
3. **No Gold Plating**: Don't add features not covered by tests
4. **Red → Green**: Tests must fail before they pass
5. **Atomic Commits**: One logical change per commit
6. **No Skipping**: Every test must pass

## Common Patterns

### TypeScript/JavaScript

```typescript
// Read test expectations
describe('UserService', () => {
  it('should create user with email and name', async () => {
    const user = await service.createUser({ email: 'test@example.com', name: 'Test' });
    expect(user.id).toBeDefined();
    expect(user.email).toBe('test@example.com');
  });
});

// Implement to match
export class UserService {
  async createUser(input: CreateUserInput): Promise<User> {
    return {
      id: crypto.randomUUID(),
      email: input.email,
      name: input.name,
      createdAt: new Date(),
    };
  }
}
```

### Go

```go
// Read test expectations
func TestCreateUser(t *testing.T) {
    svc := NewUserService()
    user, err := svc.CreateUser(context.Background(), CreateUserInput{
        Email: "test@example.com",
        Name:  "Test",
    })
    require.NoError(t, err)
    assert.NotEmpty(t, user.ID)
    assert.Equal(t, "test@example.com", user.Email)
}

// Implement to match
func (s *UserService) CreateUser(ctx context.Context, input CreateUserInput) (*User, error) {
    return &User{
        ID:        uuid.New().String(),
        Email:     input.Email,
        Name:      input.Name,
        CreatedAt: time.Now(),
    }, nil
}
```

## Failure Handling

### Test Won't Pass After Multiple Attempts

1. Re-read the test - is the expectation correct?
2. Check if test has a bug (it happens)
3. Check design decisions - is implementation matching spec?
4. Record blocker:
   ```bash
   aide memory add --category=blocker "Cannot pass test X: <reason>"
   ```
5. If stuck after 3 attempts, ask for help

### Build Fails

1. Read error message carefully
2. Check for missing imports
3. Check for type mismatches
4. Fix and re-run build
5. Then re-run tests

### Test Passes But Implementation Feels Wrong

1. If tests pass, the contract is met
2. Don't refactor during implement stage
3. Note concerns for future:
   ```bash
   aide memory add --category=issue "Implementation of X could be improved: <how>"
   ```
4. Proceed - refactoring is a separate concern

## Verification Checklist

Before completing:
- [ ] All tests pass (not just some)
- [ ] Build succeeds
- [ ] Changes are committed
- [ ] No debug code left (console.log, fmt.Println for debugging)

## Completion

When all tests pass:

```bash
# Final verification
npm test -- path/to/feature.test.ts && npm run build

# Or Go
go test -v ./pkg/feature/... && go build ./...
```

Output: "Implementation complete. All tests passing. Ready for VERIFY stage."

## Integration with SDLC Pipeline

This skill is designed for the DEV stage:

```
[DESIGN] → [TEST] → [DEV/IMPLEMENT] → [VERIFY] → [DOCS]
                         ↑
                    YOU ARE HERE
```

- **Input**: Failing tests from TEST stage
- **Output**: Passing tests, working implementation
- **Next**: VERIFY stage runs full validation
