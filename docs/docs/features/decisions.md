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

## Inheriting from Parent Projects

When a project sits inside another project — a submodule in a superrepo, a
nested repository — sessions inherit the ancestors' decisions automatically.
Nothing to configure: at session start aide resolves the chain of containing
projects (see `aide anchor`) and overlays each ancestor's decisions onto the
context, labeled `inherited from parent <name>`.

Shadowing is by topic, nearest wins: decide a topic locally and every
ancestor's version of that topic is ignored for your sessions (theirs stay
intact upstream). Nothing is copied between stores — provenance is
synthesized at read time — and `decision get` stays store-local.

To record a decision *at* the estate level instead, route the write upward:

```bash
aide --store parent decision set logging "slog, structured"  # nearest container
aide --store top decision set go-version "1.26"              # the estate root
```

## Subscribing to Other Teams

Teams that don't share a filesystem subscribe to each other's decisions over
plain git. Name your sources in `.aide/config/aide.json`:

```json
{ "subscriptions": [
    { "name": "platform-team", "url": "git@host:platform/context.git", "branch": "main" },
    { "name": "team-context",  "url": "git@host:team/context.git", "publish": true },
    { "name": "proto-repo",    "path": "../protos" }
] }
```

Peer decisions appear in session context as a read-only layer labeled
`from peer <name>`, at the lowest precedence (local > ancestors > peers).
They are never re-exported — you only publish what you authored or
explicitly adopted:

```bash
aide decision adopt api-style --from=platform-team
```

copies the peer's current decision into your store as a new local decision
with adoption provenance. A subscription with `"publish": true` is two-way:
your own decisions are pushed back out for others to subscribe to.

No scheduler is needed: session **start** refreshes stale subscription
caches and session **end** publishes — decisions are made inside sessions,
so the session lifecycle is the sync loop. `aide sync` remains the manual
lever. Only decisions cross project boundaries; memories and state never do.
`AIDE_CASCADE_DISABLED=1` turns off both the ancestor cascade and the peer
layer. See the [CLI reference](/docs/reference/cli#sync--subscriptions) for
details.

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
