---
name: docs
description: Update documentation to match implementation
triggers:
  - update docs
  - update documentation
  - document this
  - docs stage
  - documentation
---

# Documentation Mode

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Update documentation to accurately reflect the implemented code.

## Purpose

This is the DOCS stage of the SDLC pipeline. Implementation is complete and verified. Your job is to ensure documentation matches reality.

## What to Document

### 1. README Updates

If the feature affects how users interact with the project:
- Installation steps changed?
- New commands or options?
- New configuration?
- New dependencies?

### 2. API Documentation

If public APIs were added or changed:
- Function/method signatures
- Parameters and return types
- Usage examples
- Error conditions

### 3. Inline Documentation

If code needs explanation:
- Complex algorithms
- Non-obvious design decisions
- Integration points
- Edge cases

### 4. Architecture Docs

If structure changed:
- New components
- Changed data flow
- New dependencies between modules

## Workflow

### Step 1: Identify Changed Files

```bash
# See what was changed
git diff --name-only HEAD~5  # or appropriate range

# Find related docs
Glob for **/*.md
Glob for **/docs/**/*
```

### Step 2: Check Existing Documentation

Read current docs that might be affected:

```bash
Read README.md
Read docs/api.md  # if exists
Read CHANGELOG.md  # if exists
```

### Step 3: Read the Implementation

Understand what was built:

- Read the new code files
- Use `mcp__plugin_aide_aide__decision_get` with the feature topic to check design decisions
- Use `mcp__plugin_aide_aide__decision_list` to see all decisions

### Step 4: Update Documentation

#### README.md Updates

Add/update sections as needed:

```markdown
## New Feature

Description of what it does.

### Usage

\`\`\`bash
command --new-flag
\`\`\`

### Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| newOption | string | "default" | What it does |
```

#### API Documentation

Document public interfaces:

```markdown
### functionName(param1, param2)

Brief description.

**Parameters:**
- `param1` (string): What it is
- `param2` (number, optional): What it does. Default: 10

**Returns:** `Promise<Result>`

**Example:**
\`\`\`typescript
const result = await functionName("input", 20);
\`\`\`

**Throws:**
- `ValidationError`: When param1 is empty
```

#### Inline Comments

Add comments for non-obvious code:

```typescript
// Use exponential backoff to handle rate limiting
// Starts at 100ms, doubles each retry, max 5 retries
async function fetchWithRetry(url: string): Promise<Response> {
  // ...
}
```

### Step 5: Verify Documentation

```bash
# If docs have a build step
npm run docs:build

# Check for broken links (if tool available)
npm run docs:check

# Manual review - read what you wrote
```

### Step 6: Commit Documentation

```bash
git add -A
git commit -m "docs: document <feature>"
```

## Documentation Standards

### Be Accurate
- Documentation must match code exactly
- If code changes, docs must change
- Never document aspirational features

### Be Concise
- One sentence per concept
- Use examples over explanations
- Bullet points over paragraphs

### Be Complete
- Cover happy path AND error cases
- Include all public API
- Document breaking changes

### Be Findable
- Use clear headings
- Add to table of contents
- Cross-reference related sections

## What NOT to Document

- Internal implementation details (unless complex)
- Obvious code (self-documenting)
- Temporary workarounds (use TODO instead)
- Aspirational features (only document what exists)

## Output Format

```markdown
## Documentation Updates

### Files Modified
- `README.md` - Added new feature section
- `docs/api.md` - Documented new endpoints
- `src/service.ts` - Added inline comments for complex logic

### Summary of Changes
1. Added usage instructions for `--new-flag`
2. Documented `createUser` API with examples
3. Updated configuration table with new options

### Verification
- [ ] Docs build successfully
- [ ] Examples are copy-pasteable and work
- [ ] No broken internal links
```

## Failure Handling

### No Documentation Exists

If this is a new project without docs structure:
1. Create minimal README.md
2. Document the feature inline
3. Note: "Recommend creating docs/ structure in future"

### Documentation Tool Broken

If docs build fails:
1. Fix the build issue if simple
2. Document what was attempted
3. Note: "Docs build issue - needs separate fix"

### Unclear What to Document

1. Focus on public API only
2. Document what a new user would need
3. Skip internal details
4. When in doubt, add an example

## Verification Checklist

Before completing:
- [ ] All new public APIs are documented
- [ ] README is updated if user-facing changes
- [ ] Examples are tested and work
- [ ] Docs build (if applicable)
- [ ] Changes are committed

## Completion

When documentation is complete:

```
Documentation complete.
- Updated: [list of files]
- Added: [new sections/files]

SDLC pipeline complete for this story.
```

## Integration with SDLC Pipeline

```
[DESIGN] → [TEST] → [DEV] → [VERIFY] → [DOCS]
                                          ↑
                                     YOU ARE HERE
```

- **Input**: Verified, working implementation
- **Output**: Updated documentation
- **Next**: Story complete, ready for merge
