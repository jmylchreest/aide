---
name: test
description: Write and run tests for code
triggers:
  - "write tests"
  - "add tests"
  - "test this"
  - "run tests"
---

# Test Mode

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Write comprehensive tests and run test suites.

## Prerequisites

Before starting:
- Identify the code to be tested (function, module, feature)
- Understand the testing framework used in the project

## Workflow

### Step 1: Check Project Testing Decisions

Use the `mcp__plugin_aide_aide__decision_get` tool with topic `testing` to check for testing framework decisions.

Common frameworks by language:
- **TypeScript/JavaScript:** Vitest, Jest, Mocha
- **Go:** built-in `go test`
- **Python:** pytest, unittest

### Step 2: Discover Existing Test Patterns

Use `Glob` to find test files:
- Pattern: `**/*.test.ts`, `**/*.spec.ts` (TypeScript)
- Pattern: `**/*_test.go` (Go)
- Pattern: `**/test_*.py`, `**/*_test.py` (Python)

Use `mcp__plugin_aide_aide__code_search` with query `describe` and `it` to find test patterns.

Read an existing test file to understand:
- Import patterns
- Setup/teardown patterns
- Mocking approach
- Assertion style

### Step 3: Analyze Target Code

Use `mcp__plugin_aide_aide__code_symbols` with the target file path to get function signatures.
Use `mcp__plugin_aide_aide__code_search` to find related types.

Identify:
- Input parameters and types
- Return type
- Side effects
- Dependencies to mock
- Edge cases

### Step 4: Write Tests

Follow the project's testing conventions. Cover these scenarios:

**Test Categories:**
1. **Happy path** - Normal, expected inputs
2. **Edge cases** - Empty, null, boundary values
3. **Error cases** - Invalid inputs, expected failures
4. **Async behavior** - If applicable

**Naming convention:**
- Descriptive names that explain what is being tested
- Format: "should [expected behavior] when [condition]"

### Step 5: Run Tests

```bash
# TypeScript/JavaScript
npm test                    # Run all tests
npm test -- --grep "name"   # Run specific test
npm run test:coverage       # Run with coverage

# Go
go test ./...               # Run all tests
go test -v ./pkg/...        # Verbose output
go test -cover ./...        # With coverage

# Python
pytest                      # Run all tests
pytest -v                   # Verbose
pytest --cov                # With coverage
```

### Step 6: Verify Coverage

```bash
# Check coverage report
npm run test:coverage

# Go
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Coverage targets:**
- New code: aim for >80%
- Critical paths: aim for >90%
- Focus on meaningful tests, not just coverage numbers

## Failure Handling

| Failure | Action |
|---------|--------|
| Test imports fail | Check path aliases, ensure test config matches main |
| Mock not working | Verify mock setup, check dependency injection |
| Async test timeout | Add proper await, increase timeout if needed |
| Flaky test | Check for shared state, timing issues, or external deps |
| Coverage too low | Add edge case tests, error path tests |

## Test Structure Templates

### TypeScript/JavaScript (Vitest/Jest)

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { functionToTest } from './module';

describe('functionToTest', () => {
  beforeEach(() => {
    // Reset state before each test
    vi.clearAllMocks();
  });

  it('should return expected result for valid input', () => {
    const result = functionToTest('valid input');
    expect(result).toBe('expected output');
  });

  it('should handle empty input', () => {
    const result = functionToTest('');
    expect(result).toBe('');
  });

  it('should throw error for null input', () => {
    expect(() => functionToTest(null)).toThrow('Input required');
  });

  it('should handle async operation', async () => {
    const result = await functionToTest('async input');
    expect(result).resolves.toBe('async output');
  });
});
```

### Go

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "input", "expected", false},
        {"empty input", "", "", false},
        {"invalid input", "bad", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Python

```python
import pytest
from module import function_to_test

class TestFunctionToTest:
    def test_valid_input(self):
        assert function_to_test("input") == "expected"

    def test_empty_input(self):
        assert function_to_test("") == ""

    def test_invalid_input_raises(self):
        with pytest.raises(ValueError):
            function_to_test(None)

    @pytest.fixture
    def mock_dependency(self, mocker):
        return mocker.patch("module.dependency")
```

## MCP Tools

- `mcp__plugin_aide_aide__code_search` - Find existing tests, patterns, types
- `mcp__plugin_aide_aide__code_symbols` - Understand function signatures to test
- `mcp__plugin_aide_aide__decision_get` - Check testing framework decisions

## Verification Criteria

Before completing:
- [ ] All new tests pass
- [ ] Existing tests still pass
- [ ] Coverage meets project standards
- [ ] Tests are deterministic (not flaky)
- [ ] Tests follow project conventions

## Output Format

```markdown
## Tests Added

### Files
- `path/to/file.test.ts` - 5 tests for UserService

### Test Cases
1. should create user with valid data
2. should reject duplicate email
3. should hash password before saving
4. should handle empty name
5. should validate email format

### Coverage
- New code: 92%
- Total project: 84%

### Verification
- All tests: PASS
- No flaky tests observed
```
