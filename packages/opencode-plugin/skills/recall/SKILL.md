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
---

# Recall

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Search stored memories and decisions to answer questions about past learnings, architectural choices, patterns, and project context.

## MCP Tools

### Search Memories

| Tool | Purpose |
|------|---------|
| `mcp__plugin_aide_aide__memory_search` | Full-text search (fuzzy, prefix, substring matching) |
| `mcp__plugin_aide_aide__memory_list` | List memories, optionally filtered by category |

### Get Decisions

| Tool | Purpose |
|------|---------|
| `mcp__plugin_aide_aide__decision_get` | Get specific decision by topic |
| `mcp__plugin_aide_aide__decision_list` | List all decisions |
| `mcp__plugin_aide_aide__decision_history` | Get all versions of a decision |

## Workflow

1. **Parse the question** - Extract key terms
2. **Search both sources:**
   - Use `mcp__plugin_aide_aide__memory_search` with relevant keywords
   - Use `mcp__plugin_aide_aide__decision_get` with the topic, or `mcp__plugin_aide_aide__decision_list`
3. **Analyze timestamps** - Prefer most recent (ULIDs are time-ordered)
4. **Answer** - Combine relevant context from both sources

## Instructions

When the user asks about previous context:

1. **For architectural/design questions** (testing, auth, database, etc.):
   - Use `mcp__plugin_aide_aide__decision_get` with topic (e.g., "testing")
   - If unsure of topic name, use `mcp__plugin_aide_aide__decision_list`

2. **For learnings/patterns/gotchas:**
   - Use `mcp__plugin_aide_aide__memory_search` with query (e.g., "ESM imports")

3. **When answering:**
   - Cite the source (memory or decision)
   - Include the date for context
   - Note if something was updated/changed

## Examples

**User:** "What testing framework did we decide on?"
→ Use `mcp__plugin_aide_aide__decision_get` with topic="testing"
→ Answer with the decision and rationale

**User:** "What was the issue with ESM imports?"
→ Use `mcp__plugin_aide_aide__memory_search` with query="ESM imports"
→ Answer with the learning

**User:** "What approach did we take for auth?"
→ Use both `mcp__plugin_aide_aide__decision_get` (topic="auth") and `mcp__plugin_aide_aide__memory_search` (query="auth")
→ Combine both sources in answer

## Failure Handling

If no results found:

1. **Try alternative terms** - search with synonyms or related keywords
2. **Broaden the search** - use `mcp__plugin_aide_aide__memory_list` to browse all memories
3. **Report clearly**: "No memories or decisions found for '<topic>'. This may not have been recorded previously."

## Notes

- This skill is READ-ONLY - searches but doesn't modify
- Decisions are structured (topic → decision + rationale)
- Memories are freeform learnings, gotchas, patterns
- Always prefer the most recent when there are conflicts
