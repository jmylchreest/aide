---
name: build-fix
description: Fix build, lint, and type errors
triggers:
  - fix build
  - fix the build
  - build is broken
  - lint errors
  - type errors
  - fix errors
---

# Build Fix Mode

Rapidly fix all build, type, and lint errors with minimal changes.

## Prerequisites

Before starting, ensure you have:
- Access to the project root directory
- Build tools installed (npm, tsc, etc.)

## Workflow

### Step 1: Capture All Errors

Run all checks and capture output:

```bash
# Run in sequence, capture all output
npm run build 2>&1 | head -100
npm run lint 2>&1 | head -100
npx tsc --noEmit 2>&1 | head -100
```

**If commands fail to run:**
- Check package.json for available scripts
- Try `npm install` if dependencies are missing
- Use `npm ci` for clean install if lock file exists

### Step 2: Categorize Errors by Priority

Process errors in this order:

1. **Type errors** (highest priority) - Fix TypeScript/type issues first
2. **Build errors** - Compilation failures
3. **Lint errors** - ESLint, Prettier, etc.
4. **Warnings** - Address only if trivial

Group similar errors:
- Missing imports
- Type mismatches
- Unused variables
- Formatting issues

### Step 3: Search for Context

Use MCP tools to find definitions and patterns:

```
# Find type definitions
mcp__plugin_aide_aide__code_search query="InterfaceName" kind="interface"

# Find function signatures
mcp__plugin_aide_aide__code_search query="functionName" kind="function"

# Get symbols in a file
mcp__plugin_aide_aide__code_symbols file="path/to/file.ts"

# Check project conventions
mcp__plugin_aide_aide__decision_get topic="coding-style"
```

### Step 4: Apply Fixes Systematically

Fix in batches by category:

1. **All missing imports together** - Add import statements
2. **All type annotations together** - Add/fix type declarations
3. **All unused variables together** - Remove or prefix with `_`

### Step 5: Verify After Each Batch

```bash
# Run full verification
npm run build && npm run lint && npx tsc --noEmit
```

**Verification criteria:**
- Exit code 0 for all commands
- No error output
- Warnings are acceptable if not introduced by changes

### Step 6: Final Verification

```bash
# Run tests to ensure fixes didn't break functionality
npm test
```

## Failure Handling

| Failure | Action |
|---------|--------|
| `npm run build` fails to start | Run `npm install` first |
| Circular dependency error | Check import structure, may need refactoring |
| Type error in third-party lib | Check @types package version, update if needed |
| Cannot resolve module | Check tsconfig.json paths, baseUrl settings |
| ESLint config error | Check .eslintrc, ensure plugins installed |
| Fix introduces new errors | Revert and try alternative approach |

## Rules

- **Minimal changes** - Fix only what's broken
- **No refactoring** - Don't improve "while you're there"
- **No new features** - Just fix errors
- **Match style** - Follow existing patterns exactly
- **One batch at a time** - Verify between batches

## Common Fixes Reference

| Error | Fix |
|-------|-----|
| Cannot find module | Add import statement |
| Type 'X' not assignable | Add type annotation or use type assertion |
| 'X' is declared but never used | Remove or prefix with `_` |
| Missing return type | Add explicit return type |
| Unexpected any | Add proper type annotation |
| Property does not exist | Check interface, add property or fix typo |
| Argument of type X not assignable | Check function signature, cast if needed |

## MCP Tools

- `mcp__plugin_aide_aide__code_search` - Find type definitions, function signatures
- `mcp__plugin_aide_aide__code_symbols` - List all symbols in a file
- `mcp__plugin_aide_aide__decision_get` - Check project coding decisions

## Output Format

Report all fixes made:

```markdown
## Build Fix Report

### Errors Fixed

- `src/foo.ts:10` - Added missing import for `Bar`
- `src/foo.ts:25` - Fixed type: `string` -> `string | null`
- `src/bar.ts:5` - Removed unused variable `temp`

### Verification

- Build: PASS
- Lint: PASS
- Types: PASS
- Tests: PASS

### Notes
[Any observations or remaining warnings]
```
