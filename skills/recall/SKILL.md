---
name: recall
description: Search memories and decisions to answer questions about past learnings, choices, and context
triggers:
  - what did we
  - what was
  - what is the
  - do you remember
  - did we decide
  - recall
  - from memory
  - previously
allowed-tools: Bash(aide memory *), Bash(aide decision *)
---

# Recall

Search stored memories and decisions to answer questions about past learnings, architectural choices, patterns, and project context.

## CLI Commands

### Search Memories

```bash
# Full-text search (fuzzy, prefix, substring matching)
aide memory search "<query>" --limit=10

# Exact substring match (for precise matching)
aide memory select "<exact term>"

# List all memories
aide memory list --limit=50

# List by category
aide memory list --category=learning
aide memory list --category=decision
aide memory list --category=gotcha
```

### Get Decisions

```bash
# Get specific decision by topic
aide decision get <topic>

# List all decisions
aide decision list

# Get decision history (all versions of a decision)
aide decision history <topic>
```

## Workflow

1. **Parse the question** - Extract key terms
2. **Search both sources:**
   - Search memories: `aide memory search "<keywords>"`
   - Check decisions: `aide decision get <topic>` or `aide decision list`
3. **Analyze timestamps** - Prefer most recent (ULIDs are time-ordered)
4. **Answer** - Combine relevant context from both sources

## Instructions

When the user asks about previous context:

1. **For architectural/design questions** (testing, auth, database, etc.):
   ```bash
   aide decision get testing
   # or if unsure of topic name:
   aide decision list
   ```

2. **For learnings/patterns/gotchas:**
   ```bash
   aide memory search "ESM imports"
   # or for exact matches:
   aide memory select "ESM"
   ```

3. **When answering:**
   - Cite the source (memory or decision)
   - Include the date for context
   - Note if something was updated/changed

## Examples

**User:** "What testing framework did we decide on?"
```bash
aide decision get testing
```
→ Answer with the decision and rationale

**User:** "What was the issue with ESM imports?"
```bash
aide memory search "ESM imports"
```
→ Answer with the learning

**User:** "What approach did we take for auth?"
```bash
aide decision get auth
aide memory search "auth"
```
→ Combine both sources in answer

## Failure Handling

If no results found:

1. **Try alternative terms** - search with synonyms or related keywords
2. **Broaden the search** - use `aide memory list --limit=50` to browse
3. **Report clearly**: "No memories or decisions found for '<topic>'. This may not have been recorded previously."

If command fails:

1. Check if aide MCP server is running (Claude Code plugin)
2. Report the error to the user

## Notes

- This skill is READ-ONLY - searches but doesn't modify
- Decisions are structured (topic → decision + rationale)
- Memories are freeform learnings, gotchas, patterns
- Always prefer the most recent when there are conflicts
- Use `--full` flag on memory commands to see full content (not truncated)
