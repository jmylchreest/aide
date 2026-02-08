---
name: verify
description: Active QA verification - full validation before completion
triggers:
  - verify this
  - verify the
  - qa check
  - validate
  - verification
  - verify stage
---

# Verify Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Active QA verification: run all quality checks and report pass/fail status.

## Purpose

This is the VERIFY stage of the SDLC pipeline. Implementation is complete. Your job is to run comprehensive validation and report results.

**This is different from `review`**: Review is read-only analysis. Verify is active execution of quality checks.

## Verification Checklist

Run ALL of the following. Each must pass.

### 1. Full Test Suite

```bash
# TypeScript/JavaScript
npm test

# Go
go test ./...

# Python
pytest
```

**Expected**: All tests pass, no failures, no skipped tests.

### 2. Type Checking

```bash
# TypeScript
npx tsc --noEmit

# Go (implicit in build)
go build ./...

# Python (if using mypy)
mypy .
```

**Expected**: No type errors.

### 3. Linting

```bash
# TypeScript/JavaScript
npm run lint

# Go
go vet ./...
golangci-lint run

# Python
ruff check .
```

**Expected**: No lint errors. Warnings acceptable if pre-existing.

### 4. Build

```bash
# TypeScript/JavaScript
npm run build

# Go
go build ./...

# Python
python -m py_compile *.py
```

**Expected**: Build succeeds without errors.

### 5. Debug Artifact Check

Search for common debug artifacts that shouldn't be committed:

```bash
# Console logs (JS/TS)
Grep for "console.log" in changed files

# Debug prints (Go)
Grep for "fmt.Println" or "log.Print" that look like debug

# Python debug
Grep for "print(" or "pdb" or "breakpoint()"

# TODO/FIXME comments
Grep for "TODO" or "FIXME" in changed files

# Debugger statements
Grep for "debugger" in JS/TS files
```

**Expected**: No debug artifacts in new code. Pre-existing ones noted but not blocking.

### 6. Uncommitted Changes Check

```bash
git status
```

**Expected**: Working directory clean, or only expected files modified.

## Workflow

### Step 1: Run All Checks

Execute each check in sequence, capturing output:

```bash
# Run all checks, capture results
npm test 2>&1 | head -50
npm run lint 2>&1 | head -50
npx tsc --noEmit 2>&1 | head -50
npm run build 2>&1 | head -50
```

### Step 2: Check for Debug Artifacts

```bash
# Search for debug code in recently changed files
git diff --name-only HEAD~1 | xargs grep -l "console.log\|debugger" 2>/dev/null || true
```

### Step 3: Compile Results

Create a verification report.

## Output Format

### All Checks Pass

```markdown
## Verification Report: PASS

### Tests
- Status: PASS
- Total: 42
- Passed: 42
- Failed: 0

### Type Check
- Status: PASS
- Errors: 0

### Lint
- Status: PASS
- Errors: 0
- Warnings: 3 (pre-existing)

### Build
- Status: PASS

### Debug Artifacts
- Status: CLEAN
- No debug code found in new files

### Verdict: READY FOR DOCS STAGE
```

### Some Checks Fail

```markdown
## Verification Report: FAIL

### Tests
- Status: FAIL
- Total: 42
- Passed: 40
- Failed: 2
- Failures:
  - `UserService.test.ts:45` - expected 200, got 401
  - `AuthMiddleware.test.ts:23` - timeout after 5000ms

### Type Check
- Status: PASS

### Lint
- Status: FAIL
- Errors:
  - `src/user.ts:12` - 'unused' is defined but never used

### Build
- Status: PASS

### Debug Artifacts
- Status: WARNING
- Found:
  - `src/user.ts:34` - console.log("debug user:", user)

### Verdict: NEEDS BUILD-FIX

Issues to resolve:
1. Fix 2 failing tests
2. Remove console.log on line 34
3. Fix lint error (remove unused variable)
```

## Failure Handling

### Tests Fail

1. Document which tests fail and why
2. Do NOT fix them yourself (that's BUILD-FIX stage (via `/aide:build-fix`))
3. Report failures clearly
4. Verdict: FAIL, needs BUILD-FIX stage (via `/aide:build-fix`)

### Lint Errors

1. Document all lint errors
2. Distinguish new errors from pre-existing
3. New errors = FAIL
4. Pre-existing warnings = PASS with notes

### Type Errors

1. Document all type errors with file:line
2. Always FAIL on type errors
3. Type errors must be fixed before proceeding

### Build Fails

1. Document build error
2. Always FAIL on build errors
3. Critical blocker for release

## Decision Tree

```
All tests pass?
├── No → FAIL (needs BUILD-FIX)
└── Yes → Type check pass?
          ├── No → FAIL (needs BUILD-FIX)
          └── Yes → Lint pass?
                    ├── No (new errors) → FAIL (needs BUILD-FIX)
                    └── Yes/Warnings only → Build pass?
                                           ├── No → FAIL (needs BUILD-FIX)
                                           └── Yes → Debug artifacts?
                                                     ├── Found → FAIL (needs cleanup)
                                                     └── Clean → PASS
```

## Completion

### On PASS

```
Verification complete: ALL CHECKS PASS
Ready for DOCS stage.
```

### On FAIL

```
Verification complete: CHECKS FAILED
Returning to BUILD-FIX stage (via `/aide:build-fix`).

Issues:
1. [list of issues]
```

When FAIL, the BUILD-FIX stage (via `/aide:build-fix`) addresses issues, then VERIFY runs again.

## Integration with SDLC Pipeline

```
[DESIGN] → [TEST] → [DEV] → [VERIFY] → [DOCS]
                               ↑
                          YOU ARE HERE
                               │
                    ┌──────────┴──────────┐
                    │                     │
                  PASS                  FAIL
                    │                     │
                    ▼                     ▼
                 [DOCS]               [BUILD-FIX] → [VERIFY]
```

- **Input**: Completed implementation from DEV stage
- **Output**: Pass/Fail report with specifics
- **On Pass**: Proceed to DOCS stage
- **On Fail**: Return to BUILD-FIX stage (via `/aide:build-fix`), then re-verify
