# Preflight-Exec: Concept Document

Status: **Concept only** — not implemented in current phase.

## Problem

AIDE skills currently execute in a purely LLM-driven loop: the skill prompt tells the
agent what tools to call, but there's no way to run deterministic setup steps *before*
the LLM starts reasoning. This means:

1. Every review session spends tokens calling `code_stats` / `decision_list` / `memory_list`
   even though these calls are predictable.
2. Skills can't enforce prerequisites (e.g. "index must exist before searching").
3. There's no way to inject fresh analysis data into the context window without the LLM
   requesting it.

## Proposed Solution

Add an optional `preflight` block to skill frontmatter that declares commands to run
before the skill prompt is sent to the LLM. The commands run deterministically (no LLM
involvement) and their output is injected into the conversation context.

### Frontmatter Schema

```yaml
---
name: review
description: Code review and security audit
triggers:
  - review this
  - code review
preflight:
  - tool: findings_stats
    inject: context        # Add output to system context
  - tool: findings_list
    args:
      severity: critical
    inject: context
  - command: aide code stats
    inject: context
    optional: true         # Don't fail if code isn't indexed
---
```

### Execution Flow

```
User message matches skill trigger
        │
        ▼
Skill frontmatter parsed
        │
        ▼
Preflight commands executed (deterministic, no LLM)
        │
        ├─ MCP tool calls → results collected
        ├─ CLI commands → stdout captured
        └─ Failures: skip if optional, abort if required
        │
        ▼
Results injected into conversation context
        │
        ▼
Skill prompt + preflight context → LLM
```

### Injection Modes

| Mode | Behavior |
|------|----------|
| `context` | Injected as a system message before the skill prompt |
| `variable` | Available as `{{preflight.tool_name}}` in the prompt template |
| `silent` | Executed for side effects only (e.g. ensuring index exists) |

## Why Not Now

1. **Session-init already covers the common case**: `session-init.ts` injects decisions
   and memories at startup. Adding preflight would overlap.
2. **Implementation complexity**: Requires changes to skill-matcher, skill-injector hooks,
   and potentially the MCP transport layer.
3. **Risk**: Arbitrary command execution before LLM reasoning needs careful sandboxing.

## When to Implement

Consider implementing when:
- Multiple skills need the same preflight pattern (findings check before review)
- The findings store is mature and agents consistently benefit from pre-loaded data
- Users request skill-level automation beyond what session-init provides

## Alternative Approaches

1. **Skill-level hooks**: TypeScript hooks per skill (more flexible, more complex)
2. **Auto-tool-calls**: LLM always calls specific tools first (wastes tokens)
3. **Cached context**: Pre-render and cache tool outputs, refresh on file changes

## References

- Enaible's `shared/prompts/` system renders prompts with adapter variables at build time
- AIDE's `session-init.ts` already does startup injection (decisions + memories)
- `src/hooks/skill-injector.ts` is the current skill injection point
