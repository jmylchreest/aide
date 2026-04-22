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

Exported files use YAML frontmatter + markdown body, so they work as LLM context even without AIDE installed.

### Share decisions with your team

1. Run `aide share export` and commit `.aide/shared/` to git.
2. Teammates add `AIDE_SHARE_AUTO_IMPORT=1` to `.claude/settings.json` (also tracked in git).
3. On the next session start, aide imports from `.aide/shared/` — new decisions arrive automatically; decisions whose text differs from the local latest are **appended** to the topic thread (incoming becomes the new latest); identical decisions are skipped.

Decisions are append-only per topic — superseding, not overwriting. To change course, commit a new decision for the topic; it becomes the new latest on the next import, and the previous one stays visible via `aide decision history <topic>`. For the full conflict-resolution rules (including memories), see [Storage → Import Conflict Resolution](/docs/reference/storage#import-conflict-resolution).

## Blueprints

For language-specific best practices, use [Blueprints](/docs/features/blueprints) to seed decisions in bulk:

```bash
aide blueprint import go                   # 18 Go best-practice decisions
aide blueprint import go go-github-actions # + CI/CD decisions
aide blueprint import --detect             # auto-detect from project markers
```

Blueprint-imported decisions work exactly like manually recorded ones — they are injected into every session and can be overridden with `aide decision set`.

## Skill

Use `/aide:decide` for formal decision-making interviews that record architectural choices with full rationale.
