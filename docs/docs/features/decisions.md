---
sidebar_position: 2
---

# Decisions

Decisions are a specialized memory type for architectural choices that need to be enforced. They are append-only: when a new decision is set for an existing topic, it supersedes the old one. The history is preserved, but only the latest decision is injected into context.

## Usage

```bash
aide decision set "auth-strategy" "JWT with refresh tokens" --rationale="Stateless, mobile-friendly"
aide decision get "auth-strategy"     # Latest decision
aide decision list                    # All decisions
aide decision history "auth-strategy" # Full history
```

## How Decisions Differ from Memories

- **Decisions** are topic-keyed (e.g., `auth-strategy`) with versioned history
- **Memories** are general-purpose with category and tag-based organization
- All current project decisions are injected at session start and subagent spawn
- Decisions have structured fields: topic, decision, rationale, details, references

## Sharing via Git

```bash
aide share export                    # Export decisions + memories to .aide/shared/
aide share export --decisions        # Decisions only
aide share import                    # Import from .aide/shared/
aide share import --dry-run          # Preview what would be imported
```

Set `AIDE_SHARE_AUTO_IMPORT=1` for automatic import on session start.

Exported files use YAML frontmatter + markdown body, so they work as LLM context even without AIDE installed.

## Skill

Use `/aide:decide` for formal decision-making interviews that record architectural choices with full rationale.
