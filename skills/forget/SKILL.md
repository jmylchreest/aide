---
name: forget
description: Soft-delete memories by adding/removing the forget tag, or hard-delete outdated memories
triggers:
  - forget this
  - forget that
  - forget memory
  - remove memory
  - delete memory
  - outdated memory
  - wrong memory
  - incorrect memory
  - supersede memory
  - obsolete
  - no longer true
  - no longer relevant
  - not true anymore
  - that was wrong
  - disregard
allowed-tools: Bash(./.aide/bin/aide memory *)
---

# Forget

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Manage outdated, incorrect, or obsolete memories by soft-deleting (tagging with `forget`) or hard-deleting them.

## Key Concepts

### Soft Delete (Preferred)

Memories tagged with `forget` are **excluded by default** from search, list, and context injection. They remain in the database and can be recovered.

```bash
./.aide/bin/aide memory tag <MEMORY_ID> --add=forget
```

### Hard Delete (Permanent)

Permanently removes a memory from the database. Use when the memory is clearly wrong or contains sensitive data.

```bash
./.aide/bin/aide memory delete <MEMORY_ID>
```

### Unforget (Recover)

Remove the `forget` tag to restore a soft-deleted memory.

```bash
./.aide/bin/aide memory tag <MEMORY_ID> --remove=forget
```

## Workflow

When the user wants to forget, correct, or supersede a memory:

### Step 1: Find the Memory

Search for the memory to identify its ID:

```bash
# Search by content keywords
./.aide/bin/aide memory search "<keywords>" --full --limit=10

# If the memory might already be forgotten, include all
./.aide/bin/aide memory list --all
```

### Step 2: Confirm with User

Before deleting or forgetting, show the user what was found and confirm which memories to act on. Display the memory ID, content, and tags.

### Step 3: Apply the Appropriate Action

Choose based on the situation:

| Situation                            | Action                                  | Command                                |
| ------------------------------------ | --------------------------------------- | -------------------------------------- |
| Memory is outdated but was once true | Soft delete (forget)                    | `aide memory tag <ID> --add=forget`    |
| Memory is factually wrong            | Hard delete                             | `aide memory delete <ID>`              |
| Memory is resolved (issue fixed)     | Soft delete + optionally add resolution | See "Superseding" below                |
| Memory contains sensitive data       | Hard delete                             | `aide memory delete <ID>`              |
| Memory was accidentally forgotten    | Unforget                                | `aide memory tag <ID> --remove=forget` |

### Step 4: Optionally Add a Replacement

If the memory is being superseded (not just deleted), add a new corrected memory **without** the `forget` tag:

```bash
# 1. Forget the old memory
./.aide/bin/aide memory tag <OLD_ID> --add=forget

# 2. Add the corrected memory (NO forget tag)
./.aide/bin/aide memory add --category=<category> --tags=<relevant,tags> "<corrected content>"
```

### Step 5: Verify

Confirm the memory is no longer visible in normal searches:

```bash
# Should NOT appear in normal search
./.aide/bin/aide memory search "<keywords>"

# Should appear with --all flag
./.aide/bin/aide memory list --all
```

## Anti-Patterns (AVOID)

### DO NOT add a new memory with the forget tag to "supersede" an old one

This is the most common mistake. Adding a new memory tagged `forget` does nothing useful:

- The new "superseding" memory is immediately hidden (because it has the `forget` tag)
- The original incorrect memory remains visible and active

**Wrong:**

```bash
# BAD - creates a hidden memory, leaves the original untouched
aide memory add --tags="forget,some-topic" "RESOLVED: the old issue is fixed"
```

**Correct:**

```bash
# GOOD - forget the original, add a visible replacement
aide memory tag <ORIGINAL_ID> --add=forget
aide memory add --tags="some-topic" "RESOLVED: the old issue is fixed"
```

### DO NOT guess memory IDs

Always search first to find the exact memory ID. Memory IDs are ULIDs (e.g., `01KJ3X7GMQET613WMPT7JT8GYD`).

### DO NOT forget memories without user confirmation

Always show the user what will be affected and get confirmation before acting, especially for hard deletes.

## Batch Operations

To forget multiple related memories:

```bash
# Search for all related memories
./.aide/bin/aide memory search "<topic>" --full --limit=50

# Forget each one (after user confirmation)
./.aide/bin/aide memory tag <ID1> --add=forget
./.aide/bin/aide memory tag <ID2> --add=forget
```

To clear ALL memories (destructive, requires explicit user request):

```bash
./.aide/bin/aide memory clear
```

## Examples

### User says "that thing about ESM imports is wrong"

```bash
# 1. Find the memory
./.aide/bin/aide memory search "ESM imports" --full

# 2. Show user and confirm
# "Found memory 01KJ... about ESM imports requiring .js extensions. Delete this?"

# 3. Soft delete
./.aide/bin/aide memory tag 01KJ3XVFXBFTKKH1M500N6R5 --add=forget
```

### User says "we changed from JWT to sessions, update the memory"

```bash
# 1. Find the old memory
./.aide/bin/aide memory search "JWT auth" --full

# 2. Forget the old one
./.aide/bin/aide memory tag <OLD_ID> --add=forget

# 3. Add the corrected one
./.aide/bin/aide memory add --category=decision --tags=auth,sessions,project:myapp "Auth strategy changed from JWT to server-side sessions with Redis store"
```

### User says "recover that forgotten memory about testing"

```bash
# 1. Find forgotten memories
./.aide/bin/aide memory list --all

# 2. Unforget it
./.aide/bin/aide memory tag <ID> --remove=forget
```

## Failure Handling

If `aide memory tag` fails:

1. **"memory not found"** - The ID may be wrong. Re-search with `--all` flag to include forgotten memories.
2. **Panic/crash** - If the tag command panics, fall back to delete + re-add:
   ```bash
   # Workaround: get memory content, delete, re-add with forget tag
   ./.aide/bin/aide memory list --all  # Find the memory content
   ./.aide/bin/aide memory delete <ID>
   ./.aide/bin/aide memory add --category=<cat> --tags=<existing-tags>,forget "<content>"
   ```
3. **Database locked** - Another process may hold the lock. Wait and retry, or ensure the aide daemon is running (CLI routes through gRPC when daemon is active).

## Verification

After forgetting a memory, verify:

```bash
# Memory should NOT appear here
./.aide/bin/aide memory search "<term>"

# Memory SHOULD appear here (with forget tag)
./.aide/bin/aide memory list --all
```
