---
sidebar_position: 1
---

# Memory System

Memories persist across sessions and auto-inject at session start. They let your AI assistant remember your preferences, patterns, and context without you having to repeat yourself.

## How It Works

```
You: remember that I prefer vitest for testing
# Next session...
You: what testing framework should I use?
AI: Based on your preferences, you prefer vitest for testing.
```

## Memory Categories

Memories are organized by category:

- `learning` -- Things you've learned
- `decision` -- Choices you've made
- `session` -- Session-specific context
- `pattern` -- Recurring patterns
- `gotcha` -- Things to watch out for
- `discovery` -- New findings
- `blocker` -- Blockers encountered
- `issue` -- Issues found

## Memory Scoping

Tags control when and where memories are injected:

| Tag              | Scope                      | Behavior                                             |
| ---------------- | -------------------------- | ---------------------------------------------------- |
| `scope:global`   | All projects, all sessions | Injected at every session start, across all projects |
| `project:<name>` | Single project             | Injected only when working in the named project      |
| `session:<id>`   | Single session             | Groups memories from the same session                |

At session start, AIDE fetches:

1. All memories tagged `scope:global` (cross-project preferences)
2. All memories tagged `project:<current-project>` (project-specific context)
3. Recent session groups (memories sharing a `session:*` tag)

Memories without scope tags are still searchable via `memory_search` but won't be auto-injected.

## Auto-Injection

| Event              | What Gets Injected                                                             |
| ------------------ | ------------------------------------------------------------------------------ |
| Session Start      | Global memories, project memories, project decisions, recent session summaries |
| Subagent Spawn     | Global memories, project memories, project decisions                           |
| Context Compaction | State snapshot to preserve across summarization                                |

## Session Summaries

Session summaries are automatically captured when a session ends with meaningful activity (files modified, tools used, or git commits made).

## CLI Commands

```bash
aide memory add --category=learning --tags=testing "Prefers vitest"
aide memory search "authentication"
aide memory list --category=learning
aide memory delete <id>
aide memory reindex                      # Rebuild search index
aide memory export --format=markdown     # Export to markdown
```

## Skills for Memory

| Skill            | Trigger                        | Purpose                             |
| ---------------- | ------------------------------ | ----------------------------------- |
| `/aide:memorise` | `remember I prefer vitest`     | Store info for future sessions      |
| `/aide:recall`   | `what testing framework?`      | Search memories and decisions       |
| `/aide:forget`   | `forget the old auth decision` | Soft-delete or hard-delete memories |
