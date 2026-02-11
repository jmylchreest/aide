---
name: memorise
description: Capture key learnings, patterns, gotchas, or context as persistent memories
triggers:
  - remember this
  - remember that
  - dont forget
  - memorise
  - note that
  - note this
  - save this
  - store this
  - for future
  - keep in mind
allowed-tools: Bash(aide memory add *)
---

# Memorise

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Capture important information for future sessions by storing it in the aide memory database.

## How to Store

Use the `aide memory add` CLI command via Bash:

```bash
aide memory add --category=<category> --tags=<comma,separated,tags> "<content>"
```

## Categories

- `learning` - Something discovered about the codebase or tools
- `decision` - An architectural or design choice made
- `session` - Summary of a work session
- `pattern` - A reusable approach or pattern identified
- `gotcha` - A pitfall or issue to avoid in future

## When to Use

- End of a significant task or session
- After discovering something important about the codebase
- When a decision is made that should persist
- After solving a tricky problem (capture the solution)
- When user shares a preference or important information

## Examples

### Simple preference (global - injected at session start)
```bash
aide memory add --category=learning --tags=preferences,colour,scope:global "User's favourite colour is blue"
```

### Technical learning (project-specific)
```bash
aide memory add --category=learning --tags=testing,vitest,project:myapp,session:abc12345 "Vitest requires .js extensions for ESM imports even for .ts files. Configure moduleResolution: NodeNext in tsconfig."
```

### Session summary
```bash
aide memory add --category=session --tags=auth,api,project:myapp,session:abc12345 "Implemented JWT auth with 15min access tokens, 7day refresh tokens in httpOnly cookies. Files: src/auth/jwt.ts, src/middleware/auth.ts, src/routes/auth.ts"
```

### Gotcha (global - applies everywhere)
```bash
aide memory add --category=gotcha --tags=hooks,claude-code,scope:global "Hooks must not write to stderr - Claude Code interprets any stderr as error. Debug logging must go to files only."
```

## Instructions

When the user invokes `/aide:memorise <something>`:

1. Parse what they want to remember
2. Determine the scope:
   - **User preference** (colour, style, etc.) → add `scope:global`
   - **Project-specific learning** → add `project:<project-name>,session:${CLAUDE_SESSION_ID:0:8}`
   - **Session summary** → add `project:<project-name>,session:${CLAUDE_SESSION_ID:0:8}`
3. Choose appropriate category and descriptive tags
4. Format the content concisely but completely
5. Call `aide memory add` via Bash to store it
6. **Verify success** - check exit code is 0 and output contains the memory ID
7. Confirm what was stored

Keep content concise - aim for 1-3 sentences unless it's a complex session summary.

## Failure Handling

If `aide memory add` fails:

1. **Check error message** - common issues:
   - Database not accessible: ensure aide MCP server is running
   - Invalid category: use one of `learning`, `decision`, `session`, `pattern`, `gotcha`
   - Empty content: content must be non-empty

2. **Retry with fixes** if the issue is correctable

3. **Report failure** if unable to store:
   ```
   Failed to store memory: <error details>
   Please check aide MCP server status.
   ```

## Verification

A successful memory add returns:
```
Added memory: <ULID>
```

You can verify by searching for the memory:
```bash
aide memory search "<key term from content>" --limit=1
```

## Scope Tags

Use scope tags to control when memories are injected:

| Tag | When to Use | Injection |
|-----|-------------|-----------|
| `scope:global` | User preferences, universal learnings | Every session |
| `project:<name>` | Project-specific learnings | Sessions in that project |
| `session:<id>` | Context for this work session | Recent session injection |

### Tagging Rules

- **User preferences** (favourite colour, coding style): Always add `scope:global`
- **Project learnings** (API patterns, testing approach): Add `project:<name>,session:<id>`
- **Session summaries**: Add `project:<name>,session:<id>` with `category=session`

Get the project name from the git remote or directory name. Session ID is available as `$CLAUDE_SESSION_ID` (use first 8 chars).

## For Swarm/Multi-Agent

Add agent context to tags:
```bash
aide memory add --category=session --tags=swarm,agent:main "Overall task outcome..."
```
