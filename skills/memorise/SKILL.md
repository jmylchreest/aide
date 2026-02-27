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
allowed-tools: Bash(./.aide/bin/aide memory add *)
---

# Memorise

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Capture important information for future sessions by storing it in the aide memory database.

## How to Store

Use the `./.aide/bin/aide memory add` CLI command via Bash:

```bash
./.aide/bin/aide memory add --category=<category> --tags=<comma,separated,tags> "<content>"
```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

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
./.aide/bin/aide memory add --category=learning --tags=preferences,colour,scope:global,source:user "User's favourite colour is blue"
```

### Technical learning (project-specific, verified)

```bash
./.aide/bin/aide memory add --category=learning --tags=testing,vitest,project:myapp,session:abc12345,source:discovered,verified:true "Vitest requires .js extensions for ESM imports even for .ts files. Configure moduleResolution: NodeNext in tsconfig."
```

### Session summary

```bash
./.aide/bin/aide memory add --category=session --tags=auth,api,project:myapp,session:abc12345,source:discovered "Implemented JWT auth with 15min access tokens, 7day refresh tokens in httpOnly cookies. Files: src/auth/jwt.ts, src/middleware/auth.ts, src/routes/auth.ts"
```

### Gotcha (global - applies everywhere)

```bash
./.aide/bin/aide memory add --category=gotcha --tags=hooks,claude-code,scope:global,source:discovered,verified:true "Hooks must not write to stderr - Claude Code interprets any stderr as error. Debug logging must go to files only."
```

### Unverified external claim

```bash
./.aide/bin/aide memory add --category=learning --tags=api,stripe,project:myapp,source:user,verified:false "Stripe webhook signatures use HMAC-SHA256 with the whsec_ prefix."
```

## Instructions

When the user invokes `/aide:memorise <something>`:

1. Parse what they want to remember
2. **Verify factual claims before storing** (see [Verification Before Storage](#verification-before-storage-anti-poison) below)
3. Determine the scope:
   - **User preference** (colour, style, etc.) → add `scope:global`
   - **Project-specific learning** → add `project:<project-name>,session:${CLAUDE_SESSION_ID:0:8}`
   - **Session summary** → add `project:<project-name>,session:${CLAUDE_SESSION_ID:0:8}`
4. Choose appropriate category and descriptive tags
5. **Add provenance tags** (see [Provenance Tags](#provenance-tags) below)
6. Format the content concisely but completely
7. Call `./.aide/bin/aide memory add` via Bash to store it
8. **Verify success** - check exit code is 0 and output contains the memory ID
9. Confirm what was stored, including verification outcome

Keep content concise - aim for 1-3 sentences unless it's a complex session summary.

## Verification Before Storage (Anti-Poison)

Memories persist across sessions and influence future behaviour. Storing incorrect information is **worse than storing nothing** — it creates compounding errors. Before storing any memory, verify its claims.

### What to Verify

| Claim Type                | Verification Method                                   | Example                                        |
| ------------------------- | ----------------------------------------------------- | ---------------------------------------------- |
| **File exists**           | Use Glob or Read to confirm                           | "Config is in src/config.ts"                   |
| **Function/class exists** | Use Grep or code search                               | "Use `parseToken()` from auth.ts"              |
| **Function signature**    | Read the file, check the actual signature             | "parseToken takes a string and returns Claims" |
| **API behaviour**         | Check the implementation or tests                     | "The /users endpoint requires auth"            |
| **Dependency/version**    | Check package.json, go.mod, etc.                      | "Project uses Vitest v2"                       |
| **Build/test command**    | Confirm the script exists in package.json or Makefile | "Run `npm run test:e2e` for integration tests" |

### What Does NOT Need Verification

- **User preferences** — The user is the authority ("I prefer tabs over spaces")
- **Session summaries** — Recap of what just happened in the current session
- **Opinions/decisions** — Architectural choices made by the user ("We chose Postgres")
- **External facts** — Things not checkable against the codebase ("React 19 uses...")

### Verification Workflow

```
Is it a codebase claim (file, function, path, command, behaviour)?
├── No → Store directly with source:user or source:stated
└── Yes → Can you verify it right now?
          ├── Yes → Verify it
          │         ├── Correct → Store with verified:true
          │         └── Wrong → Inform user, do NOT store. Offer corrected version.
          └── No (e.g., external service, runtime behaviour)
                    → Store with verified:false, note unverified
```

### Verification Rules

1. **NEVER store a codebase claim without checking** — If the user says "the auth middleware is in src/middleware/auth.ts", confirm the file exists before memorising.
2. **If verification fails, do NOT store** — Tell the user what you found instead and offer to store the corrected version.
3. **If you cannot verify** (e.g., claim about runtime behaviour, external API), store it but tag with `verified:false`.
4. **Err on the side of not storing** — A missing memory is recoverable; a wrong memory causes future errors.

### Example: Verified vs Rejected

**User says:** "Remember that the database schema is defined in db/schema.sql"

**Verification steps:**

1. Check: does `db/schema.sql` exist? → Use Glob to search
2. If yes → Store with `verified:true`
3. If no → "I checked and `db/schema.sql` doesn't exist. I found `database/migrations/` instead. Would you like me to store that instead?"

## Provenance Tags

Always include provenance tags to track the origin and verification status of memories:

| Tag                 | Meaning                                  | When to Use                                      |
| ------------------- | ---------------------------------------- | ------------------------------------------------ |
| `source:user`       | User explicitly stated this              | User preferences, direct instructions            |
| `source:discovered` | Agent discovered this by examining code  | File paths, function signatures, patterns found  |
| `source:inferred`   | Agent inferred this from context         | Behaviour deduced from code, not directly stated |
| `verified:true`     | Codebase claim was checked and confirmed | After successful verification                    |
| `verified:false`    | Claim could not be verified against code | External facts, runtime behaviour                |

### Provenance Rules

- Every memory MUST have exactly one `source:` tag
- Codebase claims MUST have a `verified:` tag
- User preferences and opinions do NOT need `verified:` tags (user is authoritative)

## Failure Handling

If `./.aide/bin/aide memory add` fails:

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
./.aide/bin/aide memory search "<key term from content>" --limit=1
```

## Scope Tags

Use scope tags to control when memories are injected:

| Tag              | When to Use                           | Injection                |
| ---------------- | ------------------------------------- | ------------------------ |
| `scope:global`   | User preferences, universal learnings | Every session            |
| `project:<name>` | Project-specific learnings            | Sessions in that project |
| `session:<id>`   | Context for this work session         | Recent session injection |

### Tagging Rules

- **User preferences** (favourite colour, coding style): Always add `scope:global`
- **Project learnings** (API patterns, testing approach): Add `project:<name>,session:<id>`
- **Session summaries**: Add `project:<name>,session:<id>` with `category=session`

Get the project name from the git remote or directory name. Session ID is available as `$CLAUDE_SESSION_ID` (use first 8 chars).

## For Swarm/Multi-Agent

Add agent context to tags:

```bash
./.aide/bin/aide memory add --category=session --tags=swarm,agent:main "Overall task outcome..."
```
